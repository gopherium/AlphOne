// SPDX-License-Identifier: Elastic-2.0

package storage

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"gocloud.dev/blob/s3blob"
)

// shortCleanup is the cleanup timeout the tests of a slow store set, far below the default.
const shortCleanup = 50 * time.Millisecond

// lateAnswer is how long a slow store waits before it answers, past shortCleanup and below the default.
const lateAnswer = 2 * time.Second

// requestLog records the method and the upload id of each request a store took.
type requestLog struct {
	mu   sync.Mutex
	took []string
}

// add records the method and the upload id of r.
func (l *requestLog) add(r *http.Request) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.took = append(l.took, r.Method+" "+r.URL.Query().Get("uploadId"))
}

// requests returns the method and the upload id of each request taken so far.
func (l *requestLog) requests() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.took...)
}

// storeAt returns the address of a store that records each request and answers it once answer returns.
func storeAt(t *testing.T, answer func(*http.Request)) (string, *requestLog) {
	t.Helper()
	log := &requestLog{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.add(r)
		answer(r)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	return server.URL, log
}

// answerLate holds the answer to r until r ends or lateAnswer passes.
func answerLate(r *http.Request) {
	select {
	case <-r.Context().Done():
	case <-time.After(lateAnswer):
	}
}

// exampleS3 returns the settings of the bucket example-files of the store at address.
func exampleS3(address string) S3Settings {
	return S3Settings{
		Endpoint: address, Region: "garage", Bucket: "example-files", AccessKeyID: "GKexample",
		SecretAccessKey: "example-secret", PathStyle: true, Checksum: WhenRequired,
	}
}

// clientAt returns the S3 client the settings build for the store at address, cleaning up within cleanup.
func clientAt(address string, cleanup time.Duration) *s3.Client {
	options, _ := s3Config(exampleS3(address), cleanup)
	return s3.New(options)
}

// endedContext returns a context that has already ended.
func endedContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	return ctx
}

// abortInput names the unfinished upload the abort tests remove.
func abortInput() *s3.AbortMultipartUploadInput {
	return &s3.AbortMultipartUploadInput{
		Bucket: aws.String("example-files"), Key: aws.String("t/notes/receipt"), UploadId: aws.String("upload-1"),
	}
}

// optionsView is what a test reads back from the client and bucket options the S3 settings build.
type optionsView struct {
	// endpoint is the address the client sends its requests to, empty for the AWS address of the region.
	endpoint string
	// endpointSet reports whether the client names an address of its own.
	endpointSet bool
	// region is the region the client signs for.
	region string
	// pathStyle reports whether the bucket goes in the path of each request.
	pathStyle bool
	// request is when the client adds a checksum to a request.
	request aws.RequestChecksumCalculation
	// response is when the client checks the checksum of an answer.
	response aws.ResponseChecksumValidation
	// upload is when the bucket adds a checksum to an upload.
	upload aws.RequestChecksumCalculation
}

// viewOf returns what the test reads back from client and bucket.
func viewOf(client s3.Options, bucket s3blob.Options) optionsView {
	return optionsView{
		endpoint:    aws.ToString(client.BaseEndpoint),
		endpointSet: client.BaseEndpoint != nil,
		region:      client.Region,
		pathStyle:   client.UsePathStyle,
		request:     client.RequestChecksumCalculation,
		response:    client.ResponseChecksumValidation,
		upload:      bucket.RequestChecksumCalculation,
	}
}

// keysOf returns the access key and its secret the client signs with, failing the test when it holds none.
func keysOf(t *testing.T, client s3.Options) aws.Credentials {
	t.Helper()
	if client.Credentials == nil {
		t.Fatal("Credentials = nil, want the static keys")
	}
	keys, err := client.Credentials.Retrieve(t.Context())
	if err != nil {
		t.Fatalf("Credentials.Retrieve() error = %v, want the static keys", err)
	}
	return aws.Credentials{AccessKeyID: keys.AccessKeyID, SecretAccessKey: keys.SecretAccessKey,
		SessionToken: keys.SessionToken}
}

