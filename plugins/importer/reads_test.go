// SPDX-License-Identifier: Elastic-2.0

package importer_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/plugins/importer"
)

// getPath issues a GET against the plugin routes.
func getPath(t *testing.T, p *importer.Plugin, path string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	recorder := httptest.NewRecorder()
	p.Routes().ServeHTTP(recorder, request)
	return recorder
}

// putMapping issues a PUT of the mapping body for one import.
func putMapping(t *testing.T, p *importer.Plugin, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPut, "/imports/"+id+"/mapping", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	p.Routes().ServeHTTP(recorder, request)
	return recorder
}

// decodeInto reads a JSON response body into v.
func decodeInto(t *testing.T, recorder *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.NewDecoder(recorder.Body).Decode(v); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
}

// uploadNamed stores one CSV upload under the given filename and returns its id.
func uploadNamed(t *testing.T, p *importer.Plugin, filename, content string) uuid.UUID {
	t.Helper()
	body, contentType := uploadBody(t, "file", filename, []byte(content))
	recorder := postUpload(t, p, uuid.Must(uuid.NewV7()), contentType, body)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, want %d, body %s", recorder.Code, http.StatusCreated, recorder.Body)
	}
	return decodeImport(t, recorder).ID
}

type fieldBody struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
}

type summaryBody struct {
	ID            uuid.UUID `json:"id"`
	UserID        uuid.UUID `json:"user_id"`
	Filename      string    `json:"filename"`
	State         string    `json:"state"`
	RowCount      int       `json:"row_count"`
	ImportedCount int       `json:"imported_count"`
	SkippedCount  int       `json:"skipped_count"`
	FailedCount   int       `json:"failed_count"`
}

type detailBody struct {
	summaryBody
	Columns []string          `json:"columns"`
	Mapping map[string]string `json:"mapping"`
}

type rowBody struct {
	ID        uuid.UUID  `json:"id"`
	Position  int        `json:"position"`
	Cells     []string   `json:"cells"`
	Outcome   string     `json:"outcome"`
	Reason    *string    `json:"reason"`
	ContactID *uuid.UUID `json:"contact_id"`
}

type contactsBody struct {
	Contacts []struct {
		ContactID uuid.UUID `json:"contact_id"`
		Name      string    `json:"name"`
		RowID     uuid.UUID `json:"row_id"`
	} `json:"contacts"`
}

func TestFieldsServesTheMappableRegistry(t *testing.T) {
	t.Parallel()

	p, _ := newUploadPlugin(t)

	recorder := getPath(t, p, "/fields")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var got []fieldBody
	decodeInto(t, recorder, &got)
	want := []fieldBody{
		{Name: "name", Label: "Name", Required: true},
		{Name: "email", Label: "Email", Required: false},
		{Name: "phone", Label: "Phone", Required: false},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("fields mismatch (-want +got):\n%s", diff)
	}
}

func TestListImportsServesEveryImportNewestFirst(t *testing.T) {
	t.Parallel()

	p, _ := newUploadPlugin(t)
	uploadNamed(t, p, "first.csv", commaCSV)
	newest := uploadNamed(t, p, "second.csv", commaCSV)

	recorder := getPath(t, p, "/imports")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var got []summaryBody
	decodeInto(t, recorder, &got)
	if len(got) != 2 {
		t.Fatalf("imports = %d, want 2", len(got))
	}
	if got[0].ID != newest {
		t.Errorf("first entry = %v, want the newest import %v", got[0].ID, newest)
	}
	if got[0].Filename != "second.csv" {
		t.Errorf("filename = %q, want second.csv", got[0].Filename)
	}
	if got[0].UserID == uuid.Nil {
		t.Error("user_id is empty, want the uploader recorded")
	}
	if got[0].State != "ready" || got[0].RowCount != 1 {
		t.Errorf("state = %q, row_count = %d, want ready and 1", got[0].State, got[0].RowCount)
	}
}

func TestListImportsIsAnEmptyArrayWithoutImports(t *testing.T) {
	t.Parallel()

	p, _ := newUploadPlugin(t)

	recorder := getPath(t, p, "/imports")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if body := strings.TrimSpace(recorder.Body.String()); body != "[]" {
		t.Errorf("body = %s, want an empty array", body)
	}
}

