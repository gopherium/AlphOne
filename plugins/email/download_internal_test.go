// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"bytes"
	"context"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gopherium/alphone/sdk"
)

// attachedPDF is the content of the PDF the files mail carries.
const attachedPDF = "%PDF-1.4 quote"

// attachedPNG is the content of the photo the files mail carries.
const attachedPNG = "\x89PNG\r\n\x1a\n"

// downloadAttachmentMax is the largest attachment a download test keeps.
const downloadAttachmentMax = 16

// filesMail returns a mail from Maria Perez holding a PDF named under a path, a file over the cap and a photo.
func filesMail() []byte {
	return rawMail(
		"From: Maria Perez <maria.perez@example.com>", "To: info@example.com", "Subject: Quote attached",
		"Date: "+quoteDate, "Message-ID: <quote-attached@example.com>",
		"MIME-Version: 1.0", `Content-Type: multipart/mixed; boundary="outer"`, "",
		"--outer", `Content-Type: multipart/related; boundary="inner"`, "",
		"--inner", "Content-Type: text/html", "", `<p>Here is the quote.</p><img src="cid:lamp-photo@example.com">`,
		"--inner", "Content-Type: image/png", "Content-ID: <lamp-photo@example.com>",
		"Content-Transfer-Encoding: base64", "", "iVBORw0KGgo=", "--inner--",
		"--outer", "Content-Type: application/pdf", `Content-Disposition: attachment; filename="../../quote.pdf"`, "",
		attachedPDF,
		"--outer", "Content-Type: application/octet-stream", `Content-Disposition: attachment; filename="big.bin"`, "",
		strings.Repeat("x", 40), "--outer--")
}

// downloadTest is one download test: an ingest test beside the routes that serve the files it keeps.
type downloadTest struct {
	*ingestTest
	// routes serves the downloads over the test's database and file store.
	routes http.Handler
}

// newDownloadTest returns a download test that keeps no attachment over downloadAttachmentMax, changed by change.
func newDownloadTest(t *testing.T, change ...func(*settings)) *downloadTest {
	t.Helper()
	capped := func(s *settings) { s.files.attachmentMaxBytes = downloadAttachmentMax }
	it := newIngestTest(t, append([]func(*settings){capped}, change...)...)
	return &downloadTest{ingestTest: it, routes: (&downloads{db: it.pool, store: it.store}).routes()}
}

// quoteFiles returns the files mail of the workspace tenant, beside the id of each of its files in position order.
func (dt *downloadTest) quoteFiles(tenant uuid.UUID) (uuid.UUID, []uuid.UUID) {
	dt.t.Helper()
	var mail uuid.UUID
	if err := dt.pool.QueryRow(dt.t.Context(), `SELECT id FROM plugin_email.messages
		WHERE tenant_id = $1 AND subject = 'Quote attached'`, tenant).Scan(&mail); err != nil {
		dt.t.Fatalf("reading the files mail of %v: %v", tenant, err)
	}
	rows, _ := dt.pool.Query(dt.t.Context(), `SELECT id FROM plugin_email.attachments
		WHERE tenant_id = $1 AND message_id = $2 ORDER BY position`, tenant, mail)
	files, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil || len(files) != 3 {
		dt.t.Fatalf("the files of the files mail are %v, %v, want 3", files, err)
	}
	return mail, files
}

// attachmentPath returns the path of the attachment id under the download routes.
func attachmentPath(id uuid.UUID) string {
	return "/attachments/" + id.String()
}

// originalPath returns the path of the original of the mail id under the download routes.
func originalPath(id uuid.UUID) string {
	return "/messages/" + id.String() + "/original"
}

// getFile requests target through routes in the workspace tenant, with each header of extra.
func getFile(
	t *testing.T, routes http.Handler, tenant uuid.UUID, target string, extra http.Header,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(sdk.WithTenant(t.Context(), tenant), http.MethodGet, target, nil)
	for name, values := range extra {
		request.Header[name] = values
	}
	recorder := httptest.NewRecorder()
	routes.ServeHTTP(recorder, request)
	return recorder
}

// wantHeaders fails the test for every header of want the answer does not carry as want names it.
func wantHeaders(t *testing.T, header http.Header, want map[string]string) {
	t.Helper()
	for name, value := range want {
		if got := header.Get(name); got != value {
			t.Errorf("%s = %q, want %q", name, got, value)
		}
	}
}

