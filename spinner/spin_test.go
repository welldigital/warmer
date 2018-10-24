package spinner

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSpinResultMarshal(t *testing.T) {
	sr := SpinResult{
		Born:    time.Now(),
		ID:      "123",
		OK:      true,
		Version: "v1238129746",
	}
	_, err := json.Marshal(sr)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
}

func TestSpin(t *testing.T) {
	t.Parallel()
	sr1, err := runSpin()
	if err != nil {
		t.Fatalf("failed to spin: %v", err)
	}
	sr2, err := runSpin()
	if err != nil {
		t.Fatalf("failed to spin: %v", err)
	}
	if sr1.ID != sr2.ID {
		t.Errorf("expected the same IDs, but they were different: %v, %v", sr1.ID, sr2.ID)
	}
	if !sr1.OK {
		t.Errorf("expected sr1.OK %v, got %v", true, sr1.OK)
	}
	if !sr2.OK {
		t.Errorf("expected sr1.OK %v, got %v", true, sr2.OK)
	}
}

func runSpin() (sr SpinResult, err error) {
	w := httptest.NewRecorder()
	Spin(w, httptest.NewRequest("GET", "/spin", nil))
	r := w.Result()
	if r.StatusCode != 200 {
		err = fmt.Errorf("expected status %d, got %d", 200, r.StatusCode)
		return
	}
	b, err := ioutil.ReadAll(w.Body)
	if err != nil {
		return
	}
	err = json.Unmarshal(b, &sr)
	return
}

func TestSpinMiddleware(t *testing.T) {

	w := httptest.NewRecorder()
	var baseCalled bool
	baseResponse := "Hello"
	baseHandler := func(w http.ResponseWriter, r *http.Request) {
		baseCalled = true
		w.Write([]byte(baseResponse))
	}
	handler := AddSpin("/spin", http.HandlerFunc(baseHandler))

	// try out spin request.
	spinRequest := httptest.NewRequest("GET", "/spin", nil)
	handler.ServeHTTP(w, spinRequest)
	var sr SpinResult
	dc := json.NewDecoder(w.Body)
	err := dc.Decode(&sr)
	if err != nil {
		t.Errorf("failed to read spin JSON result: %v", err)
	}
	if baseCalled {
		t.Error("the spin handler also ran the other handler")
	}

	baseCalled = false
	otherRequest := httptest.NewRequest("GET", "/spin/somethingelse", nil)
	handler.ServeHTTP(w, otherRequest)
	bytes, err := ioutil.ReadAll(w.Body)
	if err != nil {
		t.Fatalf("failed to read non-spin body: %v", err)
	}
	if baseResponse != string(bytes) {
		t.Errorf("non-spin body: expected '%v', got '%v'", baseResponse, string(bytes))
	}
}
