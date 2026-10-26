// SPDX-License-Identifier: Elastic-2.0

//go:build s3

package storage_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
	"gocloud.dev/blob"

	"github.com/gopherium/alphone/internal/storage"
	"github.com/gopherium/alphone/internal/testdb"
	"github.com/gopherium/alphone/sdk"
)

// manyPartsBytes is a body past the 16 MiB at which an upload goes up in parts of 8 MiB, three parts in all.
const manyPartsBytes = 17 << 20

// emptyBudget bounds how long a test takes to delete what it left in the bucket.
const emptyBudget = 30 * time.Second

// cleanEnd names a read that ended without a failure.
const cleanEnd = "cleanly"

// checksumMismatch names a read that ended in a checksum the client could not match.
const checksumMismatch = "in a checksum mismatch"

// checksumCase is one checksum setting the S3 tests run under.
type checksumCase struct {
	// checksum names when the client adds and checks checksums.
	checksum storage.Checksum
	// partsRead names how the read of a file stored in parts ends under the setting.
	partsRead string
}

// checksumCases are the checksum settings every S3 test runs under, with what the pinned Garage answers to each.
var checksumCases = []checksumCase{
	{checksum: storage.WhenSupported, partsRead: checksumMismatch},
	{checksum: storage.WhenRequired, partsRead: cleanEnd},
}

// s3Store is the bucket the ALPHONE_TEST_S3 settings name, seen by one test for a workspace of its own.
type s3Store struct {
	// t is the test that uses the store.
	t *testing.T
	// bucket is the bucket the files live in.
	bucket *blob.Bucket
	// plain is an S3 client built apart from storage, for the checks and the cleanup of the test.
	plain *s3.Client
	// name names the bucket.
	name string
	// tenant is the workspace of the test, one no other test saves under.
	tenant uuid.UUID
}

// s3SettingsUnder returns the settings of the bucket the ALPHONE_TEST_S3 settings name under checksum.
func s3SettingsUnder(t *testing.T, checksum storage.Checksum) storage.Settings {
	t.Helper()
	return storage.Settings{Driver: storage.S3, MaxBytes: testCap, S3: storage.S3Settings{
		Endpoint:        testdb.Setting(t, "ALPHONE_TEST_S3_ENDPOINT"),
		Region:          testdb.Setting(t, "ALPHONE_TEST_S3_REGION"),
		Bucket:          testdb.Setting(t, "ALPHONE_TEST_S3_BUCKET"),
		AccessKeyID:     testdb.Setting(t, "ALPHONE_TEST_S3_ACCESS_KEY_ID"),
		SecretAccessKey: testdb.Setting(t, "ALPHONE_TEST_S3_SECRET_ACCESS_KEY"),
		PathStyle:       true,
		Checksum:        checksum,
	}}
}

// openS3 returns the S3 store under checksum for a fresh workspace, emptied of what the test left when it ends.
func openS3(t *testing.T, checksum storage.Checksum) s3Store {
	t.Helper()
	settings := s3SettingsUnder(t, checksum)
	files := storage.NewFiles(settings, nil)
	t.Cleanup(func() { _ = files.Close() })
	if err := files.Check(t.Context()); err != nil {
		t.Fatalf("Check() error = %v, want the bucket the ALPHONE_TEST_S3 settings name", err)
	}
	store := s3Store{t: t, bucket: bucketOf(t, files), plain: plainClient(settings.S3), name: settings.S3.Bucket,
		tenant: uuid.Must(uuid.NewV7())}
	t.Cleanup(store.empty)
	return store
}

// plainClient returns an S3 client of the store the settings name, with none of the request steps of storage.
func plainClient(settings storage.S3Settings) *s3.Client {
	return s3.New(s3.Options{
		Region:       settings.Region,
		BaseEndpoint: aws.String(settings.Endpoint),
		UsePathStyle: true,
		Credentials:  credentials.NewStaticCredentialsProvider(settings.AccessKeyID, settings.SecretAccessKey, ""),
	})
}