// guardHeaders are the headers every download carries.
var guardHeaders = map[string]string{
	"Cache-Control": "private, no-store", "X-Content-Type-Options": "nosniff", "Content-Security-Policy": "sandbox",
	"Accept-Ranges": "bytes",
}

// openFailingStore is a file store whose every Open answers errStoreDown.
type openFailingStore struct {
	sdk.FileStore
}

// Open answers errStoreDown.
func (openFailingStore) Open(context.Context, uuid.UUID) (io.ReadSeekCloser, error) {
	return nil, errStoreDown
}

// closeCountingStore hands every call to the host's file store, counting each close of a file it opened.
type closeCountingStore struct {
	sdk.FileStore
	// closed counts the closes.
	closed *atomic.Int32
}

// Open opens the file through the host's store, counting its close.
func (c closeCountingStore) Open(ctx context.Context, id uuid.UUID) (io.ReadSeekCloser, error) {
	file, err := c.FileStore.Open(ctx, id)
	if err != nil {
		return nil, err
	}
	return countedFile{ReadSeekCloser: file, closed: c.closed}, nil
}

// countedFile is an open file that counts its close.
type countedFile struct {
	io.ReadSeekCloser
	// closed counts the closes.
	closed *atomic.Int32
}

// Close counts the close and closes the file.
func (f countedFile) Close() error {
	f.closed.Add(1)
	return f.ReadSeekCloser.Close()
}

func TestTheAttachmentRouteServesAKeptFileWithTheDownloadHeaders(t *testing.T) {
	t.Parallel()

	dt := newDownloadTest(t)
	dt.ingest(dt.incoming(filesMail(), "file:quote-attached"))
	_, files := dt.quoteFiles(sdk.DefaultTenantID)

	for name, c := range map[string]struct {
		id                                uuid.UUID
		content, contentType, disposition string
	}{
		"a document saved under its safe name": {
			id: files[0], content: attachedPDF, contentType: "application/pdf",
			disposition: "attachment; filename=quote.pdf",
		},
		"a photo shown inline": {id: files[2], content: attachedPNG, contentType: "image/png", disposition: "inline"},
	} {
		recorder := getFile(t, dt.routes, sdk.DefaultTenantID, attachmentPath(c.id), nil)

		if recorder.Code != http.StatusOK || recorder.Body.String() != c.content {
			t.Fatalf("%s answered %d %q, want 200 %q", name, recorder.Code, recorder.Body.String(), c.content)
		}
		wantHeaders(t, recorder.Header(), guardHeaders)
		wantHeaders(t, recorder.Header(), map[string]string{
			"Content-Type": c.contentType, "Content-Disposition": c.disposition,
			"ETag": `"` + hex.EncodeToString(sum(c.content)) + `"`,
		})
		if recorder.Header().Get("Last-Modified") == "" {
			t.Errorf("%s answered no Last-Modified, want the time the file was kept", name)
		}
	}
}

func TestTheAttachmentRouteAnswersARangeWithThatPartOfTheFile(t *testing.T) {
	t.Parallel()

	dt := newDownloadTest(t)
	dt.ingest(dt.incoming(filesMail(), "file:quote-attached"))
	_, files := dt.quoteFiles(sdk.DefaultTenantID)

	recorder := getFile(t, dt.routes, sdk.DefaultTenantID, attachmentPath(files[0]), http.Header{"Range": {"bytes=0-3"}})

	if recorder.Code != http.StatusPartialContent || recorder.Body.String() != "%PDF" ||
		recorder.Header().Get("Content-Range") != "bytes 0-3/14" {
		t.Errorf("a range of the PDF answered %d %q %q, want 206 %q of bytes 0-3/14",
			recorder.Code, recorder.Body.String(), recorder.Header().Get("Content-Range"), "%PDF")
	}
}

