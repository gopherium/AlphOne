// SPDX-License-Identifier: Elastic-2.0

package importer_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/plugins/importer"
	"github.com/gopherium/alphone/sdk"
)

// commaCSV is a comma separated export with an aligned header and row.
const commaCSV = "Name,Email\nMaria Perez,maria@example.com\n"

// uploadBody returns a multipart body carrying one named part and its content type.
func uploadBody(t *testing.T, field, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(field, filename)
	if err != nil {
		t.Fatalf("creating form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("writing form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing writer: %v", err)
	}
	return &body, writer.FormDataContentType()
}

// postUpload sends a multipart upload as the given user and returns the response.
func postUpload(
	t *testing.T, p *importer.Plugin, userID uuid.UUID, contentType string, body io.Reader,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/imports", body)
	request.Header.Set("Content-Type", contentType)
	if userID != uuid.Nil {
		request = request.WithContext(sdk.WithUser(request.Context(), userID))
	}
	recorder := httptest.NewRecorder()
	p.Routes().ServeHTTP(recorder, request)
	return recorder
}

// uploadCSV posts one CSV upload and returns the response.
func uploadCSV(
	t *testing.T, p *importer.Plugin, userID uuid.UUID, content string,
) *httptest.ResponseRecorder {
	t.Helper()
	body, contentType := uploadBody(t, "file", "contacts.csv", []byte(content))
	return postUpload(t, p, userID, contentType, body)
}

// newUploadPlugin returns a migrated plugin and an assertion pool over its database.
func newUploadPlugin(t *testing.T) (*importer.Plugin, *pgxpool.Pool) {
	t.Helper()
	cfg := newTestDatabase(t)
	p := newPlugin(t, cfg.URL())
	if err := p.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	return p, newAssertionPool(t, cfg.URL())
}

type importBody struct {
	ID            uuid.UUID `json:"id"`
	UserID        uuid.UUID `json:"user_id"`
	Filename      string    `json:"filename"`
	State         string    `json:"state"`
	Columns       []string  `json:"columns"`
	RowCount      int       `json:"row_count"`
	ImportedCount int       `json:"imported_count"`
	SkippedCount  int       `json:"skipped_count"`
	FailedCount   int       `json:"failed_count"`
	CreatedAt     time.Time `json:"created_at"`
}

// decodeImport reads the import a successful upload answers with.
func decodeImport(t *testing.T, recorder *httptest.ResponseRecorder) importBody {
	t.Helper()
	var body importBody
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	return body
}

func TestUploadStoresTheImportAndItsRows(t *testing.T) {
	t.Parallel()

	p, pool := newUploadPlugin(t)
	uploader := uuid.Must(uuid.NewV7())

	recorder := uploadCSV(t, p, uploader, commaCSV)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusCreated, recorder.Body)
	}
	got := decodeImport(t, recorder)
	if got.State != "ready" {
		t.Errorf("state = %q, want ready", got.State)
	}
	if got.Filename != "contacts.csv" {
		t.Errorf("filename = %q, want contacts.csv", got.Filename)
	}
	if got.RowCount != 1 {
		t.Errorf("row_count = %d, want 1", got.RowCount)
	}
	if got.UserID != uploader {
		t.Errorf("user_id = %v, want the uploader %v", got.UserID, uploader)
	}
	if got.CreatedAt.IsZero() {
		t.Error("created_at is zero, want the moment the import was stored")
	}
	if got.ImportedCount != 0 || got.SkippedCount != 0 || got.FailedCount != 0 {
		t.Errorf("counts = %d, %d, %d, want them all zero before a commit",
			got.ImportedCount, got.SkippedCount, got.FailedCount)
	}
	var storedUser uuid.UUID
	var storedColumns []string
	var cells []string
	var position int
	var outcome string
	if err := pool.QueryRow(t.Context(),
		"SELECT i.user_id, i.columns, r.position, r.outcome, "+
			"ARRAY(SELECT jsonb_array_elements_text(r.cells)) "+
			"FROM plugin_importer.imports i "+
			"JOIN plugin_importer.import_rows r ON r.import_id = i.id WHERE i.id = $1",
		got.ID,
	).Scan(&storedUser, &storedColumns, &position, &outcome, &cells); err != nil {
		t.Fatalf("reading the stored import: %v", err)
	}
	if storedUser != uploader {
		t.Errorf("user_id = %v, want the uploader %v", storedUser, uploader)
	}
	if strings.Join(storedColumns, ",") != "Name,Email" {
		t.Errorf("columns = %v, want the header row", storedColumns)
	}
	if position != 1 {
		t.Errorf("position = %d, want 1", position)
	}
	if outcome != "pending" {
		t.Errorf("outcome = %q, want pending", outcome)
	}
	if strings.Join(cells, ",") != "Maria Perez,maria@example.com" {
		t.Errorf("cells = %v, want the data row", cells)
	}
}

