// SPDX-License-Identifier: Elastic-2.0

package storage_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/peterldowns/pgtestdb"
	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"

	"github.com/gopherium/alphone/internal/storage"
	"github.com/gopherium/alphone/internal/testdb"
	"github.com/gopherium/alphone/sdk"
)

// objectRows counts the rows of the storage schema, live and pending alike.
const objectRows = "SELECT count(*) FROM storage.objects"

// pendingRowsLike counts the pending rows whose key matches the LIKE pattern its one argument names.
const pendingRowsLike = "SELECT count(*) FROM storage.objects WHERE state = 'pending' AND key LIKE $1"

// smallRow is the most bytes one row of the postgres driver holds in the streaming test.
const smallRow = 16

// pieceBody hands out its bytes a few at a time and counts once what the storage schema holds midway.
type pieceBody struct {
	// rest holds the bytes not yet handed out.
	rest []byte
	// piece is the most bytes one read hands out.
	piece int
	// handed counts the bytes handed out so far.
	handed int
	// watch counts the rows of interest once more than three pieces were handed out.
	watch func() int
	// seen is what watch counted, -1 until it ran.
	seen int
}

// pieces returns a body handing out text piece bytes at a time and watching nothing.
func pieces(text string, piece int) *pieceBody {
	return &pieceBody{rest: []byte(text), piece: piece, watch: func() int { return 0 }, seen: -1}
}

// Read hands out the next piece, running watch first once more than three pieces were handed out.
func (b *pieceBody) Read(p []byte) (int, error) {
	if len(b.rest) == 0 {
		return 0, io.EOF
	}
	if b.seen < 0 && b.handed > 3*b.piece {
		b.seen = b.watch()
	}
	n := copy(p[:min(len(p), b.piece)], b.rest)
	b.rest = b.rest[n:]
	b.handed += n
	return n, nil
}

// s3Settings returns the settings of an S3 bucket at endpoint.
func s3Settings(endpoint string) storage.Settings {
	return storage.Settings{Driver: storage.S3, MaxBytes: testCap, S3: storage.S3Settings{
		Endpoint: endpoint, Region: "garage", Bucket: "example-files", AccessKeyID: "GKexample",
		SecretAccessKey: "example-secret", PathStyle: true, Checksum: storage.WhenRequired,
	}}
}

// postgresSettings returns the settings of the postgres driver, rows of four bytes and a sweep age below its interval.
func postgresSettings() storage.Settings {
	return storage.Settings{
		Driver: storage.Postgres, MaxBytes: testCap, ChunkBytes: 4, CleanupTimeout: time.Second,
		SweepInterval: time.Hour, SweepAge: 30 * time.Minute, SweepBatch: 10,
	}
}

