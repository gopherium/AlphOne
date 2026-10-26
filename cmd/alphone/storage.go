// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"fmt"
	"net/url"

	"github.com/gopherium/framework/gonsole"

	"github.com/gopherium/alphone/internal/storage"
	"github.com/gopherium/alphone/sdk"
)

// s3SettingNames names every setting of the s3 driver, past the program prefix.
var s3SettingNames = []string{
	"STORAGE_S3_ENDPOINT", "STORAGE_S3_REGION", "STORAGE_S3_BUCKET", "STORAGE_S3_ACCESS_KEY_ID",
	"STORAGE_S3_SECRET_ACCESS_KEY", "STORAGE_S3_PATH_STYLE", "STORAGE_S3_CHECKSUM",
}

// loadStorageSettings reads where the files live and the bounds of each one.
func loadStorageSettings(env gonsole.Env) (storage.Settings, error) {
	driver, err := gonsole.Parse(env, "STORAGE_DRIVER", storage.Postgres, parseStorageDriver)
	if err != nil {
		return storage.Settings{}, err
	}
	settings, err := loadStorageBounds(env)
	if err != nil {
		return storage.Settings{}, err
	}
	if settings, err = loadSweepSettings(env, settings); err != nil {
		return storage.Settings{}, err
	}
	settings.Driver = driver
	settings.S3, err = loadS3Settings(env, driver)
	return settings, err
}

// loadStorageBounds reads the most bytes one file holds and the bounds of the rows of the postgres driver.
func loadStorageBounds(env gonsole.Env) (storage.Settings, error) {
	maxBytes, err := env.Count("STORAGE_MAX_BYTES", storage.DefaultMaxBytes)
	if err != nil {
		return storage.Settings{}, err
	}
	chunkBytes, err := env.Count("STORAGE_CHUNK_BYTES", storage.DefaultChunkBytes, gonsole.AtMost(storage.MaxChunkBytes))
	if err != nil {
		return storage.Settings{}, err
	}
	cleanup, err := env.Duration("STORAGE_CLEANUP_TIMEOUT", storage.DefaultCleanupTimeout)
	if err != nil {
		return storage.Settings{}, err
	}
	return storage.Settings{MaxBytes: int64(maxBytes), ChunkBytes: chunkBytes, CleanupTimeout: cleanup}, nil
}

// loadSweepSettings reads how often, how old and how many unfinished uploads the sweeper deletes into settings.
func loadSweepSettings(env gonsole.Env, settings storage.Settings) (storage.Settings, error) {
	interval, err := env.Duration("STORAGE_SWEEP_INTERVAL", storage.DefaultSweepInterval)
	if err != nil {
		return storage.Settings{}, err
	}
	age, err := env.Duration("STORAGE_SWEEP_AGE", storage.DefaultSweepAge)
	if err != nil {
		return storage.Settings{}, err
	}
	batch, err := env.Count("STORAGE_SWEEP_BATCH", storage.DefaultSweepBatch)
	if err != nil {
		return storage.Settings{}, err
	}
	settings.SweepInterval, settings.SweepAge, settings.SweepBatch = interval, age, batch
	return settings, nil
}

// loadS3Settings reads the bucket of the s3 driver, refusing any of its settings under another driver.
func loadS3Settings(env gonsole.Env, driver storage.Driver) (storage.S3Settings, error) {
	if driver != storage.S3 {
		for _, name := range s3SettingNames {
			if env.Value(name) != "" {
				return storage.S3Settings{}, fmt.Errorf("%s is set but %s is not s3", env.Key(name), env.Key("STORAGE_DRIVER"))
			}
		}
		return storage.S3Settings{}, nil
	}
	endpoint, err := gonsole.Parse(env, "STORAGE_S3_ENDPOINT", "", parseS3Endpoint)
	if err != nil {
		return storage.S3Settings{}, err
	}
	pathStyle, err := env.Flag("STORAGE_S3_PATH_STYLE", false)
	if err != nil {
		return storage.S3Settings{}, err
	}
	checksum, err := gonsole.Parse(env, "STORAGE_S3_CHECKSUM", storage.WhenSupported, parseS3Checksum)
	if err != nil {
		return storage.S3Settings{}, err
	}
	held := storage.S3Settings{Endpoint: endpoint, PathStyle: pathStyle, Checksum: checksum}
	return requireS3Keys(env, held)
}

// requireS3Keys reads the region, the bucket and both keys into held, each one required under the s3 driver.
func requireS3Keys(env gonsole.Env, held storage.S3Settings) (storage.S3Settings, error) {
	required := []struct {
		name string
		into *string
	}{
		{"STORAGE_S3_REGION", &held.Region},
		{"STORAGE_S3_BUCKET", &held.Bucket},
		{"STORAGE_S3_ACCESS_KEY_ID", &held.AccessKeyID},
		{"STORAGE_S3_SECRET_ACCESS_KEY", &held.SecretAccessKey},
	}
	for _, setting := range required {
		*setting.into = env.Value(setting.name)
		if *setting.into == "" {
			return storage.S3Settings{}, fmt.Errorf("%s is required when %s is s3",
				env.Key(setting.name), env.Key("STORAGE_DRIVER"))
		}
	}
	return held, nil
}

// parseStorageDriver reads where the files live.
func parseStorageDriver(value string) (storage.Driver, error) {
	switch driver := storage.Driver(value); driver {
	case storage.Postgres, storage.S3:
		return driver, nil
	}
	return "", fmt.Errorf("must be postgres or s3, got %q", value)
}

// parseS3Checksum reads when the S3 client adds and checks checksums.
func parseS3Checksum(value string) (storage.Checksum, error) {
	switch checksum := storage.Checksum(value); checksum {
	case storage.WhenSupported, storage.WhenRequired:
		return checksum, nil
	}
	return "", fmt.Errorf("must be when_supported or when_required, got %q", value)
}

// parseS3Endpoint reads the address of an S3 compatible store.
func parseS3Endpoint(value string) (string, error) {
	if !isWebAddress(value) {
		return "", fmt.Errorf("must be an http or https address, got %q", value)
	}
	return value, nil
}

// isWebAddress reports whether value reads as the address of a host over http or https.
func isWebAddress(value string) bool {
	held, err := url.Parse(value)
	if err != nil || held.Host == "" {
		return false
	}
	return held.Scheme == "http" || held.Scheme == "https"
}

// wireFileStores hands every plugin taking a file store a store of its own over files.
func wireFileStores(registered []sdk.Plugin, files *storage.Files) {
	for _, plugin := range registered {
		if consumer, ok := plugin.(sdk.FileStoreConsumer); ok {
			consumer.UseFileStore(files.Store(plugin.ID()))
		}
	}
}