func TestUploadRecordsARaggedRowReason(t *testing.T) {
	t.Parallel()

	p, pool := newUploadPlugin(t)

	recorder := uploadCSV(t, p, uuid.Must(uuid.NewV7()),
		"Name,Email\nMaria Perez\n")

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusCreated, recorder.Body)
	}
	var reason *string
	if err := pool.QueryRow(t.Context(),
		"SELECT reason FROM plugin_importer.import_rows WHERE import_id = $1",
		decodeImport(t, recorder).ID,
	).Scan(&reason); err != nil {
		t.Fatalf("reading the row reason: %v", err)
	}
	if reason == nil || *reason == "" {
		t.Error("reason is empty, want the ragged row recorded")
	}
}

func TestUploadRejectsABodyBeyondTheSizeCap(t *testing.T) {
	t.Parallel()

	p, pool := newUploadPlugin(t)
	oversized := strings.Repeat("Maria Perez,maria@example.com\n", 200000)

	recorder := uploadCSV(t, p, uuid.Must(uuid.NewV7()), "Name,Email\n"+oversized)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnprocessableEntity)
	}
	var stored int
	if err := pool.QueryRow(t.Context(),
		"SELECT count(*) FROM plugin_importer.imports").Scan(&stored); err != nil {
		t.Fatalf("counting imports: %v", err)
	}
	if stored != 0 {
		t.Errorf("stored %d imports, want none", stored)
	}
}

func TestUploadRejectsABodyOfDecoyPartsBeyondTheSizeCap(t *testing.T) {
	t.Parallel()

	p, _ := newUploadPlugin(t)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("notes", "notes.txt")
	if err != nil {
		t.Fatalf("creating form file: %v", err)
	}
	if _, err := part.Write([]byte(strings.Repeat("x", 6<<20))); err != nil {
		t.Fatalf("writing form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing writer: %v", err)
	}

	recorder := postUpload(t, p, uuid.Must(uuid.NewV7()), writer.FormDataContentType(), &body)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnprocessableEntity)
	}
}

func TestUploadRejectsMalformedRequests(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		field       string
		contentType string
		content     []byte
		wantStatus  int
	}{
		"missing file field": {
			field:      "notes",
			content:    []byte(commaCSV),
			wantStatus: http.StatusBadRequest,
		},
		"unsupported format": {
			field:      "file",
			content:    []byte{0x89, 0x50, 0x4E, 0x47, 0x00},
			wantStatus: http.StatusUnprocessableEntity,
		},
		"not multipart": {
			field:       "file",
			contentType: "application/json",
			content:     []byte(commaCSV),
			wantStatus:  http.StatusBadRequest,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p, _ := newUploadPlugin(t)
			body, contentType := uploadBody(t, tc.field, "contacts.csv", tc.content)
			if tc.contentType != "" {
				contentType = tc.contentType
			}

			recorder := postUpload(t, p, uuid.Must(uuid.NewV7()), contentType, body)

			if recorder.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d, body %s", recorder.Code, tc.wantStatus, recorder.Body)
			}
		})
	}
}

func TestUploadRequiresAnActingUser(t *testing.T) {
	t.Parallel()

	p, _ := newUploadPlugin(t)

	recorder := uploadCSV(t, p, uuid.Nil, commaCSV)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestUploadReportsAStorageFailure(t *testing.T) {
	t.Parallel()

	cfg := newTestDatabase(t)
	p := newPlugin(t, cfg.URL())

	recorder := uploadCSV(t, p, uuid.Must(uuid.NewV7()), commaCSV)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}
