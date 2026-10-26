// SPDX-License-Identifier: Elastic-2.0

// Package storage keeps the plugins' files in the bucket the settings name, apart for each workspace and plugin.
package storage

import (
	"cmp"
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"gocloud.dev/blob"
	"gocloud.dev/blob/s3blob"

	"github.com/gopherium/framework/postgresblob"
)

// Driver names where the files live.
type Driver string

const (
	// Postgres keeps the files in the tables of the storage schema.
	Postgres Driver = "postgres"
	// S3 keeps the files in a bucket of an S3 compatible store.
	S3 Driver = "s3"
)

// Checksum names when the S3 client adds a checksum to a request and checks the one an answer carries.
type Checksum string

const (
	// WhenSupported adds and checks a checksum on every request that takes one.
	WhenSupported Checksum = "when_supported"
	// WhenRequired adds and checks a checksum only on the requests that need one.
	WhenRequired Checksum = "when_required"
)

// Schema is the PostgreSQL schema that holds the files of the postgres driver.
const Schema = "storage"

// DefaultMaxBytes is the most bytes one file holds when the settings name no other, 25 MiB.
const DefaultMaxBytes = 25 << 20

// DefaultChunkBytes is the most bytes one row of the postgres driver holds when the settings name no other.
const DefaultChunkBytes = postgresblob.DefaultChunkSize

// MaxChunkBytes is the most bytes the settings may let one row of the postgres driver hold.
const MaxChunkBytes = postgresblob.MaxChunkSize

// DefaultCleanupTimeout bounds the removal of a failed upload when the settings name no other bound.
const DefaultCleanupTimeout = postgresblob.DefaultCleanupTimeout

// DefaultSweepInterval is how often the sweeper runs when the settings name no other interval.
const DefaultSweepInterval = time.Hour

// DefaultSweepAge is how long an unfinished upload waits before the sweeper deletes it, unless the settings say.
const DefaultSweepAge = 24 * time.Hour

// DefaultSweepBatch is the most unfinished uploads one sweep deletes when the settings name no other count.
const DefaultSweepBatch = 1000

// Settings names the bucket the files live in and the bounds of each file.
type Settings struct {
	// Driver names where the files live.
	Driver Driver
	// MaxBytes is the most bytes one file holds.
	MaxBytes int64
	// ChunkBytes is the most bytes one row of the postgres driver holds.
	ChunkBytes int
	// CleanupTimeout bounds the removal of an upload that failed, its rows under postgres or its parts under s3.
	CleanupTimeout time.Duration
	// SweepInterval is how often the sweeper deletes unfinished uploads.
	SweepInterval time.Duration
	// SweepAge is how long an unfinished upload goes without a new row before the sweeper deletes it.
	SweepAge time.Duration
	// SweepBatch is the most unfinished uploads one sweep deletes.
	SweepBatch int
	// S3 names the bucket of the s3 driver.
	S3 S3Settings
}

// S3Settings names the bucket of an S3 compatible store and how the client reaches it.
type S3Settings struct {
	// Endpoint is the address of the store, the AWS address of the region when empty.
	Endpoint string
	// Region is the region the client signs its requests for.
	Region string
	// Bucket names the bucket the files live in.
	Bucket string
	// AccessKeyID names the key the client signs with.
	AccessKeyID string
	// SecretAccessKey is the secret of that key.
	SecretAccessKey string
	// PathStyle puts the bucket in the path of each request instead of its host name.
	PathStyle bool
	// Checksum names when the client adds and checks checksums.
	Checksum Checksum
}

// Open returns the bucket the settings name, the tables of the postgres driver reached through pool.
func Open(ctx context.Context, settings Settings, pool *pgxpool.Pool) (*blob.Bucket, error) {
	bucket, err := openDriver(ctx, settings, pool)
	if err != nil {
		return nil, fmt.Errorf("storage: open the %s bucket: %w", settings.Driver, err)
	}
	return bucket, nil
}

// openDriver returns the bucket of the driver the settings name.
func openDriver(ctx context.Context, settings Settings, pool *pgxpool.Pool) (*blob.Bucket, error) {
	if settings.Driver == S3 {
		client, options := s3Config(settings.S3, settings.CleanupTimeout)
		return s3blob.OpenBucket(ctx, s3.New(client), settings.S3.Bucket, &options)
	}
	return postgresblob.OpenBucket(ctx, pool, &postgresblob.Options{
		Schema:         Schema,
		ChunkSize:      settings.ChunkBytes,
		CleanupTimeout: settings.CleanupTimeout,
	})
}

// s3Config returns the client options and the bucket options the S3 settings name, with aborts bounded by cleanup.
func s3Config(settings S3Settings, cleanup time.Duration) (s3.Options, s3blob.Options) {
	request, response := aws.RequestChecksumCalculationWhenSupported, aws.ResponseChecksumValidationWhenSupported
	if settings.Checksum == WhenRequired {
		request, response = aws.RequestChecksumCalculationWhenRequired, aws.ResponseChecksumValidationWhenRequired
	}
	keys := credentials.NewStaticCredentialsProvider(settings.AccessKeyID, settings.SecretAccessKey, "")
	abort := detachedAbort{limit: cmp.Or(cleanup, DefaultCleanupTimeout)}
	client := s3.Options{
		Region:                     settings.Region,
		UsePathStyle:               settings.PathStyle,
		Credentials:                keys,
		RequestChecksumCalculation: request,
		ResponseChecksumValidation: response,
		APIOptions:                 []func(*middleware.Stack) error{abort.addTo},
	}
	if settings.Endpoint != "" {
		client.BaseEndpoint = aws.String(settings.Endpoint)
	}
	return client, s3blob.Options{RequestChecksumCalculation: request}
}

// abortStepID names the step of detachedAbort in the stack of each request.
const abortStepID = "storage.detachedAbort"

// detachedAbort sends the abort of an upload in parts even once the context of the upload ended, within limit.
type detachedAbort struct {
	// limit bounds how long the abort may take.
	limit time.Duration
}

// HandleInitialize sends an abort on a context of its own bounded by limit, and every other request on ctx.
func (d detachedAbort) HandleInitialize(
	ctx context.Context, in middleware.InitializeInput, next middleware.InitializeHandler,
) (middleware.InitializeOutput, middleware.Metadata, error) {
	if _, ok := in.Parameters.(*s3.AbortMultipartUploadInput); !ok {
		return next.HandleInitialize(ctx, in)
	}
	detached, cancel := context.WithTimeout(context.WithoutCancel(ctx), d.limit)
	defer cancel()
	return next.HandleInitialize(detached, in)
}

// addTo puts the step first in the stack of a request.
func (d detachedAbort) addTo(stack *middleware.Stack) error {
	return stack.Initialize.Add(middleware.InitializeMiddlewareFunc(abortStepID, d.HandleInitialize), middleware.Before)
}
