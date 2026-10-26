// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/gopherium/framework/gonsole/testkit"

	"github.com/gopherium/alphone/internal/storage"
)

// refusedSetting is one storage setting the server refuses and the error that names it.
type refusedSetting struct {
	driver string
	key    string
	raw    string
	want   string
}

// refusedSettings returns every storage setting the server refuses, by the case it covers.
func refusedSettings() map[string]refusedSetting {
	return map[string]refusedSetting{
		"a driver AlphOne does not ship": {
			key: "ALPHONE_STORAGE_DRIVER", raw: "disk",
			want: `ALPHONE_STORAGE_DRIVER: must be postgres or s3, got "disk"`,
		},
		"an S3 bucket without a region": {
			driver: "s3", key: "ALPHONE_STORAGE_S3_REGION",
			want: "ALPHONE_STORAGE_S3_REGION is required when ALPHONE_STORAGE_DRIVER is s3",
		},
		"an S3 bucket without a name": {
			driver: "s3", key: "ALPHONE_STORAGE_S3_BUCKET",
			want: "ALPHONE_STORAGE_S3_BUCKET is required when ALPHONE_STORAGE_DRIVER is s3",
		},
		"an S3 bucket without an access key": {
			driver: "s3", key: "ALPHONE_STORAGE_S3_ACCESS_KEY_ID",
			want: "ALPHONE_STORAGE_S3_ACCESS_KEY_ID is required when ALPHONE_STORAGE_DRIVER is s3",
		},
		"an S3 bucket without a secret": {
			driver: "s3", key: "ALPHONE_STORAGE_S3_SECRET_ACCESS_KEY",
			want: "ALPHONE_STORAGE_S3_SECRET_ACCESS_KEY is required when ALPHONE_STORAGE_DRIVER is s3",
		},
		"an S3 endpoint on ftp": {
			driver: "s3", key: "ALPHONE_STORAGE_S3_ENDPOINT", raw: "ftp://files.example.com",
			want: `ALPHONE_STORAGE_S3_ENDPOINT: must be an http or https address, got "ftp://files.example.com"`,
		},
		"an S3 endpoint without a host": {
			driver: "s3", key: "ALPHONE_STORAGE_S3_ENDPOINT", raw: "https:///files",
			want: `ALPHONE_STORAGE_S3_ENDPOINT: must be an http or https address, got "https:///files"`,
		},
		"an S3 endpoint that is no address": {
			driver: "s3", key: "ALPHONE_STORAGE_S3_ENDPOINT", raw: "https://files.example.com:port",
			want: `ALPHONE_STORAGE_S3_ENDPOINT: must be an http or https address, got "https://files.example.com:port"`,
		},
		"an S3 path style in words": {
			driver: "s3", key: "ALPHONE_STORAGE_S3_PATH_STYLE", raw: "sometimes",
			want: `ALPHONE_STORAGE_S3_PATH_STYLE: must be true or false, got "sometimes"`,
		},
		"an S3 checksum AlphOne does not know": {
			driver: "s3", key: "ALPHONE_STORAGE_S3_CHECKSUM", raw: "always",
			want: `ALPHONE_STORAGE_S3_CHECKSUM: must be when_supported or when_required, got "always"`,
		},
		"an S3 bucket under the postgres driver": {
			driver: "postgres", key: "ALPHONE_STORAGE_S3_BUCKET", raw: "example-files",
			want: "ALPHONE_STORAGE_S3_BUCKET is set but ALPHONE_STORAGE_DRIVER is not s3",
		},
		"an S3 checksum under the default driver": {
			key: "ALPHONE_STORAGE_S3_CHECKSUM", raw: "when_required",
			want: "ALPHONE_STORAGE_S3_CHECKSUM is set but ALPHONE_STORAGE_DRIVER is not s3",
		},
		"no bytes a file": {
			key: "ALPHONE_STORAGE_MAX_BYTES", raw: "0",
			want: `ALPHONE_STORAGE_MAX_BYTES: must stand above zero, got "0"`,
		},
		"a row past the largest chunk": {
			key: "ALPHONE_STORAGE_CHUNK_BYTES", raw: "268435457",
			want: `ALPHONE_STORAGE_CHUNK_BYTES: must stand at or below 268435456, got "268435457"`,
		},
		"a cleanup in words": {
			key: "ALPHONE_STORAGE_CLEANUP_TIMEOUT", raw: "soon",
			want: `ALPHONE_STORAGE_CLEANUP_TIMEOUT: must be a duration like 30s, got "soon"`,
		},
		"no sweep interval": {
			key: "ALPHONE_STORAGE_SWEEP_INTERVAL", raw: "0s",
			want: `ALPHONE_STORAGE_SWEEP_INTERVAL: must stand above zero, got "0s"`,
		},
		"a sweep age in words": {
			key: "ALPHONE_STORAGE_SWEEP_AGE", raw: "a day",
			want: `ALPHONE_STORAGE_SWEEP_AGE: must be a duration like 30s, got "a day"`,
		},
		"no uploads a sweep": {
			key: "ALPHONE_STORAGE_SWEEP_BATCH", raw: "0",
			want: `ALPHONE_STORAGE_SWEEP_BATCH: must stand above zero, got "0"`,
		},
	}
}

