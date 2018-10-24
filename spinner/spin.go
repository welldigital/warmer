package spinner

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/welldigital/dmd/version"
)

var spinInit sync.Once
var spinAge time.Time
var spinID string

// SpinResult is the result of a spin.
type SpinResult struct {
	OK      bool      `json:"ok"`
	Version string    `json:"version"`
	ID      string    `json:"id"`
	Born    time.Time `json:"born"`
}

// AddSpin adds spin as middleware using the specified path.
func AddSpin(path string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.String() == path {
			Spin(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Spin is a response handler which returns a unique ID per application start.
func Spin(w http.ResponseWriter, r *http.Request) {
	spinInit.Do(func() {
		spinID = ksuid.New().String()
		spinAge = time.Now().UTC()
	})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	rv := SpinResult{
		OK:      true,
		Version: version.Version,
		ID:      spinID,
		Born:    spinAge,
	}
	// There's a unit test to check marshalling, making a runtime error unlikely.
	bytes, _ := json.Marshal(rv)
	w.Write(bytes)
}
