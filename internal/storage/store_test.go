// SPDX-License-Identifier: Elastic-2.0

package storage_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/google/uuid"
	"gocloud.dev/blob"
	"gocloud.dev/blob/driver"
	"gocloud.dev/blob/memblob"
	"gocloud.dev/gcerrors"

	"github.com/gopherium/alphone/internal/storage"
	"github.com/gopherium/alphone/sdk"
)

// receiptText is the body most tests save.
const receiptText = "Receipt 184467235 for Maria Perez"

// testCap is the most bytes one file holds in the memory tests.
const testCap = 64

// errBrokenBody is the failure a broken body answers.
var errBrokenBody = errors.New("the body broke")

// errNoDetails is the failure of a bucket that cannot read the details of a file.
var errNoDetails = errors.New("the details cannot be read")

// errHalfWrite is the failure of a write the store took only part of.
var errHalfWrite = errors.New("the write broke part way")

// acme is the workspace that saves the files the tests open.
var acme = uuid.MustParse("01900000-0000-7000-8000-000000000001")

// globex is a workspace that saved none of the files the tests open.
var globex = uuid.MustParse("01900000-0000-7000-8000-000000000002")

// caller is one test calling a file store for one workspace.
type caller struct {
	// t is the test that calls.
	t *testing.T
	// ctx serves the workspace the test calls for.
	ctx context.Context
}

// as returns the test t calling for the workspace id.
func as(t *testing.T, id uuid.UUID) caller {
	t.Helper()
	return caller{t: t, ctx: sdk.WithTenant(t.Context(), id)}
}

// save saves body under the content type through store, failing the test unless it is stored.
func (c caller) save(store sdk.FileStore, contentType, body string) sdk.StoredFile {
	c.t.Helper()
	file, err := store.Save(c.ctx, contentType, strings.NewReader(body))
	if err != nil {
		c.t.Fatalf("Save() error = %v, want the file stored", err)
	}
	return file
}

// open opens the file id through store and returns its bytes, failing the test when it cannot.
func (c caller) open(store sdk.FileStore, id uuid.UUID) string {
	c.t.Helper()
	reader, err := store.Open(c.ctx, id)
	if err != nil || reader == nil {
		c.t.Fatalf("Open() = %v, %v, want a reader of the file", reader, err)
	}
	defer func() { _ = reader.Close() }()
	body, err := io.ReadAll(reader)
	if err != nil {
		c.t.Fatalf("reading the opened file: %v", err)
	}
	return string(body)
}

// memoryBucket returns an empty bucket in memory, closed when the test ends.
func memoryBucket(t *testing.T) *blob.Bucket {
	t.Helper()
	bucket := memblob.OpenBucket(nil)
	t.Cleanup(func() { _ = bucket.Close() })
	return bucket
}

// keysIn returns every key the bucket holds.
func keysIn(t *testing.T, bucket *blob.Bucket) []string {
	t.Helper()
	var keys []string
	iter := bucket.List(nil)
	for {
		object, err := iter.Next(t.Context())
		if errors.Is(err, io.EOF) {
			return keys
		}
		if err != nil {
			t.Fatalf("List().Next() error = %v, want an object or io.EOF", err)
		}
		keys = append(keys, object.Key)
	}
}

// storedAttributes returns the details the bucket keeps under its one key, failing the test unless it holds one.
func storedAttributes(t *testing.T, bucket *blob.Bucket) *blob.Attributes {
	t.Helper()
	keys := keysIn(t, bucket)
	if len(keys) != 1 {
		t.Fatalf("the bucket holds %v, want the one key of the save", keys)
	}
	attributes, err := bucket.Attributes(t.Context(), keys[0])
	if err != nil {
		t.Fatalf("Attributes() error = %v, want the details of the saved file", err)
	}
	return attributes
}