func TestGetImportServesItsColumnsAndMapping(t *testing.T) {
	t.Parallel()

	p, _ := newUploadPlugin(t)
	uploadNamed(t, p, "decoy.csv", commaCSV)
	wanted := uploadNamed(t, p, "wanted.csv", "Name,Email,Phone\nMaria Perez,maria@example.com,184467235\n")

	recorder := getPath(t, p, "/imports/"+wanted.String())

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var got detailBody
	decodeInto(t, recorder, &got)
	if got.ID != wanted || got.Filename != "wanted.csv" {
		t.Errorf("import = %v %q, want the requested one", got.ID, got.Filename)
	}
	if diff := cmp.Diff([]string{"Name", "Email", "Phone"}, got.Columns); diff != "" {
		t.Errorf("columns mismatch (-want +got):\n%s", diff)
	}
	if len(got.Mapping) != 0 {
		t.Errorf("mapping = %v, want empty before it is assigned", got.Mapping)
	}
}

func TestImportReadsRejectMalformedAndUnknownIDs(t *testing.T) {
	t.Parallel()

	suffixes := []string{"", "/rows", "/contacts"}
	for _, suffix := range suffixes {
		t.Run("malformed"+suffix, func(t *testing.T) {
			t.Parallel()

			p, _ := newUploadPlugin(t)

			recorder := getPath(t, p, "/imports/not-a-uuid"+suffix)

			if recorder.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
		})
		t.Run("unknown"+suffix, func(t *testing.T) {
			t.Parallel()

			p, _ := newUploadPlugin(t)

			recorder := getPath(t, p, "/imports/"+uuid.Must(uuid.NewV7()).String()+suffix)

			if recorder.Code != http.StatusNotFound {
				t.Errorf("status = %d, want %d", recorder.Code, http.StatusNotFound)
			}
		})
	}
}

func TestListRowsServesEveryRowInPositionOrder(t *testing.T) {
	t.Parallel()

	p, pool := newUploadPlugin(t)
	id := uploadNamed(t, p, "ragged.csv",
		"Name,Email,Phone\nMaria Perez\nAna Lopez,ana@example.com,184467235,extra\nJuan Ruiz,juan@example.com,184467236\n")
	if _, err := pool.Exec(t.Context(),
		"UPDATE plugin_importer.import_rows SET outcome = 'skipped', reason = 'already known' "+
			"WHERE import_id = $1 AND position = 3", id,
	); err != nil {
		t.Fatalf("marking a row: %v", err)
	}

	recorder := getPath(t, p, "/imports/"+id.String()+"/rows")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var got []rowBody
	decodeInto(t, recorder, &got)
	if len(got) != 3 {
		t.Fatalf("rows = %d, want 3", len(got))
	}
	for i, row := range got {
		if row.Position != i+1 {
			t.Errorf("row %d position = %d, want %d, rows must arrive in position order", i, row.Position, i+1)
		}
	}
	if len(got[1].Cells) != 4 {
		t.Errorf("long row cells = %d, want all 4 preserved beside a 3 column header", len(got[1].Cells))
	}
	if got[0].Reason == nil {
		t.Error("short row reason is null, want the mismatch recorded")
	}
	if got[2].Outcome != "skipped" || got[2].Reason == nil || *got[2].Reason != "already known" {
		t.Errorf("row 3 = %q %v, want the stored outcome and reason", got[2].Outcome, got[2].Reason)
	}
}

// seedImportedContacts marks import rows with outcomes and contact links.
func seedImportedContacts(t *testing.T, pool *pgxpool.Pool, importID uuid.UUID) uuid.UUID {
	t.Helper()
	kept := uuid.Must(uuid.NewV7())
	removed := uuid.Must(uuid.NewV7())
	for _, pair := range []struct {
		id   uuid.UUID
		name string
	}{{kept, "Maria Perez"}, {removed, "Ana Lopez"}} {
		if _, err := pool.Exec(t.Context(),
			"INSERT INTO core.contacts (id, name, created_at) VALUES ($1, $2, now())",
			pair.id, pair.name,
		); err != nil {
			t.Fatalf("inserting contact: %v", err)
		}
	}
	updates := []struct {
		position  int
		outcome   string
		contactID *uuid.UUID
	}{
		{1, "imported", &kept},
		{2, "skipped", &kept},
		{3, "imported", &removed},
	}
	for _, update := range updates {
		if _, err := pool.Exec(t.Context(),
			"UPDATE plugin_importer.import_rows SET outcome = $2, contact_id = $3 "+
				"WHERE import_id = $1 AND position = $4",
			importID, update.outcome, update.contactID, update.position,
		); err != nil {
			t.Fatalf("marking a row: %v", err)
		}
	}
	if _, err := pool.Exec(t.Context(), "DELETE FROM core.contacts WHERE id = $1", removed); err != nil {
		t.Fatalf("deleting contact: %v", err)
	}
	return kept
}