// answeringS3 returns the address of a store that answers every request with status and the S3 error code.
func answeringS3(t *testing.T, status int, code string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(status)
		_, _ = w.Write([]byte("<Error><Code>" + code + "</Code><Message>" + code + "</Message></Error>"))
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// bareDatabase returns a fresh database holding no schema, skipping the test in short mode.
func bareDatabase(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	return testdb.New(t, pgtestdb.NoopMigrator{})
}

// migratedDatabase returns a fresh database holding the storage schema.
func migratedDatabase(t *testing.T) string {
	t.Helper()
	address := bareDatabase(t)
	if err := storage.Migrate(t.Context(), address); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	return address
}

// poolOn returns a pool on the database at address, closed when the test ends.
func poolOn(t *testing.T, address string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), address)
	if err != nil {
		t.Fatalf("opening a pool on the test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// bucketOf returns the bucket files open, failing the test when they cannot.
func bucketOf(t *testing.T, files *storage.Files) *blob.Bucket {
	t.Helper()
	bucket, err := files.Bucket(t.Context())
	if err != nil {
		t.Fatalf("Bucket() error = %v, want the bucket", err)
	}
	return bucket
}

// count returns the count query answers on pool for args.
func count(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func TestOpenBuildsTheS3BucketOnTheClientTheSettingsName(t *testing.T) {
	t.Parallel()

	settings := s3Settings("https://files.example.com")

	bucket, err := storage.Open(t.Context(), settings, nil)

	if err != nil {
		t.Fatalf("Open() error = %v, want an S3 bucket opened without reaching the store", err)
	}
	t.Cleanup(func() { _ = bucket.Close() })
	var client *s3.Client
	if !bucket.As(&client) {
		t.Fatal("As(*s3.Client) = false, want the client of the bucket")
	}
	options := client.Options()
	got := clientView{
		endpoint: aws.ToString(options.BaseEndpoint), region: options.Region, pathStyle: options.UsePathStyle,
		checksum: options.RequestChecksumCalculation,
	}
	want := clientView{
		endpoint: settings.S3.Endpoint, region: settings.S3.Region, pathStyle: true,
		checksum: aws.RequestChecksumCalculationWhenRequired,
	}
	if got != want {
		t.Errorf("the client options = %+v, want the endpoint, region, path style and checksum named %+v", got, want)
	}
}

// clientView is what a test reads back from the options of an S3 client.
type clientView struct {
	// endpoint is the address the client sends its requests to.
	endpoint string
	// region is the region the client signs for.
	region string
	// pathStyle reports whether the bucket goes in the path of each request.
	pathStyle bool
	// checksum is when the client adds a checksum to a request.
	checksum aws.RequestChecksumCalculation
}

func TestOpenNamesTheDriverOfABucketItCannotOpen(t *testing.T) {
	t.Parallel()

	nameless := s3Settings("https://files.example.com")
	nameless.S3.Bucket = ""
	tests := map[string]storage.Settings{
		"s3":       nameless,
		"postgres": postgresSettings(),
	}

	for driver, settings := range tests {
		_, err := storage.Open(t.Context(), settings, nil)

		if !mentions(err, "the "+driver+" bucket") {
			t.Errorf("Open() of the %s driver error = %v, want one naming the driver", driver, err)
		}
	}
}

func TestCheckNamesTheDriverOfABucketThatIsNotAccessible(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		status int
		code   string
	}{
		"a bucket the store does not hold": {status: http.StatusNotFound, code: "NoSuchBucket"},
		"a bucket the keys may not read":   {status: http.StatusForbidden, code: "AccessDenied"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			files := storage.NewFiles(s3Settings(answeringS3(t, tc.status, tc.code)), nil)
			t.Cleanup(func() { _ = files.Close() })

			err := files.Check(t.Context())

			if !mentions(err, "the s3 bucket") {
				t.Errorf("Check() error = %v, want one naming the s3 bucket", err)
			}
		})
	}
}

func TestFilesThatCannotOpenTheirBucketFailEveryCall(t *testing.T) {
	t.Parallel()

	files := storage.NewFiles(postgresSettings(), nil)
	store := files.Store("notes")
	ctx := as(t, acme).ctx

	_, saveErr := store.Save(ctx, "text/plain", strings.NewReader(receiptText))
	_, openErr := store.Open(ctx, acme)
	deleteErr := store.Delete(ctx, acme)
	checkErr := files.Check(ctx)

	for name, err := range map[string]error{"Save()": saveErr, "Open()": openErr, "Delete()": deleteErr,
		"Check()": checkErr} {
		if errors.Is(err, sdk.ErrFileNotFound) || !mentions(err, "the postgres bucket") {
			t.Errorf("%s error = %v, want the bucket that cannot open named", name, err)
		}
	}
	if err := files.Close(); err != nil {
		t.Errorf("Close() of files that never opened a bucket error = %v, want nil", err)
	}
}

func TestAFileSurvivesAReopenOfThePostgresBucket(t *testing.T) {
	t.Parallel()

	pool := poolOn(t, migratedDatabase(t))
	owner := as(t, acme)
	first := storage.NewFiles(postgresSettings(), pool)
	if err := first.Check(owner.ctx); err != nil {
		t.Fatalf("Check() of a migrated database error = %v, want nil", err)
	}
	file := owner.save(first.Store("notes"), "application/pdf", receiptText)
	if err := first.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}

	second := storage.NewFiles(postgresSettings(), pool)
	t.Cleanup(func() { _ = second.Close() })
	body := owner.open(second.Store("notes"), file.ID)

	if body != receiptText {
		t.Errorf("reopened %q, want the bytes saved %q", body, receiptText)
	}
	kept := storedAttributes(t, bucketOf(t, second))
	reopened := sdk.StoredFile{ID: file.ID, Size: kept.Size, ContentType: kept.ContentType, SHA256: shaText(body)}
	if saved := withoutTime(file); reopened != saved {
		t.Errorf("reopened details %+v, want the ones Save answered %+v", reopened, saved)
	}
	if err := pool.Ping(owner.ctx); err != nil {
		t.Errorf("Ping() after closing the files error = %v, want the pool left open", err)
	}
}