// eachChecksum runs test in parallel once under each checksum setting, each run on a store of its own.
func eachChecksum(t *testing.T, test func(t *testing.T, store s3Store, tc checksumCase)) {
	t.Helper()
	for _, tc := range checksumCases {
		t.Run(string(tc.checksum), func(t *testing.T) {
			t.Parallel()

			test(t, openS3(t, tc.checksum), tc)
		})
	}
}

// files returns the file store of the notes plugin over the bucket, each file held to maxBytes.
func (s s3Store) files(maxBytes int64) sdk.FileStore {
	return storage.NewFileStore(s.bucket, "notes", maxBytes)
}

// owner returns the test calling for the workspace of the store.
func (s s3Store) owner() caller {
	return as(s.t, s.tenant)
}

// prefix returns the start of the key of every file of the workspace of the store.
func (s s3Store) prefix() string {
	return "t/" + s.tenant.String() + "/"
}

// head returns the details the store keeps of the file id of the notes plugin, failing the test without them.
func (s s3Store) head(id uuid.UUID) *s3.HeadObjectOutput {
	s.t.Helper()
	head, err := s.plain.HeadObject(s.t.Context(), &s3.HeadObjectInput{
		Bucket: aws.String(s.name), Key: aws.String(s.prefix() + "notes/" + id.String()),
	})
	if err != nil {
		s.t.Fatalf("HeadObject() error = %v, want the details of the saved file", err)
	}
	return head
}

// keys returns every key the bucket holds under the prefix of the store.
func (s s3Store) keys(ctx context.Context) []string {
	s.t.Helper()
	listed, err := s.plain.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.name), Prefix: aws.String(s.prefix()),
	})
	if err != nil {
		s.t.Fatalf("ListObjectsV2() error = %v, want the files", err)
	}
	var keys []string
	for _, object := range listed.Contents {
		keys = append(keys, aws.ToString(object.Key))
	}
	return keys
}

// uploads returns every unfinished upload the bucket holds under the prefix of the store.
func (s s3Store) uploads(ctx context.Context) []types.MultipartUpload {
	s.t.Helper()
	listed, err := s.plain.ListMultipartUploads(ctx, &s3.ListMultipartUploadsInput{
		Bucket: aws.String(s.name), Prefix: aws.String(s.prefix()),
	})
	if err != nil {
		s.t.Fatalf("ListMultipartUploads() error = %v, want the unfinished uploads", err)
	}
	return listed.Uploads
}

// partsUp reports whether an unfinished upload under the prefix of the store holds a part.
func (s s3Store) partsUp() bool {
	for _, upload := range s.uploads(s.t.Context()) {
		parts, err := s.plain.ListParts(s.t.Context(), &s3.ListPartsInput{
			Bucket: aws.String(s.name), Key: upload.Key, UploadId: upload.UploadId,
		})
		if err == nil && len(parts.Parts) > 0 {
			return true
		}
	}
	return false
}

// assertEmpty fails the test unless the bucket holds no file and no unfinished upload under the prefix of the store.
func (s s3Store) assertEmpty(after string) {
	s.t.Helper()
	if keys := s.keys(s.t.Context()); len(keys) != 0 {
		s.t.Errorf("after %s the bucket holds %v, want no file", after, keys)
	}
	if uploads := s.uploads(s.t.Context()); len(uploads) != 0 {
		s.t.Errorf("after %s the bucket holds %d unfinished uploads, want none", after, len(uploads))
	}
}

// empty deletes every file and aborts every unfinished upload the test left under the prefix of the store.
func (s s3Store) empty() {
	ctx, cancel := context.WithTimeout(context.Background(), emptyBudget)
	defer cancel()
	s.deleteFiles(ctx)
	s.abortUploads(ctx)
}

// deleteFiles deletes every file under the prefix of the store.
func (s s3Store) deleteFiles(ctx context.Context) {
	for _, key := range s.keys(ctx) {
		_, err := s.plain.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.name), Key: aws.String(key)})
		if err != nil {
			s.t.Errorf("deleting %s the test left: %v", key, err)
		}
	}
}

// abortUploads aborts every unfinished upload under the prefix of the store.
func (s s3Store) abortUploads(ctx context.Context) {
	for _, upload := range s.uploads(ctx) {
		_, err := s.plain.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
			Bucket: aws.String(s.name), Key: upload.Key, UploadId: upload.UploadId,
		})
		if err != nil {
			s.t.Errorf("aborting the upload of %s the test left: %v", aws.ToString(upload.Key), err)
		}
	}
}