// shaText returns the lower case hex SHA-256 of body.
func shaText(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

// mentions reports whether err is an error whose text holds text.
func mentions(err error, text string) bool {
	return err != nil && strings.Contains(err.Error(), text)
}

// withoutTime returns file with its StoredAt left out.
func withoutTime(file sdk.StoredFile) sdk.StoredFile {
	file.StoredAt = time.Time{}
	return file
}

// storedAround reports whether at falls within a second of the span from before to now.
func storedAround(at, before time.Time) bool {
	return !at.Before(before.Add(-time.Second)) && !at.After(time.Now().Add(time.Second))
}

// detailLessBucket holds receiptText under every key and cannot read the details of any.
type detailLessBucket struct {
	driver.Bucket
}

// Attributes answers that the details cannot be read.
func (detailLessBucket) Attributes(context.Context, string) (*driver.Attributes, error) {
	return nil, errNoDetails
}

// NewRangeReader returns a reader of receiptText.
func (detailLessBucket) NewRangeReader(context.Context, string, int64, int64, *driver.ReaderOptions) (
	driver.Reader, error) {
	return textReader{Reader: strings.NewReader(receiptText)}, nil
}

// ErrorCode reads every failure as unknown.
func (detailLessBucket) ErrorCode(error) gcerrors.ErrorCode {
	return gcerrors.Unknown
}

// Close closes nothing.
func (detailLessBucket) Close() error {
	return nil
}

// textReader reads a text held in memory as the reader of a driver.
type textReader struct {
	*strings.Reader
}

// Close closes nothing.
func (textReader) Close() error {
	return nil
}

// Attributes answers the size of the text.
func (r textReader) Attributes() *driver.ReaderAttributes {
	return &driver.ReaderAttributes{ContentType: "text/plain", Size: r.Size()}
}

// As answers false for every type.
func (textReader) As(any) bool {
	return false
}

// halfWriteBucket hands out writers that fail part way and keep their file on Close unless the write was aborted.
type halfWriteBucket struct {
	driver.Bucket
	// kept holds the keys a writer stored.
	kept []string
}

// NewTypedWriter returns a writer of key that fails part way.
func (b *halfWriteBucket) NewTypedWriter(ctx context.Context, key, _ string, _ *driver.WriterOptions) (
	driver.Writer, error) {
	return &halfWriter{ctx: ctx, key: key, bucket: b}, nil
}

// ErrorCode reads every failure as unknown.
func (*halfWriteBucket) ErrorCode(error) gcerrors.ErrorCode {
	return gcerrors.Unknown
}

// Close closes nothing.
func (*halfWriteBucket) Close() error {
	return nil
}

// exhaustedBucket is a halfWriteBucket whose failures read as a database out of room, such as one out of disk.
type exhaustedBucket struct {
	*halfWriteBucket
}

// ErrorCode reads every failure as resource exhausted.
func (exhaustedBucket) ErrorCode(error) gcerrors.ErrorCode {
	return gcerrors.ResourceExhausted
}

// halfWriter takes one byte of a write and then fails, as a store whose connection drops does.
type halfWriter struct {
	// ctx aborts the write once it ends.
	ctx context.Context
	// key is the key the file goes under.
	key string
	// bucket records the key once the file is kept.
	bucket *halfWriteBucket
}

// Write takes the first byte of p and answers errHalfWrite.
func (*halfWriter) Write(p []byte) (int, error) {
	return min(len(p), 1), errHalfWrite
}

// Close keeps the file under its key unless its context ended.
func (w *halfWriter) Close() error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	w.bucket.kept = append(w.bucket.kept, w.key)
	return nil
}

func TestASavedFileOpensWithItsBytesAndSaveAnswersItsDetails(t *testing.T) {
	t.Parallel()

	store := storage.NewFileStore(memoryBucket(t), "notes", testCap)
	owner := as(t, acme)
	before := time.Now()

	file := owner.save(store, "application/pdf", receiptText)
	body := owner.open(store, file.ID)

	if body != receiptText {
		t.Errorf("opened body = %q, want %q", body, receiptText)
	}
	want := sdk.StoredFile{ID: file.ID, Size: int64(len(receiptText)), ContentType: "application/pdf",
		SHA256: shaText(receiptText)}
	if got := withoutTime(file); got != want {
		t.Errorf("Save() = %+v, want %+v", got, want)
	}
	if !storedAround(file.StoredAt, before) {
		t.Errorf("Save() StoredAt = %v, want the time of the save", file.StoredAt)
	}
	if file.ID.Version() != 7 {
		t.Errorf("the id %v is version %d, want a version 7 UUID", file.ID, file.ID.Version())
	}
}