func TestImportedContactsServeOnlyLinkedImportedRows(t *testing.T) {
	t.Parallel()

	p, pool := newUploadPlugin(t)
	id := uploadNamed(t, p, "contacts.csv",
		"Name\nMaria Perez\nAna Lopez\nJuan Ruiz\nRosa Diaz\n")
	kept := seedImportedContacts(t, pool, id)

	recorder := getPath(t, p, "/imports/"+id.String()+"/contacts")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var got contactsBody
	decodeInto(t, recorder, &got)
	if len(got.Contacts) != 1 {
		t.Fatalf("contacts = %d, want only the imported row that still holds a link", len(got.Contacts))
	}
	if got.Contacts[0].ContactID != kept {
		t.Errorf("contact_id = %v, want %v", got.Contacts[0].ContactID, kept)
	}
	if got.Contacts[0].Name != "Maria Perez" {
		t.Errorf("name = %q, want the contact name from core", got.Contacts[0].Name)
	}
	if got.Contacts[0].RowID == uuid.Nil {
		t.Error("row_id is empty, want the source row named")
	}
}

func TestImportedContactsAreAnEmptyArrayWithoutAny(t *testing.T) {
	t.Parallel()

	p, _ := newUploadPlugin(t)
	id := uploadNamed(t, p, "contacts.csv", commaCSV)

	recorder := getPath(t, p, "/imports/"+id.String()+"/contacts")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if body := strings.TrimSpace(recorder.Body.String()); body != `{"contacts":[]}` {
		t.Errorf("body = %s, want an envelope holding an empty array", body)
	}
}

func TestPutMappingStoresTheAssignments(t *testing.T) {
	t.Parallel()

	p, _ := newUploadPlugin(t)
	id := uploadNamed(t, p, "contacts.csv", "Name,Email\nMaria Perez,maria@example.com\n")

	recorder := putMapping(t, p, id.String(),
		`{"assignments":[{"column":0,"field":"name"},{"column":1,"field":"email"}]}`)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusNoContent, recorder.Body)
	}
	var got detailBody
	decodeInto(t, getPath(t, p, "/imports/"+id.String()), &got)
	want := map[string]string{"0": "name", "1": "email"}
	if diff := cmp.Diff(want, got.Mapping); diff != "" {
		t.Errorf("stored mapping mismatch (-want +got):\n%s", diff)
	}
}