// readBack opens the file id through store and returns what a read of it got and the failure it ended in.
func (c caller) readBack(store sdk.FileStore, id uuid.UUID) ([]byte, error) {
	c.t.Helper()
	reader, err := store.Open(c.ctx, id)
	if err != nil {
		c.t.Fatalf("Open() error = %v, want a reader of the file", err)
	}
	defer func() { _ = reader.Close() }()
	return io.ReadAll(reader)
}

// endOf names how a read that ended in err ended.
func endOf(err error) string {
	switch {
	case err == nil:
		return cleanEnd
	case mentions(err, "checksum did not match"):
		return checksumMismatch
	}
	return "in " + err.Error()
}

// manyParts returns a body of manyPartsBytes bytes whose parts all differ.
func manyParts() []byte {
	line := []byte(receiptText + "\n")
	return bytes.Repeat(line, manyPartsBytes/len(line)+1)[:manyPartsBytes]
}

// breakingBody hands out its bytes and then breaks, once ready reports true.
type breakingBody struct {
	// t is the test that waits for ready.
	t *testing.T
	// rest holds the bytes not yet handed out.
	rest io.Reader
	// ready reports whether the body may break.
	ready func() bool
}

// Read hands out the next bytes, and once none are left waits for ready and answers errBrokenBody.
func (b *breakingBody) Read(p []byte) (int, error) {
	n, err := b.rest.Read(p)
	if !errors.Is(err, io.EOF) {
		return n, err
	}
	waitFor(b.t, "the first part of the upload to go up", b.ready)
	return n, errBrokenBody
}

func TestAFileSavedInS3OpensWithItsBytesTypeAndSHA256(t *testing.T) {
	t.Parallel()

	eachChecksum(t, func(t *testing.T, store s3Store, _ checksumCase) {
		files := store.files(testCap)
		owner := store.owner()

		file := owner.save(files, "application/pdf", receiptText)
		body := owner.open(files, file.ID)

		if body != receiptText {
			t.Errorf("opened %q, want %q", body, receiptText)
		}
		want := sdk.StoredFile{ID: file.ID, Size: int64(len(receiptText)), ContentType: "application/pdf",
			SHA256: shaText(receiptText)}
		if got := withoutTime(file); got != want {
			t.Errorf("Save() = %+v, want %+v", got, want)
		}
		if stored := aws.ToString(store.head(file.ID).ContentType); stored != want.ContentType {
			t.Errorf("the bucket keeps the content type %q, want %s", stored, want.ContentType)
		}
	})
}

func TestADeletedS3FileIsNotFoundAndLeavesNothing(t *testing.T) {
	t.Parallel()

	eachChecksum(t, func(t *testing.T, store s3Store, _ checksumCase) {
		files := store.files(testCap)
		owner := store.owner()
		file := owner.save(files, "text/plain", receiptText)

		if err := files.Delete(owner.ctx, file.ID); err != nil {
			t.Fatalf("Delete() error = %v, want nil", err)
		}

		_, openErr := files.Open(owner.ctx, file.ID)
		deleteErr := files.Delete(owner.ctx, file.ID)
		if !errors.Is(openErr, sdk.ErrFileNotFound) || !errors.Is(deleteErr, sdk.ErrFileNotFound) {
			t.Errorf("after Delete() Open() = %v and Delete() = %v, want both %v", openErr, deleteErr, sdk.ErrFileNotFound)
		}
		store.assertEmpty("the delete")
	})
}