func TestOpenReadsTheFileWithoutAskingForItsDetails(t *testing.T) {
	t.Parallel()

	bucket := blob.NewBucket(detailLessBucket{})
	t.Cleanup(func() { _ = bucket.Close() })
	store := storage.NewFileStore(bucket, "notes", testCap)

	reader, err := store.Open(as(t, acme).ctx, uuid.Must(uuid.NewV7()))

	if err != nil || reader == nil {
		t.Fatalf("Open() = %v, %v, want the file read without its details", reader, err)
	}
	defer func() { _ = reader.Close() }()
	if body, err := io.ReadAll(reader); err != nil || string(body) != receiptText {
		t.Errorf("read %q, %v, want %q", body, err, receiptText)
	}
}

func TestAFileWithoutAContentTypeIsStoredAsOctetStream(t *testing.T) {
	t.Parallel()

	bucket := memoryBucket(t)
	store := storage.NewFileStore(bucket, "notes", testCap)

	file := as(t, acme).save(store, "", "<html><script>alert(1)</script></html>")

	stored := storedAttributes(t, bucket).ContentType
	if file.ContentType != "application/octet-stream" || stored != file.ContentType {
		t.Errorf("Save() answered %q and the bucket keeps %q, want application/octet-stream with nothing sniffed",
			file.ContentType, stored)
	}
}

func TestAMalformedContentTypeIsRefusedBeforeAnythingIsStored(t *testing.T) {
	t.Parallel()

	for _, contentType := range []string{"image/png; =", "inline"} {
		bucket := memoryBucket(t)
		store := storage.NewFileStore(bucket, "notes", testCap)

		_, err := store.Save(as(t, acme).ctx, contentType, strings.NewReader(receiptText))

		if !mentions(err, `"`+contentType+`"`) {
			t.Errorf("Save() under %q error = %v, want the malformed content type named", contentType, err)
		}
		if keys := keysIn(t, bucket); len(keys) != 0 {
			t.Errorf("the bucket holds %v after a save under %q, want nothing stored", keys, contentType)
		}
	}
}

func TestSaveAnswersAndStoresTheContentTypeInOneForm(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		given string
		want  string
	}{
		"a type in mixed case": {given: "Text/Plain", want: "text/plain"},
		"a quoted parameter":   {given: `text/plain; charset="utf-8"`, want: "text/plain; charset=utf-8"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			bucket := memoryBucket(t)
			store := storage.NewFileStore(bucket, "notes", testCap)

			file := as(t, acme).save(store, tc.given, receiptText)

			stored := storedAttributes(t, bucket).ContentType
			if file.ContentType != tc.want || stored != tc.want {
				t.Errorf("Save() answered %q and the bucket keeps %q, want both %q", file.ContentType, stored, tc.want)
			}
		})
	}
}

func TestTheLargestCapStoresTheWholeBody(t *testing.T) {
	t.Parallel()

	store := storage.NewFileStore(memoryBucket(t), "notes", math.MaxInt64)
	owner := as(t, acme)

	file := owner.save(store, "text/plain", receiptText)
	body := owner.open(store, file.ID)

	if body != receiptText || file.Size != int64(len(receiptText)) {
		t.Errorf("under the largest cap stored %q of size %d, want %q", body, file.Size, receiptText)
	}
}

