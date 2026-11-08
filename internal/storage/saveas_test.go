// SPDX-License-Identifier: Elastic-2.0

package storage_test

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/google/uuid"
	"gocloud.dev/blob"
	"gocloud.dev/blob/driver"
	"gocloud.dev/gcerrors"

	"github.com/gopherium/alphone/internal/storage"
	"github.com/gopherium/alphone/sdk"
)

// laterText is the body a second save under the same id carries.
const laterText = "Refund for Maria Perez"

// errPrecondition is the failure of a write whose precondition failed.
var errPrecondition = errors.New("the precondition of the write failed")

// saveAs saves body under id and the content type through store, failing the test unless it is stored.
func (c caller) saveAs(store sdk.FileStore, id uuid.UUID, contentType, body string) sdk.StoredFile {
	c.t.Helper()
	file, err := store.SaveAs(c.ctx, id, contentType, strings.NewReader(body))
	if err != nil {
		c.t.Fatalf("SaveAs() error = %v, want the file stored", err)
	}
	return file
}

// preconditionBucket hands out writers that answer closeErr on Close and records what each call asked for.
type preconditionBucket struct {
	driver.Bucket
	// closeErr is what every writer answers on Close.
	closeErr error
	// lookups holds what each look up of a key answers in turn, nil for a key that holds a file, errNoFile once empty.
	lookups []error
	// looked counts the look ups of a key.
	looked int
	// exclusive records, for each write, whether it asked the bucket to keep a file its key already holds.
	exclusive []bool
}

// NewTypedWriter records whether the write asked to keep a file its key holds and returns a writer.
func (b *preconditionBucket) NewTypedWriter(_ context.Context, _, _ string, opts *driver.WriterOptions) (
	driver.Writer, error) {
	b.exclusive = append(b.exclusive, opts.IfNotExist)
	return writerClosingWith{err: b.closeErr}, nil
}

// Attributes answers the next of lookups.
func (b *preconditionBucket) Attributes(context.Context, string) (*driver.Attributes, error) {
	b.looked++
	if len(b.lookups) == 0 {
		return nil, errNoFile
	}
	answer := b.lookups[0]
	b.lookups = b.lookups[1:]
	return &driver.Attributes{}, answer
}

// ErrorCode reads errPrecondition as a failed precondition, errNoFile as not found and every other failure as unknown.
func (*preconditionBucket) ErrorCode(err error) gcerrors.ErrorCode {
	switch {
	case errors.Is(err, errPrecondition):
		return gcerrors.FailedPrecondition
	case errors.Is(err, errNoFile):
		return gcerrors.NotFound
	}
	return gcerrors.Unknown
}

// Close closes nothing.
func (*preconditionBucket) Close() error {
	return nil
}

// writerClosingWith takes every write and answers err on Close.
type writerClosingWith struct {
	// err is what Close answers.
	err error
}

// Write takes all of p.
func (writerClosingWith) Write(p []byte) (int, error) {
	return len(p), nil
}

// Close answers err.
func (w writerClosingWith) Close() error {
	return w.err
}

// bodyRunningAtEnd hands out its bytes and, once none are left, runs atEnd before it answers io.EOF.
type bodyRunningAtEnd struct {
	// rest holds the bytes not yet handed out.
	rest io.Reader
	// atEnd runs once, when the bytes run out.
	atEnd func()
	// ended reports whether atEnd ran.
	ended bool
}

// Read hands out the next bytes, running atEnd the first time none are left.
func (b *bodyRunningAtEnd) Read(p []byte) (int, error) {
	n, err := b.rest.Read(p)
	if errors.Is(err, io.EOF) && !b.ended {
		b.ended = true
		b.atEnd()
	}
	return n, err
}

// preconditionStore returns the file store of the notes plugin over bucket, closed when the test ends.
func preconditionStore(t *testing.T, bucket *preconditionBucket) sdk.FileStore {
	t.Helper()
	opened := blob.NewBucket(bucket)
	t.Cleanup(func() { _ = opened.Close() })
	return storage.NewFileStore(opened, "notes", testCap)
}

func TestSaveAsKeepsTheFileUnderTheCallersID(t *testing.T) {
	t.Parallel()

	bucket := memoryBucket(t)
	store := storage.NewFileStore(bucket, "notes", testCap)
	owner := as(t, acme)
	id := uuid.Must(uuid.NewV7())
	before := time.Now()

	file := owner.saveAs(store, id, "application/pdf", receiptText)
	body := owner.open(store, id)

	if body != receiptText {
		t.Errorf("opened body = %q, want %q", body, receiptText)
	}
	want := sdk.StoredFile{ID: id, Size: int64(len(receiptText)), ContentType: "application/pdf",
		SHA256: shaText(receiptText)}
	if got := withoutTime(file); got != want {
		t.Errorf("SaveAs() = %+v, want %+v", got, want)
	}
	if !storedAround(file.StoredAt, before) {
		t.Errorf("SaveAs() StoredAt = %v, want the time of the save", file.StoredAt)
	}
	if keys := keysIn(t, bucket); len(keys) != 1 || keys[0] != "t/"+acme.String()+"/notes/"+id.String() {
		t.Errorf("the bucket holds %v, want the one key of the id the caller chose", keys)
	}
}