func TestAnS3FileNeverSavedOrOfAnotherWorkspaceIsNotFound(t *testing.T) {
	t.Parallel()

	eachChecksum(t, func(t *testing.T, store s3Store, _ checksumCase) {
		files := store.files(testCap)
		owner := store.owner()
		file := owner.save(files, "text/plain", receiptText)
		tests := map[string]struct {
			ctx context.Context
			id  uuid.UUID
		}{
			"a file never saved":          {ctx: owner.ctx, id: uuid.Must(uuid.NewV7())},
			"a file of another workspace": {ctx: as(t, uuid.Must(uuid.NewV7())).ctx, id: file.ID},
		}

		for name, tc := range tests {
			_, openErr := files.Open(tc.ctx, tc.id)
			deleteErr := files.Delete(tc.ctx, tc.id)

			if !errors.Is(openErr, sdk.ErrFileNotFound) || !errors.Is(deleteErr, sdk.ErrFileNotFound) {
				t.Errorf("%s: Open() = %v and Delete() = %v, want both %v", name, openErr, deleteErr, sdk.ErrFileNotFound)
			}
		}
		if body := owner.open(files, file.ID); body != receiptText {
			t.Errorf("the owner opened %q after the others tried, want %q", body, receiptText)
		}
	})
}

func TestAnS3BodyOverTheCapIsTooLargeAndLeavesNothing(t *testing.T) {
	t.Parallel()

	eachChecksum(t, func(t *testing.T, store s3Store, _ checksumCase) {
		body := strings.NewReader(strings.Repeat("x", testCap+1))

		_, err := store.files(testCap).Save(store.owner().ctx, "text/plain", body)

		if !errors.Is(err, sdk.ErrFileTooLarge) {
			t.Errorf("Save() of %d bytes error = %v, want %v", testCap+1, err, sdk.ErrFileTooLarge)
		}
		store.assertEmpty("a body over the cap")
	})
}

func TestAnS3BodyThatFailsToReadLeavesNothing(t *testing.T) {
	t.Parallel()

	eachChecksum(t, func(t *testing.T, store s3Store, _ checksumCase) {
		body := io.MultiReader(strings.NewReader("Receipt"), iotest.ErrReader(errBrokenBody))

		_, err := store.files(testCap).Save(store.owner().ctx, "text/plain", body)

		if !errors.Is(err, errBrokenBody) {
			t.Errorf("Save() error = %v, want %v", err, errBrokenBody)
		}
		store.assertEmpty("a body that failed to read")
	})
}

func TestAFileOfManyPartsGoesUpInPartsAndReadsBackFromS3(t *testing.T) {
	t.Parallel()

	eachChecksum(t, func(t *testing.T, store s3Store, tc checksumCase) {
		files := store.files(manyPartsBytes)
		owner := store.owner()
		body := manyParts()

		file, err := files.Save(owner.ctx, "application/octet-stream", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Save() of %d bytes error = %v, want the file stored", len(body), err)
		}

		if etag := aws.ToString(store.head(file.ID).ETag); !strings.HasSuffix(etag, `-3"`) {
			t.Errorf("the ETag %s does not end in -3, want the file stored in three parts", etag)
		}
		want := sdk.StoredFile{ID: file.ID, Size: manyPartsBytes, ContentType: "application/octet-stream",
			SHA256: shaText(string(body))}
		if got := withoutTime(file); got != want {
			t.Errorf("Save() = %+v, want %+v", got, want)
		}
		read, readErr := owner.readBack(files, file.ID)
		if !bytes.Equal(read, body) {
			t.Errorf("read %d bytes, want the %d bytes saved", len(read), len(body))
		}
		if end := endOf(readErr); end != tc.partsRead {
			t.Errorf("the read ended %s, want it to end %s", end, tc.partsRead)
		}
	})
}

func TestAnUploadOfManyPartsThatStopsLeavesNothingInS3(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		body func(store s3Store) io.Reader
		want error
	}{
		"a body that breaks after its first part went up": {
			body: func(store s3Store) io.Reader {
				return &breakingBody{t: store.t, rest: bytes.NewReader(manyParts()), ready: store.partsUp}
			},
			want: errBrokenBody,
		},
		"a body one byte over the cap": {
			body: func(s3Store) io.Reader {
				return io.MultiReader(bytes.NewReader(manyParts()), strings.NewReader("x"))
			},
			want: sdk.ErrFileTooLarge,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			eachChecksum(t, func(t *testing.T, store s3Store, _ checksumCase) {
				_, err := store.files(manyPartsBytes).Save(store.owner().ctx, "application/octet-stream", tc.body(store))

				if !errors.Is(err, tc.want) {
					t.Errorf("Save() error = %v, want %v", err, tc.want)
				}
				store.assertEmpty(name)
			})
		})
	}
}