func TestTheOriginalRouteServesTheWholeMailUnderANameWithoutItsSubject(t *testing.T) {
	t.Parallel()

	dt := newDownloadTest(t)
	raw := filesMail()
	dt.ingest(dt.incoming(raw, "file:quote-attached"))
	mail, _ := dt.quoteFiles(sdk.DefaultTenantID)

	recorder := getFile(t, dt.routes, sdk.DefaultTenantID, originalPath(mail), nil)

	if recorder.Code != http.StatusOK || !bytes.Equal(recorder.Body.Bytes(), raw) {
		t.Fatalf("the original answered %d %q, want 200 and the mail as it came", recorder.Code, recorder.Body.String())
	}
	wantHeaders(t, recorder.Header(), guardHeaders)
	wantHeaders(t, recorder.Header(), map[string]string{
		"Content-Type":        "message/rfc822",
		"Content-Disposition": "attachment; filename=original-" + mail.String() + ".eml",
		"ETag":                `"` + hex.EncodeToString(sum(string(raw))) + `"`,
	})
}

func TestTheDownloadRoutesRejectAnIDThatIsNoUUID(t *testing.T) {
	t.Parallel()

	routes := (&downloads{}).routes()

	for _, target := range []string{"/attachments/not-a-uuid", "/messages/not-a-uuid/original"} {
		if recorder := getFile(t, routes, sdk.DefaultTenantID, target, nil); recorder.Code != http.StatusBadRequest {
			t.Errorf("%s answered %d, want %d", target, recorder.Code, http.StatusBadRequest)
		}
	}
}

func TestTheDownloadRoutesAnswerNotFoundForAFileTheWorkspaceDoesNotKeep(t *testing.T) {
	t.Parallel()

	dt := newDownloadTest(t)
	acme, box := dt.addWorkspace()
	dt.ingest(dt.incoming(filesMail(), "file:quote-attached"))
	dt.ingestIn(acme, box, filesMail(), "file:quote-attached")
	mail, files := dt.quoteFiles(sdk.DefaultTenantID)
	acmeMail, acmeFiles := dt.quoteFiles(acme)
	full := newDownloadTest(t, func(s *settings) { s.files.workspaceMaxBytes = 1 })
	full.ingest(full.incoming(filesMail(), "file:quote-attached"))
	fullMail, fullFiles := full.quoteFiles(sdk.DefaultTenantID)
	unknown := uuid.Must(uuid.NewV7())
	held, fullHeld := dt.attachmentsOf(mail), full.attachmentsOf(fullMail)
	if held[1].state != fileTooLarge || fullHeld[0].state != fileOverLimit ||
		storedRow(t, full.pool, fullMail).rawState != fileOverLimit {
		t.Fatalf("the files are %+v and %+v, want big.bin too large and the full workspace's files over the limit",
			held, fullHeld)
	}

	for name, c := range map[string]struct {
		routes http.Handler
		target string
	}{
		"an unknown attachment":                  {routes: dt.routes, target: attachmentPath(unknown)},
		"an attachment of another workspace":     {routes: dt.routes, target: attachmentPath(acmeFiles[0])},
		"an attachment too large to keep":        {routes: dt.routes, target: attachmentPath(files[1])},
		"an attachment past the workspace limit": {routes: full.routes, target: attachmentPath(fullFiles[0])},
		"an unknown original":                    {routes: dt.routes, target: originalPath(unknown)},
		"the original of another workspace":      {routes: dt.routes, target: originalPath(acmeMail)},
		"an original past the workspace limit":   {routes: full.routes, target: originalPath(fullMail)},
	} {
		if recorder := getFile(t, c.routes, sdk.DefaultTenantID, c.target, nil); recorder.Code != http.StatusNotFound {
			t.Errorf("%s answered %d, want %d", name, recorder.Code, http.StatusNotFound)
		}
	}
	for name, target := range map[string]string{
		"Acme's own attachment": attachmentPath(acmeFiles[0]), "Acme's own original": originalPath(acmeMail),
	} {
		if recorder := getFile(t, dt.routes, acme, target, nil); recorder.Code != http.StatusOK {
			t.Errorf("%s asked in Acme answered %d, want %d", name, recorder.Code, http.StatusOK)
		}
	}
}