func TestTheFileIsKeptUnderItsWorkspaceAndPlugin(t *testing.T) {
	t.Parallel()

	bucket := memoryBucket(t)
	store := storage.NewFileStore(bucket, "notes", testCap)

	file := as(t, acme).save(store, "text/plain", receiptText)

	want := "t/" + acme.String() + "/notes/" + file.ID.String()
	if keys := keysIn(t, bucket); len(keys) != 1 || keys[0] != want {
		t.Errorf("the bucket holds %v, want the one key %s", keys, want)
	}
}

func TestACallerWithoutAWorkspaceServesTheDefaultOne(t *testing.T) {
	t.Parallel()

	store := storage.NewFileStore(memoryBucket(t), "notes", testCap)
	unplaced := caller{t: t, ctx: t.Context()}

	file := unplaced.save(store, "text/plain", receiptText)
	body := as(t, sdk.DefaultTenantID).open(store, file.ID)

	if body != receiptText {
		t.Errorf("the default workspace opened %q, want %q", body, receiptText)
	}
}

func TestAnotherWorkspaceOrPluginNeverReachesTheFile(t *testing.T) {
	t.Parallel()

	bucket := memoryBucket(t)
	owned := storage.NewFileStore(bucket, "notes", testCap)
	file := as(t, acme).save(owned, "text/plain", receiptText)
	tests := map[string]struct {
		ctx   context.Context
		store sdk.FileStore
	}{
		"another workspace": {ctx: as(t, globex).ctx, store: owned},
		"another plugin":    {ctx: as(t, acme).ctx, store: storage.NewFileStore(bucket, "mail", testCap)},
	}

	for name, tc := range tests {
		_, openErr := tc.store.Open(tc.ctx, file.ID)
		deleteErr := tc.store.Delete(tc.ctx, file.ID)

		if !errors.Is(openErr, sdk.ErrFileNotFound) || !errors.Is(deleteErr, sdk.ErrFileNotFound) {
			t.Errorf("%s: Open() = %v and Delete() = %v, want both %v", name, openErr, deleteErr, sdk.ErrFileNotFound)
		}
	}
	if body := as(t, acme).open(owned, file.ID); body != receiptText {
		t.Errorf("the owner opened %q after the others tried, want %q", body, receiptText)
	}
}

func TestABodyOverTheCapIsTooLargeAndLeavesNoKey(t *testing.T) {
	t.Parallel()

	bucket := memoryBucket(t)
	store := storage.NewFileStore(bucket, "notes", testCap)

	_, err := store.Save(as(t, acme).ctx, "text/plain", strings.NewReader(strings.Repeat("x", testCap+1)))

	if !errors.Is(err, sdk.ErrFileTooLarge) {
		t.Errorf("Save() of %d bytes error = %v, want %v", testCap+1, err, sdk.ErrFileTooLarge)
	}
	if keys := keysIn(t, bucket); len(keys) != 0 {
		t.Errorf("the bucket holds %v, want nothing stored", keys)
	}
}

func TestABodyExactlyAtTheCapIsStored(t *testing.T) {
	t.Parallel()

	store := storage.NewFileStore(memoryBucket(t), "notes", testCap)
	owner := as(t, acme)
	body := strings.Repeat("x", testCap)

	file := owner.save(store, "text/plain", body)

	if got := owner.open(store, file.ID); got != body || file.Size != testCap {
		t.Errorf("stored %d bytes of size %d, want the %d bytes at the cap", len(got), file.Size, testCap)
	}
}

func TestABodyThatFailsToReadLeavesNoKey(t *testing.T) {
	t.Parallel()

	bucket := memoryBucket(t)
	store := storage.NewFileStore(bucket, "notes", testCap)
	body := io.MultiReader(strings.NewReader("Receipt"), iotest.ErrReader(errBrokenBody))

	_, err := store.Save(as(t, acme).ctx, "text/plain", body)

	if !errors.Is(err, errBrokenBody) {
		t.Errorf("Save() error = %v, want %v", err, errBrokenBody)
	}
	if keys := keysIn(t, bucket); len(keys) != 0 {
		t.Errorf("the bucket holds %v, want nothing stored", keys)
	}
}