func TestSaveAsFollowsEveryRuleOfSave(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		contentType string
		body        func() io.Reader
		refused     func(error) bool
	}{
		"a body over the cap": {
			contentType: "text/plain",
			body:        func() io.Reader { return strings.NewReader(strings.Repeat("x", testCap+1)) },
			refused:     func(err error) bool { return errors.Is(err, sdk.ErrFileTooLarge) },
		},
		"a malformed content type": {
			contentType: "image/png; =",
			body:        func() io.Reader { return strings.NewReader(receiptText) },
			refused:     func(err error) bool { return mentions(err, `"image/png; ="`) },
		},
		"a content type without a subtype": {
			contentType: "inline",
			body:        func() io.Reader { return strings.NewReader(receiptText) },
			refused:     func(err error) bool { return mentions(err, `"inline"`) },
		},
		"a body that fails to read": {
			contentType: "text/plain",
			body: func() io.Reader {
				return io.MultiReader(strings.NewReader("Receipt"), iotest.ErrReader(errBrokenBody))
			},
			refused: func(err error) bool { return errors.Is(err, errBrokenBody) },
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			bucket := memoryBucket(t)
			store := storage.NewFileStore(bucket, "notes", testCap)
			owner := as(t, acme)
			id := uuid.Must(uuid.NewV7())

			_, err := store.SaveAs(owner.ctx, id, tc.contentType, tc.body())

			if !tc.refused(err) {
				t.Errorf("SaveAs() error = %v, want the save refused as Save refuses it", err)
			}
			if keys := keysIn(t, bucket); len(keys) != 0 {
				t.Errorf("the bucket holds %v after the refused save, want nothing stored", keys)
			}
			owner.saveAs(store, id, "text/plain", receiptText)
			if body := owner.open(store, id); body != receiptText {
				t.Errorf("the id opens %q after a later save, want the refused save to leave the id free", body)
			}
		})
	}
	t.Run("a write that fails part way", func(t *testing.T) {
		t.Parallel()

		half := &halfWriteBucket{}
		bucket := blob.NewBucket(half)
		t.Cleanup(func() { _ = bucket.Close() })
		store := storage.NewFileStore(bucket, "notes", testCap)

		_, err := store.SaveAs(as(t, acme).ctx, uuid.Must(uuid.NewV7()), "text/plain", strings.NewReader(receiptText))

		if !errors.Is(err, errHalfWrite) || len(half.kept) != 0 {
			t.Errorf("SaveAs() error = %v and the store kept %v, want %v and the write aborted", err, half.kept,
				errHalfWrite)
		}
	})
}

func TestAnotherWorkspaceOrPluginNeverReachesAFileSavedAs(t *testing.T) {
	t.Parallel()

	bucket := memoryBucket(t)
	owned := storage.NewFileStore(bucket, "notes", testCap)
	id := uuid.Must(uuid.NewV7())
	as(t, acme).saveAs(owned, id, "text/plain", receiptText)
	tests := map[string]struct {
		ctx   context.Context
		store sdk.FileStore
	}{
		"another workspace": {ctx: as(t, globex).ctx, store: owned},
		"another plugin":    {ctx: as(t, acme).ctx, store: storage.NewFileStore(bucket, "mail", testCap)},
	}

	for name, tc := range tests {
		_, openErr := tc.store.Open(tc.ctx, id)
		deleteErr := tc.store.Delete(tc.ctx, id)
		file, saveErr := tc.store.SaveAs(tc.ctx, id, "text/plain", strings.NewReader(laterText))

		if !errors.Is(openErr, sdk.ErrFileNotFound) || !errors.Is(deleteErr, sdk.ErrFileNotFound) {
			t.Errorf("%s: Open() = %v and Delete() = %v, want both %v", name, openErr, deleteErr, sdk.ErrFileNotFound)
		}
		if saveErr != nil || file.ID != id {
			t.Errorf("%s: SaveAs() under the owner's id = %+v, %v, want a file of its own", name, file, saveErr)
		}
	}
	if body := as(t, acme).open(owned, id); body != receiptText {
		t.Errorf("the owner opened %q after the others tried, want %q", body, receiptText)
	}
}

