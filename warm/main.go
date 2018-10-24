package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/welldigital/warmer/spinner"

	"github.com/pkg/errors"

	"github.com/aws/aws-lambda-go/events"

	lambdago "github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/lambda"
	log "github.com/sirupsen/logrus"
)

func init() {
	log.SetFormatter(&log.JSONFormatter{})
}

var matcher = regexp.MustCompile(`LAMBDA_(\d+)_FUNCTION`)

// Handler is the handler for the warmer.
func Handler() (err error) {
	// Extract details from the environment.
	region := os.Getenv("REGION")
	if region == "" {
		region = "eu-west-2"
	}
	functions := []functionDetails{}
	for _, v := range os.Environ() {
		if matcher.MatchString(v) {
			index, parseErr := strconv.ParseInt(matcher.FindStringSubmatch(v)[1], 10, 64)
			if parseErr != nil {
				err = errors.Wrapf(parseErr, "failed to parse value from: %v", v)
				return
			}
			fd := functionDetails{
				name:   os.Getenv(fmt.Sprintf("LAMBDA_%d_FUNCTION", index)),
				path:   os.Getenv(fmt.Sprintf("LAMBDA_%d_PATH", index)),
				count:  1,
				region: region,
			}
			if pCount, pErr := strconv.ParseInt(os.Getenv(fmt.Sprintf("LAMBDA_%d_COUNT", index)), 10, 32); pErr == nil && pCount > 0 {
				fd.count = int(pCount)
			}
			functions = append(functions, fd)
		}
	}
	// Sum the total amount of Lambda invocations we're going to do.
	var count int
	for _, d := range functions {
		count += d.count
	}
	// Execute the functions concurrently, up to their count.
	c := make(chan executeResult, count)
	for _, fn := range functions {
		run(fn, c)
	}
	// Retrieve the results.
	var results []executeResult
	for i := 0; i < count; i++ {
		results = append(results, <-c)
	}
	// Log the achieved concurrency.
	functionNameToExecuteResult := map[string][]executeResult{}
	for _, r := range results {
		functionNameToExecuteResult[r.fn.name] = append(functionNameToExecuteResult[r.fn.name], r)
	}
	for fn, rr := range functionNameToExecuteResult {
		concurrency := map[string]struct{}{}
		var targetConcurrency int
		var path string
		for _, r := range rr {
			l := log.
				WithField("region", region).
				WithField("name", fn).
				WithField("path", r.fn.path).
				WithField("targetConcurrency", r.fn.count).
				WithField("timeTaken", r.timeTaken)
			path = r.fn.path
			targetConcurrency = r.fn.count
			if r.err != nil {
				l.WithError(r.err).Error("error running function")
				continue
			}
			concurrency[r.result.ID] = struct{}{}
			l.
				WithField("lambdaId", r.result.ID).
				WithField("lambdaBorn", r.result.Born).
				WithField("lambdaAgeMinutes", time.Now().Sub(r.result.Born).Minutes()).
				WithField("lambdaVersion", r.result.Version).
				Info("details")
		}
		log.
			WithField("region", region).
			WithField("name", fn).
			WithField("path", path).
			WithField("targetConcurrency", targetConcurrency).
			WithField("actualConcurrency", len(concurrency)).
			Info("execution complete")
	}
	return
}

type functionDetails struct {
	region string
	name   string
	path   string
	count  int
}

func run(fn functionDetails, result chan<- executeResult) {
	for i := 0; i < fn.count; i++ {
		go func() {
			result <- executeLambda(fn)
		}()
	}
}

type executeResult struct {
	fn        functionDetails
	result    spinner.SpinResult
	timeTaken time.Duration
	err       error
}

func executeLambda(fn functionDetails) (result executeResult) {
	result.fn = fn
	start := time.Now()
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	client := lambda.New(sess, &aws.Config{Region: aws.String(fn.region)})
	apir := events.APIGatewayProxyRequest{
		Path:       fn.path,
		HTTPMethod: "GET",
	}
	payload, err := json.Marshal(apir)
	if err != nil {
		result.err = fmt.Errorf("failed to marshal api request: %v", err)
		return
	}
	lambdaResult, err := client.Invoke(&lambda.InvokeInput{FunctionName: aws.String(fn.name), Payload: payload})
	if err != nil {
		result.err = fmt.Errorf("failed to invoke function '%v': %v", fn.name, err)
		return
	}
	var resp events.APIGatewayProxyResponse
	err = json.Unmarshal(lambdaResult.Payload, &resp)
	if err != nil {
		result.err = fmt.Errorf("failed to unmarshal payload '%v': %v", string(lambdaResult.Payload), err)
		return
	}
	// If the status code is NOT 200, the call failed
	if resp.StatusCode != 200 {
		result.err = fmt.Errorf("unexpected HTTP response: %v", resp.StatusCode)
		return
	}
	result.err = json.Unmarshal([]byte(resp.Body), &result.result)
	result.timeTaken = time.Now().Sub(start)
	return
}

func main() {
	lambdago.Start(Handler)
}