func TestAWriteThatFailsPartWayIsAbortedAndKeepsNothing(t *testing.T) {
	t.Parallel()

	half := &halfWriteBucket{}
	bucket := blob.NewBucket(half)
	t.Cleanup(func() { _ = bucket.Close() })
	store := storage.NewFileStore(bucket, "notes", testCap)

	_, err := store.Save(as(t, acme).ctx, "text/plain", strings.NewReader(receiptText))

	if !errors.Is(err, errHalfWrite) || !mentions(err, "storage: write the file") {
		t.Errorf("Save() error = %v, want the failed write and %v", err, errHalfWrite)
	}
	if len(half.kept) != 0 {
		t.Errorf("the store kept %v, want the failed write aborted before the writer closed", half.kept)
	}
}

func TestABucketOutOfRoomIsNeverAFileTooLarge(t *testing.T) {
	t.Parallel()

	bucket := blob.NewBucket(exhaustedBucket{halfWriteBucket: &halfWriteBucket{}})
	t.Cleanup(func() { _ = bucket.Close() })
	store := storage.NewFileStore(bucket, "notes", testCap)

	_, err := store.Save(as(t, acme).ctx, "text/plain", strings.NewReader(receiptText))

	if errors.Is(err, sdk.ErrFileTooLarge) || gcerrors.Code(err) != gcerrors.ResourceExhausted {
		t.Errorf("Save() into a bucket out of room error = %v, want the bucket's own failure, not %v", err,
			sdk.ErrFileTooLarge)
	}
}

func TestADeletedFileIsNotFound(t *testing.T) {
	t.Parallel()

	store := storage.NewFileStore(memoryBucket(t), "notes", testCap)
	owner := as(t, acme)
	file := owner.save(store, "text/plain", receiptText)

	if err := store.Delete(owner.ctx, file.ID); err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}

	if _, err := store.Open(owner.ctx, file.ID); !errors.Is(err, sdk.ErrFileNotFound) {
		t.Errorf("Open() after Delete() error = %v, want %v", err, sdk.ErrFileNotFound)
	}
	if err := store.Delete(owner.ctx, file.ID); !errors.Is(err, sdk.ErrFileNotFound) {
		t.Errorf("a second Delete() error = %v, want %v", err, sdk.ErrFileNotFound)
	}
}

func TestAnOpenedFileSeeksForARange(t *testing.T) {
	t.Parallel()

	store := storage.NewFileStore(memoryBucket(t), "notes", testCap)
	owner := as(t, acme)
	file := owner.save(store, "text/plain", receiptText)
	reader, err := store.Open(owner.ctx, file.ID)
	if err != nil || reader == nil {
		t.Fatalf("Open() = %v, %v, want a reader of the file", reader, err)
	}
	defer func() { _ = reader.Close() }()

	if _, err := reader.Seek(8, io.SeekStart); err != nil {
		t.Fatalf("Seek() error = %v, want nil", err)
	}
	part := make([]byte, 9)
	if _, err := io.ReadFull(reader, part); err != nil || string(part) != "184467235" {
		t.Errorf("read after Seek() = %q, %v, want %q", part, err, "184467235")
	}
}

func TestAClosedBucketFailsEveryCall(t *testing.T) {
	t.Parallel()

	bucket := memblob.OpenBucket(nil)
	store := storage.NewFileStore(bucket, "notes", testCap)
	owner := as(t, acme)
	file := owner.save(store, "text/plain", receiptText)
	if err := bucket.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}

	_, saveErr := store.Save(owner.ctx, "text/plain", strings.NewReader(receiptText))
	_, openErr := store.Open(owner.ctx, file.ID)
	deleteErr := store.Delete(owner.ctx, file.ID)

	for name, err := range map[string]error{"Save()": saveErr, "Open()": openErr, "Delete()": deleteErr} {
		if errors.Is(err, sdk.ErrFileNotFound) || !strings.HasPrefix(fmt.Sprint(err), "storage: ") {
			t.Errorf("%s on a closed bucket error = %v, want a storage error that is not a missing file", name, err)
		}
	}
}
