// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"

	"github.com/gopherium/framework/gonsole/testkit"

	"github.com/gopherium/alphone/internal/storage"
	"github.com/gopherium/alphone/sdk"
)

// receiptText is the body the file tests save.
const receiptText = "Receipt 184467235 for Maria Perez"

// sweepBudget bounds how long a test waits for serve to delete an upload that never finished.
const sweepBudget = 10 * time.Second

// ageUnfinished makes every upload that never finished look untouched for the interval its one argument names.
const ageUnfinished = "UPDATE storage.objects SET modified_at = now() - $1::interval WHERE state = 'pending'"

// unfinishedUpload leaves an upload longer than one row unfinished on pool, untouched past the default sweep age.
func unfinishedUpload(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	bucket, err := storage.Open(t.Context(), storage.Settings{Driver: storage.Postgres, ChunkBytes: 4}, pool)
	if err != nil {
		t.Fatalf("Open() error = %v, want the postgres bucket", err)
	}
	t.Cleanup(func() { _ = bucket.Close() })
	writer, err := bucket.NewWriter(t.Context(), "uploads/stale", &blob.WriterOptions{ContentType: "text/plain"})
	if err != nil {
		t.Fatalf("NewWriter() error = %v, want a writer", err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	if _, err := writer.Write([]byte(receiptText)); err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}
	if _, err := pool.Exec(t.Context(), ageUnfinished, 2*storage.DefaultSweepAge); err != nil {
		t.Fatalf("ageing the unfinished upload: %v", err)
	}
}

// fileTakingPlugin records the file store the host hands it.
type fileTakingPlugin struct {
	inertPlugin
	received sdk.FileStore
}

// UseFileStore keeps the handed store.
func (p *fileTakingPlugin) UseFileStore(store sdk.FileStore) {
	p.received = store
}

// migratedPostgres returns a fresh database every schema step migrated.
func migratedPostgres(t *testing.T) string {
	t.Helper()
	databaseURL := barePostgres(t)
	if err := migrate(t.Context(), databaseURL); err != nil {
		t.Fatalf("migrate() error = %v, want nil", err)
	}
	return databaseURL
}

// missingBucketStore returns the address of an S3 store that holds no bucket at all.
func missingBucketStore(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, "<Error><Code>NoSuchBucket</Code><Message>no such bucket</Message></Error>")
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// composedFileTakers composes the host over a migrated database with one file taking plugin under each id.
func composedFileTakers(t *testing.T, ids ...string) []*fileTakingPlugin {
	t.Helper()
	taking := make([]*fileTakingPlugin, 0, len(ids))
	registered := make([]sdk.Plugin, 0, len(ids))
	for _, id := range ids {
		plugin := &fileTakingPlugin{inertPlugin: inertPlugin{id: id}}
		taking, registered = append(taking, plugin), append(registered, plugin)
	}
	cfg := composeConfigOf(t, map[string]string{"ALPHONE_DATABASE_URL": migratedPostgres(t)})
	built, err := compose(t.Context(), cfg, func(sdk.Deps) ([]sdk.Plugin, error) { return registered, nil })
	if err != nil {
		t.Fatalf("compose() error = %v, want nil", err)
	}
	abandonAtEnd(t, built)
	return taking
}

// openedText returns the text store opens under id, failing the test when it opens nothing.
func openedText(t *testing.T, store sdk.FileStore, id uuid.UUID) string {
	t.Helper()
	reader, err := store.Open(t.Context(), id)
	if err != nil {
		t.Fatalf("Open() error = %v, want the file", err)
	}
	defer func() { _ = reader.Close() }()
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("reading the opened file: %v", err)
	}
	return string(body)
}

func TestComposeHandsEachFileTakingPluginAStoreOfItsOwn(t *testing.T) {
	t.Parallel()

	taking := composedFileTakers(t, "notes", "mail")
	notes, mail := taking[0].received, taking[1].received

	if notes == nil || mail == nil {
		t.Fatalf("the plugins received %v and %v, want a file store each", notes, mail)
	}
	file, err := notes.Save(t.Context(), "application/pdf", strings.NewReader(receiptText))
	if err != nil {
		t.Fatalf("Save() through the notes store error = %v, want the file stored", err)
	}
	if _, err := mail.Open(t.Context(), file.ID); !errors.Is(err, sdk.ErrFileNotFound) {
		t.Errorf("Open() through the mail store error = %v, want %v", err, sdk.ErrFileNotFound)
	}
	if body := openedText(t, notes, file.ID); body != receiptText {
		t.Errorf("the notes store opened %q, want %q", body, receiptText)
	}
}