func TestSaveAsOverAnIDInUseAnswersErrFileExists(t *testing.T) {
	t.Parallel()

	bucket := memoryBucket(t)
	store := storage.NewFileStore(bucket, "notes", testCap)
	owner := as(t, acme)
	id := uuid.Must(uuid.NewV7())
	first := owner.saveAs(store, id, "text/plain", receiptText)

	_, err := store.SaveAs(owner.ctx, id, "application/pdf", strings.NewReader(laterText))

	if !errors.Is(err, sdk.ErrFileExists) {
		t.Errorf("SaveAs() over an id in use error = %v, want %v", err, sdk.ErrFileExists)
	}
	if body := owner.open(store, id); body != receiptText {
		t.Errorf("the id in use opens %q after the second save, want the first bytes %q", body, receiptText)
	}
	if kept := storedAttributes(t, bucket); kept.ContentType != first.ContentType || kept.Size != first.Size {
		t.Errorf("the bucket keeps %s of %d bytes, want the first file's %s of %d bytes", kept.ContentType, kept.Size,
			first.ContentType, first.Size)
	}
}

func TestSaveAsThatLosesTheRaceForItsIDAnswersErrFileExists(t *testing.T) {
	t.Parallel()

	bucket := memoryBucket(t)
	store := storage.NewFileStore(bucket, "notes", testCap)
	owner := as(t, acme)
	id := uuid.Must(uuid.NewV7())
	rival := func() { owner.saveAs(store, id, "text/plain", laterText) }
	body := &bodyRunningAtEnd{rest: strings.NewReader(receiptText), atEnd: rival}

	_, err := store.SaveAs(owner.ctx, id, "application/pdf", body)

	if !errors.Is(err, sdk.ErrFileExists) {
		t.Errorf("SaveAs() of an id another save took during the write error = %v, want %v", err, sdk.ErrFileExists)
	}
	if got := owner.open(store, id); got != laterText {
		t.Errorf("the id opens %q after the race, want the bytes of the save that finished first %q", got, laterText)
	}
}

func TestAFailedPreconditionIsAnIDInUseOnlyWhenTheIDHoldsAFile(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		closeErr   error
		afterWrite error
		want       error
	}{
		"an id that holds no file": {closeErr: errPrecondition, afterWrite: errNoFile, want: errPrecondition},
		"an id the bucket cannot look up after the write": {
			closeErr: errPrecondition, afterWrite: errNoDetails, want: errPrecondition,
		},
		"an id another save took during the write": {closeErr: errPrecondition, afterWrite: nil, want: sdk.ErrFileExists},
		"a failure that is no precondition on an id another save took": {
			closeErr: errHalfWrite, afterWrite: nil, want: errHalfWrite,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			bucket := &preconditionBucket{closeErr: tc.closeErr, lookups: []error{errNoFile, tc.afterWrite}}

			_, err := preconditionStore(t, bucket).SaveAs(as(t, acme).ctx, uuid.Must(uuid.NewV7()), "text/plain",
				strings.NewReader(receiptText))

			if !errors.Is(err, tc.want) || errors.Is(err, sdk.ErrFileExists) != errors.Is(tc.want, sdk.ErrFileExists) {
				t.Errorf("SaveAs() error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestASaveAsWhoseIDCannotBeLookedUpWritesNothing(t *testing.T) {
	t.Parallel()

	bucket := &preconditionBucket{lookups: []error{errNoDetails}}

	_, err := preconditionStore(t, bucket).SaveAs(as(t, acme).ctx, uuid.Must(uuid.NewV7()), "text/plain",
		strings.NewReader(receiptText))

	if !errors.Is(err, errNoDetails) || !mentions(err, "storage: look up the id") {
		t.Errorf("SaveAs() error = %v, want the failed look up named", err)
	}
	if len(bucket.exclusive) != 0 {
		t.Errorf("the bucket took %d writes, want none after the failed look up", len(bucket.exclusive))
	}
}

func TestOnlySaveAsLooksUpItsIDAndAsksTheBucketToKeepTheFileItHolds(t *testing.T) {
	t.Parallel()

	bucket := &preconditionBucket{}
	store := preconditionStore(t, bucket)
	owner := as(t, acme)

	owner.save(store, "text/plain", receiptText)
	owner.saveAs(store, uuid.Must(uuid.NewV7()), "text/plain", receiptText)

	if want := []bool{false, true}; !slices.Equal(bucket.exclusive, want) || bucket.looked != 1 {
		t.Errorf("Save() then SaveAs() asked to keep the file an id holds %v after %d look ups, want %v after the "+
			"one look up of SaveAs", bucket.exclusive, bucket.looked, want)
	}
}