func TestTheDownloadRoutesAnswerNotFoundForAFileTheStoreLost(t *testing.T) {
	t.Parallel()

	dt := newDownloadTest(t)
	dt.ingest(dt.incoming(filesMail(), "file:quote-attached"))
	mail, files := dt.quoteFiles(sdk.DefaultTenantID)
	ctx := sdk.WithTenant(t.Context(), sdk.DefaultTenantID)
	for _, id := range []uuid.UUID{dt.attachmentsOf(mail)[0].fileID, storedRow(t, dt.pool, mail).rawFileID} {
		if err := dt.store.Delete(ctx, id); err != nil {
			t.Fatalf("deleting the file %v: %v", id, err)
		}
	}

	for _, target := range []string{attachmentPath(files[0]), originalPath(mail)} {
		if recorder := getFile(t, dt.routes, sdk.DefaultTenantID, target, nil); recorder.Code != http.StatusNotFound {
			t.Errorf("%s of a lost file answered %d, want %d", target, recorder.Code, http.StatusNotFound)
		}
	}
}

func TestTheDownloadRoutesAnswerAServerErrorForAStoreThatFails(t *testing.T) {
	t.Parallel()

	dt := newDownloadTest(t)
	dt.ingest(dt.incoming(filesMail(), "file:quote-attached"))
	mail, files := dt.quoteFiles(sdk.DefaultTenantID)
	routes := (&downloads{db: dt.pool, store: openFailingStore{FileStore: dt.store}}).routes()

	for _, target := range []string{attachmentPath(files[0]), originalPath(mail)} {
		if recorder := getFile(t, routes, sdk.DefaultTenantID, target, nil); recorder.Code != http.StatusInternalServerError {
			t.Errorf("%s over a failing store answered %d, want %d", target, recorder.Code, http.StatusInternalServerError)
		}
	}
}

func TestTheDownloadRoutesAnswerAServerErrorForAClosedPool(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	pool.Close()
	routes := (&downloads{db: pool}).routes()
	id := uuid.Must(uuid.NewV7())

	for _, target := range []string{attachmentPath(id), originalPath(id)} {
		if recorder := getFile(t, routes, sdk.DefaultTenantID, target, nil); recorder.Code != http.StatusInternalServerError {
			t.Errorf("%s over a closed pool answered %d, want %d", target, recorder.Code, http.StatusInternalServerError)
		}
	}
}

func TestTheDownloadRoutesCloseTheFileAfterTheAnswer(t *testing.T) {
	t.Parallel()

	dt := newDownloadTest(t)
	dt.ingest(dt.incoming(filesMail(), "file:quote-attached"))
	mail, files := dt.quoteFiles(sdk.DefaultTenantID)
	var closed atomic.Int32
	routes := (&downloads{db: dt.pool, store: closeCountingStore{FileStore: dt.store, closed: &closed}}).routes()

	for _, target := range []string{attachmentPath(files[0]), originalPath(mail)} {
		if recorder := getFile(t, routes, sdk.DefaultTenantID, target, nil); recorder.Code != http.StatusOK {
			t.Errorf("%s answered %d, want %d", target, recorder.Code, http.StatusOK)
		}
	}

	if got := closed.Load(); got != 2 {
		t.Errorf("the routes closed %d files, want both they opened", got)
	}
}

func TestDownloadNameKeepsTheLastPartWithNoControlCharacterOrEndDot(t *testing.T) {
	t.Parallel()

	for name, c := range map[string]struct{ filename, want string }{
		"a plain name":                     {filename: "quote.pdf", want: "quote.pdf"},
		"dots inside the name":             {filename: "quote.v2.pdf", want: "quote.v2.pdf"},
		"a path":                           {filename: "../../etc/passwd", want: "passwd"},
		"a backslash path":                 {filename: `..\..\quote.pdf`, want: "quote.pdf"},
		"control characters":               {filename: "quote\t\x1b\x7f.pdf", want: "quote.pdf"},
		"a right to left override":         {filename: "quote\xe2\x80\xaefdp.exe", want: "quotefdp.exe"},
		"dots and blanks at the ends":      {filename: " ..quote.pdf. . ", want: "quote.pdf"},
		"a path that ends in a slash":      {filename: "files/", want: "attachment"},
		"only dots":                        {filename: "../..", want: "attachment"},
		"a name of format characters only": {filename: "\xe2\x80\x8b\xef\xbb\xbf", want: "attachment"},
		"an empty name":                    {filename: "", want: "attachment"},
	} {
		if got := downloadName(c.filename); got != c.want {
			t.Errorf("downloadName() of %s %q = %q, want %q", name, c.filename, got, c.want)
		}
	}
}