func TestFilesKeepOneBucketUntilTheyClose(t *testing.T) {
	t.Parallel()

	files := storage.NewFiles(postgresSettings(), poolOn(t, migratedDatabase(t)))
	store := files.Store("notes")
	owner := as(t, acme)
	file := owner.save(store, "text/plain", receiptText)

	first, second := bucketOf(t, files), bucketOf(t, files)

	if first != second {
		t.Errorf("Bucket() twice = %p and %p, want the one bucket both times", first, second)
	}
	if err := files.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
	if _, err := store.Open(owner.ctx, file.ID); gcerrors.Code(err) != gcerrors.FailedPrecondition {
		t.Errorf("Open() after Close() error = %v, want the closed bucket named and no missing file", err)
	}
}

func TestTheFilesOfOnePostgresWorkspaceStayApart(t *testing.T) {
	t.Parallel()

	files := storage.NewFiles(postgresSettings(), poolOn(t, migratedDatabase(t)))
	t.Cleanup(func() { _ = files.Close() })
	store := files.Store("notes")
	file := as(t, acme).save(store, "text/plain", receiptText)

	_, err := store.Open(as(t, globex).ctx, file.ID)

	if !errors.Is(err, sdk.ErrFileNotFound) {
		t.Errorf("Open() from another workspace error = %v, want %v", err, sdk.ErrFileNotFound)
	}
}

func TestAFileThePostgresBucketCannotFinishIsNotStored(t *testing.T) {
	t.Parallel()

	pool := poolOn(t, bareDatabase(t))
	whole := postgresSettings()
	whole.ChunkBytes = 0
	bucket, err := storage.Open(t.Context(), whole, pool)
	if err != nil {
		t.Fatalf("Open() error = %v, want the postgres bucket over a database without its tables", err)
	}
	t.Cleanup(func() { _ = bucket.Close() })

	_, err = storage.NewFileStore(bucket, "notes", testCap).Save(as(t, acme).ctx, "text/plain",
		strings.NewReader(receiptText))

	if !strings.HasPrefix(fmt.Sprint(err), "storage: store the file: ") {
		t.Errorf("Save() into a database without the storage tables error = %v, want the failed store", err)
	}
}

func TestSaveStreamsTheBodyIntoThePostgresBucketWhileItReads(t *testing.T) {
	t.Parallel()

	pool := poolOn(t, migratedDatabase(t))
	settings := postgresSettings()
	settings.ChunkBytes, settings.MaxBytes = smallRow, 8*smallRow
	files := storage.NewFiles(settings, pool)
	t.Cleanup(func() { _ = files.Close() })
	owner := as(t, acme)
	text := strings.Repeat("x", 8*smallRow)
	body := pieces(text, smallRow)
	body.watch = func() int { return count(t, pool, pendingRowsLike, []byte("t/"+acme.String()+"/notes/%")) }

	file, err := files.Store("notes").Save(owner.ctx, "text/plain", body)
	if err != nil {
		t.Fatalf("Save() error = %v, want the file stored", err)
	}

	if body.seen != 1 {
		t.Errorf("the storage schema held %d pending rows of the save while its body was read, want the one row of a "+
			"save that streams", body.seen)
	}
	if got := owner.open(files.Store("notes"), file.ID); got != text || file.SHA256 != shaText(text) {
		t.Errorf("opened %d bytes under the SHA-256 %s, want the %d bytes saved under %s", len(got), file.SHA256,
			len(text), shaText(text))
	}
}

func TestABodyOverTheCapLeavesNoRowInThePostgresBucket(t *testing.T) {
	t.Parallel()

	pool := poolOn(t, migratedDatabase(t))
	files := storage.NewFiles(postgresSettings(), pool)
	t.Cleanup(func() { _ = files.Close() })

	_, err := files.Store("notes").Save(as(t, acme).ctx, "text/plain", pieces(strings.Repeat("x", testCap+1), smallRow))

	if !errors.Is(err, sdk.ErrFileTooLarge) || mentions(err, "postgresblob") {
		t.Errorf("Save() of %d bytes error = %v, want %v and no driver error", testCap+1, err, sdk.ErrFileTooLarge)
	}
	if rows := count(t, pool, objectRows); rows != 0 {
		t.Errorf("the storage schema holds %d rows, live or pending, want nothing stored", rows)
	}
}