func TestComposeOpensNoBucketBeforeAFileIsUsed(t *testing.T) {
	t.Parallel()

	taking := &fileTakingPlugin{inertPlugin: inertPlugin{id: "notes"}}
	env := map[string]string{"ALPHONE_DATABASE_URL": unreachableDatabaseURL}

	built, err := compose(t.Context(), composeConfigOf(t, env), func(sdk.Deps) ([]sdk.Plugin, error) {
		return []sdk.Plugin{taking}, nil
	})

	if err != nil {
		t.Fatalf("compose() of the postgres driver over an unreachable database error = %v, want the bucket left unopened",
			err)
	}
	abandonAtEnd(t, built)
	if taking.received == nil {
		t.Error("the plugin received no file store, want one handed before any bucket opens")
	}
}

func TestAbandonClosesTheBucketComposeOpened(t *testing.T) {
	t.Parallel()

	built, err := compose(t.Context(), composeConfigOf(t, filledSettings()), registeringNothing)
	if err != nil {
		t.Fatalf("compose() error = %v, want nil", err)
	}
	bucket, err := built.files.Bucket(t.Context())
	if err != nil {
		t.Fatalf("Bucket() error = %v, want the s3 bucket opened without reaching the store", err)
	}

	if err := abandon(t.Context(), built, pluginStopGrace); err != nil {
		t.Fatalf("abandon() error = %v, want nil", err)
	}

	if err := bucket.Close(); gcerrors.Code(err) != gcerrors.FailedPrecondition {
		t.Errorf("Close() of the bucket after abandon() error = %v, want the bucket closed already", err)
	}
}

func TestServeDeletesTheUploadsThatNeverFinished(t *testing.T) {
	t.Parallel()

	databaseURL := migratedPostgres(t)
	pool := testPool(t, databaseURL)
	unfinishedUpload(t, pool)
	addr := testkit.FreeAddr(t)
	ctx, cancel := context.WithCancel(t.Context())
	runErr := make(chan error, 1)
	go func() {
		env := map[string]string{"ALPHONE_DATABASE_URL": databaseURL, "ALPHONE_ADDR": addr}
		runErr <- run(ctx, testkit.Getenv(env), io.Discard, registerPlugins)
	}()
	t.Cleanup(func() { stopRun(t, cancel, runErr) })
	waitForServer(t, "http://"+addr)

	deadline := time.Now().Add(sweepBudget)
	for countRows(t, pool, "storage.objects") != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("waited %v for serve to delete the upload that never finished", sweepBudget)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRunStopsWhenTheBucketIsNotAccessible(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"ALPHONE_DATABASE_URL":                 testDatabaseURL(t),
		"ALPHONE_STORAGE_DRIVER":               "s3",
		"ALPHONE_STORAGE_S3_ENDPOINT":          missingBucketStore(t),
		"ALPHONE_STORAGE_S3_REGION":            "garage",
		"ALPHONE_STORAGE_S3_BUCKET":            "example-files",
		"ALPHONE_STORAGE_S3_ACCESS_KEY_ID":     "GKexample",
		"ALPHONE_STORAGE_S3_SECRET_ACCESS_KEY": "example-secret",
		"ALPHONE_STORAGE_S3_PATH_STYLE":        "true",
	}
	plugin := newHoldingPlugin()

	err := run(t.Context(), testkit.Getenv(env), io.Discard, func(sdk.Deps) ([]sdk.Plugin, error) {
		return []sdk.Plugin{plugin}, nil
	})

	if err == nil || !strings.Contains(err.Error(), "the s3 bucket") {
		t.Fatalf("run() error = %v, want the s3 bucket that is not accessible named", err)
	}
	select {
	case <-plugin.stopped:
	default:
		t.Error("the plugin was never stopped, want run to stop the host before it gives up")
	}
}