// s3StorageSettings returns every setting of a bucket in an S3 compatible store, each one valid.
func s3StorageSettings() map[string]string {
	return map[string]string{
		"ALPHONE_DATABASE_URL":                 unreachableDatabaseURL,
		"ALPHONE_STORAGE_DRIVER":               "s3",
		"ALPHONE_STORAGE_S3_ENDPOINT":          "https://files.example.com",
		"ALPHONE_STORAGE_S3_REGION":            "garage",
		"ALPHONE_STORAGE_S3_BUCKET":            "example-files",
		"ALPHONE_STORAGE_S3_ACCESS_KEY_ID":     "GKexample",
		"ALPHONE_STORAGE_S3_SECRET_ACCESS_KEY": "example-secret",
		"ALPHONE_STORAGE_S3_PATH_STYLE":        "true",
		"ALPHONE_STORAGE_S3_CHECKSUM":          "when_required",
		"ALPHONE_STORAGE_MAX_BYTES":            "1048576",
		"ALPHONE_STORAGE_CHUNK_BYTES":          "65536",
		"ALPHONE_STORAGE_CLEANUP_TIMEOUT":      "3s",
		"ALPHONE_STORAGE_SWEEP_INTERVAL":       "15m",
		"ALPHONE_STORAGE_SWEEP_AGE":            "6h",
		"ALPHONE_STORAGE_SWEEP_BATCH":          "250",
	}
}

// settingsAround returns the settings that carry the refused one, every S3 setting filled under the s3 driver.
func settingsAround(refused refusedSetting) map[string]string {
	env := map[string]string{"ALPHONE_DATABASE_URL": unreachableDatabaseURL}
	switch refused.driver {
	case "s3":
		env = s3StorageSettings()
	case "postgres":
		env["ALPHONE_STORAGE_DRIVER"] = "postgres"
	}
	env[refused.key] = refused.raw
	return env
}

// storageOf returns the storage settings loadRunConfig reads from env, failing the test on an error.
func storageOf(t *testing.T, env map[string]string) storage.Settings {
	t.Helper()
	settings, err := loadRunConfig(testkit.Getenv(env))
	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	return settings.storage
}

// statedValue returns what the example file states for key, a duration written the way Go prints one.
func statedValue(named map[string]string, key string) string {
	if held, err := time.ParseDuration(named[key]); err == nil {
		return held.String()
	}
	return named[key]
}