func TestPutMappingRejectsUnusableAssignments(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		body       string
		wantStatus int
	}{
		"malformed json": {
			body:       `{"assignments":`,
			wantStatus: http.StatusBadRequest,
		},
		"unknown field": {
			body:       `{"assignments":[{"column":0,"field":"name"},{"column":1,"field":"nickname"}]}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		"missing the required field": {
			body:       `{"assignments":[{"column":1,"field":"email"}]}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		"no assignments at all": {
			body:       `{"assignments":[]}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		"column past the header": {
			body:       `{"assignments":[{"column":0,"field":"name"},{"column":9,"field":"email"}]}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		"negative column": {
			body:       `{"assignments":[{"column":0,"field":"name"},{"column":-1,"field":"email"}]}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		"two columns claiming one field": {
			body:       `{"assignments":[{"column":0,"field":"name"},{"column":1,"field":"name"}]}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		"two assignments claiming one column": {
			body:       `{"assignments":[{"column":0,"field":"name"},{"column":0,"field":"email"}]}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p, _ := newUploadPlugin(t)
			id := uploadNamed(t, p, "contacts.csv", "Name,Email\nMaria Perez,maria@example.com\n")

			recorder := putMapping(t, p, id.String(), tc.body)

			if recorder.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d, body %s", recorder.Code, tc.wantStatus, recorder.Body)
			}
			var got detailBody
			decodeInto(t, getPath(t, p, "/imports/"+id.String()), &got)
			if len(got.Mapping) != 0 {
				t.Errorf("mapping = %v, want it left unassigned", got.Mapping)
			}
		})
	}
}

func TestPutMappingRejectsAnImportThatIsNoLongerReady(t *testing.T) {
	t.Parallel()

	for _, state := range []string{"committing", "committed"} {
		t.Run(state, func(t *testing.T) {
			t.Parallel()

			p, pool := newUploadPlugin(t)
			id := uploadNamed(t, p, "contacts.csv", "Name,Email\nMaria Perez,maria@example.com\n")
			if _, err := pool.Exec(t.Context(),
				"UPDATE plugin_importer.imports SET state = $2 WHERE id = $1", id, state,
			); err != nil {
				t.Fatalf("moving the import state: %v", err)
			}

			recorder := putMapping(t, p, id.String(),
				`{"assignments":[{"column":0,"field":"name"}]}`)

			if recorder.Code != http.StatusConflict {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusConflict)
			}
		})
	}
}

func TestPutMappingRejectsMalformedAndUnknownIDs(t *testing.T) {
	t.Parallel()

	body := `{"assignments":[{"column":0,"field":"name"}]}`
	tests := map[string]struct {
		id         string
		wantStatus int
	}{
		"malformed id": {id: "not-a-uuid", wantStatus: http.StatusBadRequest},
		"unknown id":   {id: uuid.Must(uuid.NewV7()).String(), wantStatus: http.StatusNotFound},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p, _ := newUploadPlugin(t)

			recorder := putMapping(t, p, tc.id, body)

			if recorder.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tc.wantStatus)
			}
		})
	}
}

func TestRowReadsReportAFailureBehindAReadableImport(t *testing.T) {
	t.Parallel()

	p, pool := newUploadPlugin(t)
	id := uploadNamed(t, p, "contacts.csv", commaCSV)
	if _, err := pool.Exec(t.Context(), "DROP TABLE plugin_importer.import_rows"); err != nil {
		t.Fatalf("dropping the rows table: %v", err)
	}

	if recorder := getPath(t, p, "/imports/"+id.String()); recorder.Code != http.StatusOK {
		t.Fatalf("the import itself status = %d, want it still readable", recorder.Code)
	}
	for _, suffix := range []string{"/rows", "/contacts"} {
		recorder := getPath(t, p, "/imports/"+id.String()+suffix)

		if recorder.Code != http.StatusInternalServerError {
			t.Errorf("GET %s status = %d, want %d", suffix, recorder.Code, http.StatusInternalServerError)
		}
	}
}

func TestPutMappingReportsAFailedWrite(t *testing.T) {
	t.Parallel()

	p, pool := newUploadPlugin(t)
	id := uploadNamed(t, p, "contacts.csv", "Name,Email\nMaria Perez,maria@example.com\n")
	if _, err := pool.Exec(t.Context(),
		"ALTER TABLE plugin_importer.imports ADD CONSTRAINT mapping_stays_empty "+
			"CHECK (mapping = '{}'::jsonb)"); err != nil {
		t.Fatalf("adding the constraint: %v", err)
	}

	recorder := putMapping(t, p, id.String(), `{"assignments":[{"column":0,"field":"name"}]}`)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

func TestReadEndpointsReportAStorageFailure(t *testing.T) {
	t.Parallel()

	cfg := newTestDatabase(t)
	p := newPlugin(t, cfg.URL())
	id := uuid.Must(uuid.NewV7()).String()

	for _, path := range []string{"/imports", "/imports/" + id, "/imports/" + id + "/rows",
		"/imports/" + id + "/contacts"} {
		recorder := getPath(t, p, path)

		if recorder.Code != http.StatusInternalServerError {
			t.Errorf("GET %s status = %d, want %d", path, recorder.Code, http.StatusInternalServerError)
		}
	}
	recorder := putMapping(t, p, id, `{"assignments":[{"column":0,"field":"name"}]}`)
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("PUT mapping status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if recorder := getPath(t, p, "/fields"); recorder.Code != http.StatusOK {
		t.Errorf("GET /fields status = %d, want it served without a database", recorder.Code)
	}
}