func TestTheS3OptionsCarryWhatTheSettingsName(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		settings S3Settings
		want     optionsView
	}{
		"a store of its own checking only what needs a checksum": {
			settings: S3Settings{
				Endpoint: "https://files.example.com", Region: "garage", Bucket: "example-files",
				AccessKeyID: "GKexample", SecretAccessKey: "example-secret", PathStyle: true, Checksum: WhenRequired,
			},
			want: optionsView{
				endpoint: "https://files.example.com", endpointSet: true, region: "garage", pathStyle: true,
				request:  aws.RequestChecksumCalculationWhenRequired,
				response: aws.ResponseChecksumValidationWhenRequired,
				upload:   aws.RequestChecksumCalculationWhenRequired,
			},
		},
		"the AWS address of the region checking every checksum": {
			settings: S3Settings{
				Region: "eu-central-1", Bucket: "example-files", AccessKeyID: "AKIAEXAMPLE",
				SecretAccessKey: "example-secret", Checksum: WhenSupported,
			},
			want: optionsView{
				region:   "eu-central-1",
				request:  aws.RequestChecksumCalculationWhenSupported,
				response: aws.ResponseChecksumValidationWhenSupported,
				upload:   aws.RequestChecksumCalculationWhenSupported,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client, bucket := s3Config(tc.settings, 0)

			if got := viewOf(client, bucket); got != tc.want {
				t.Errorf("the options = %+v, want %+v", got, tc.want)
			}
			want := aws.Credentials{AccessKeyID: tc.settings.AccessKeyID, SecretAccessKey: tc.settings.SecretAccessKey}
			if got := keysOf(t, client); got != want {
				t.Errorf("the keys = %+v, want the static keys %+v and no session token", got, want)
			}
		})
	}
}

func TestTheAbortOfAnUploadGoesOutAfterItsContextEndedAndNoOtherRequestDoes(t *testing.T) {
	t.Parallel()

	tests := map[string]time.Duration{
		"a cleanup timeout of its own":       time.Second,
		"no cleanup timeout, so the default": 0,
	}

	for name, cleanup := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			address, log := storeAt(t, func(*http.Request) {})
			client := clientAt(address, cleanup)
			ended := endedContext(t)

			_, abortErr := client.AbortMultipartUpload(ended, abortInput())
			_, completeErr := client.CompleteMultipartUpload(ended, &s3.CompleteMultipartUploadInput{
				Bucket: aws.String("example-files"), Key: aws.String("t/notes/receipt"), UploadId: aws.String("upload-1"),
			})

			if abortErr != nil || !errors.Is(completeErr, context.Canceled) {
				t.Errorf("after the context ended the abort answered %v and the completion %v, want nil and %v",
					abortErr, completeErr, context.Canceled)
			}
			if took := log.requests(); len(took) != 1 || took[0] != "DELETE upload-1" {
				t.Errorf("the store took %q, want the one abort of upload-1", took)
			}
		})
	}
}

func TestTheAbortOfAnUploadStopsAtTheCleanupTimeout(t *testing.T) {
	t.Parallel()

	address, _ := storeAt(t, answerLate)
	client := clientAt(address, shortCleanup)

	_, err := client.AbortMultipartUpload(endedContext(t), abortInput())

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("the abort of an upload the store answers late error = %v, want %v", err, context.DeadlineExceeded)
	}
}

func TestOpenBoundsTheAbortOfAnS3UploadByTheCleanupTimeout(t *testing.T) {
	t.Parallel()

	address, _ := storeAt(t, answerLate)
	bucket, err := Open(t.Context(), Settings{Driver: S3, CleanupTimeout: shortCleanup, S3: exampleS3(address)}, nil)
	if err != nil {
		t.Fatalf("Open() error = %v, want the S3 bucket", err)
	}
	t.Cleanup(func() { _ = bucket.Close() })
	var client *s3.Client
	if !bucket.As(&client) {
		t.Fatal("As(*s3.Client) = false, want the client of the bucket")
	}

	_, err = client.AbortMultipartUpload(endedContext(t), abortInput())

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("the abort through a bucket opened with a cleanup timeout of %v error = %v, want %v",
			shortCleanup, err, context.DeadlineExceeded)
	}
}