func TestTheStorageSettingsFallBackToTheirDefaults(t *testing.T) {
	t.Parallel()

	got := storageOf(t, map[string]string{"ALPHONE_DATABASE_URL": unreachableDatabaseURL})

	want := storage.Settings{
		Driver: storage.Postgres, MaxBytes: 26214400, ChunkBytes: 1048576, CleanupTimeout: 10 * time.Second,
		SweepInterval: time.Hour, SweepAge: 24 * time.Hour, SweepBatch: 1000,
	}
	if got != want {
		t.Errorf("storage = %+v, want the defaults %+v", got, want)
	}
}

func TestTheStorageSettingsAreReadFromTheEnvironment(t *testing.T) {
	t.Parallel()

	got := storageOf(t, s3StorageSettings())

	want := storage.Settings{
		Driver: storage.S3, MaxBytes: 1048576, ChunkBytes: 65536, CleanupTimeout: 3 * time.Second,
		SweepInterval: 15 * time.Minute, SweepAge: 6 * time.Hour, SweepBatch: 250,
		S3: storage.S3Settings{
			Endpoint: "https://files.example.com", Region: "garage", Bucket: "example-files",
			AccessKeyID: "GKexample", SecretAccessKey: "example-secret", PathStyle: true, Checksum: storage.WhenRequired,
		},
	}
	if got != want {
		t.Errorf("storage = %+v, want every value the environment named %+v", got, want)
	}
}

func TestAnS3BucketWithoutAnEndpointOrAChecksumTakesTheAWSDefaults(t *testing.T) {
	t.Parallel()

	env := s3StorageSettings()
	delete(env, "ALPHONE_STORAGE_S3_ENDPOINT")
	delete(env, "ALPHONE_STORAGE_S3_CHECKSUM")
	delete(env, "ALPHONE_STORAGE_S3_PATH_STYLE")

	got := storageOf(t, env).S3

	want := storage.S3Settings{
		Region: "garage", Bucket: "example-files", AccessKeyID: "GKexample", SecretAccessKey: "example-secret",
		Checksum: storage.WhenSupported,
	}
	if got != want {
		t.Errorf("S3 = %+v, want no endpoint, no path style and the checksum when_supported %+v", got, want)
	}
}

func TestARefusedStorageSettingIsNamed(t *testing.T) {
	t.Parallel()

	for testName, refused := range refusedSettings() {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			_, err := loadRunConfig(testkit.Getenv(settingsAround(refused)))

			if err == nil || err.Error() != refused.want {
				t.Errorf("loadRunConfig() error = %v, want %q", err, refused.want)
			}
		})
	}
}

func TestTheStorageDefaultsAreTheOnesTheExampleFileStates(t *testing.T) {
	t.Parallel()

	named := exampleSettings(t)
	defaults := map[string]string{
		"ALPHONE_STORAGE_DRIVER":          string(storage.Postgres),
		"ALPHONE_STORAGE_MAX_BYTES":       fmt.Sprint(storage.DefaultMaxBytes),
		"ALPHONE_STORAGE_CHUNK_BYTES":     fmt.Sprint(storage.DefaultChunkBytes),
		"ALPHONE_STORAGE_CLEANUP_TIMEOUT": storage.DefaultCleanupTimeout.String(),
		"ALPHONE_STORAGE_SWEEP_INTERVAL":  storage.DefaultSweepInterval.String(),
		"ALPHONE_STORAGE_SWEEP_AGE":       storage.DefaultSweepAge.String(),
		"ALPHONE_STORAGE_SWEEP_BATCH":     fmt.Sprint(storage.DefaultSweepBatch),
		"ALPHONE_STORAGE_S3_PATH_STYLE":   "false",
		"ALPHONE_STORAGE_S3_CHECKSUM":     string(storage.WhenSupported),
	}

	for key, want := range defaults {
		if stated := statedValue(named, key); stated != want {
			t.Errorf("the example file states %s=%q, want the default %q", key, named[key], want)
		}
	}
}
