// SPDX-License-Identifier: Elastic-2.0

package storage

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"gocloud.dev/blob"

	"github.com/gopherium/alphone/sdk"
)

// Files is the host's one bucket, opened on first use, and the file store of each plugin over it.
type Files struct {
	// settings name the bucket and the cap of each file.
	settings Settings
	// pool reaches the tables of the postgres driver.
	pool *pgxpool.Pool
	// mu guards bucket.
	mu sync.Mutex
	// bucket is the bucket once a call opened it.
	bucket *blob.Bucket
}

// NewFiles returns the files the settings name over pool, opening no bucket yet.
func NewFiles(settings Settings, pool *pgxpool.Pool) *Files {
	return &Files{settings: settings, pool: pool}
}

// Bucket returns the bucket, opening it when no call opened it before.
func (f *Files) Bucket(ctx context.Context) (*blob.Bucket, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.bucket != nil {
		return f.bucket, nil
	}
	bucket, err := Open(ctx, f.settings, f.pool)
	if err != nil {
		return nil, err
	}
	f.bucket = bucket
	return bucket, nil
}

// Store returns the file store of the plugin pluginID over the bucket.
func (f *Files) Store(pluginID string) sdk.FileStore {
	return fileStore{bucket: f.Bucket, plugin: pluginID, maxBytes: f.settings.MaxBytes}
}

// Check opens the bucket and returns an error naming the driver unless the bucket is accessible.
func (f *Files) Check(ctx context.Context) error {
	bucket, err := f.Bucket(ctx)
	if err != nil {
		return err
	}
	accessible, err := bucket.IsAccessible(ctx)
	if err != nil {
		return fmt.Errorf("storage: reach the %s bucket: %w", f.settings.Driver, err)
	}
	if !accessible {
		return fmt.Errorf("storage: the %s bucket is not there, check the storage settings", f.settings.Driver)
	}
	return nil
}

// Close closes the bucket when a call opened it.
func (f *Files) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.bucket == nil {
		return nil
	}
	return f.bucket.Close()
}
