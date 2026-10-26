// SPDX-License-Identifier: Elastic-2.0

package storage

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"gocloud.dev/blob/s3blob"
)

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

			client, bucket := s3Config(tc.settings)

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
