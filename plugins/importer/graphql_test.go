// SPDX-License-Identifier: Elastic-2.0

package importer_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/graphroot"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/plugins/fields"
	"github.com/gopherium/alphone/plugins/importer"
	"github.com/gopherium/alphone/plugins/whatsapp"
	"github.com/gopherium/alphone/sdk"
)

// twoRowCSV is a comma separated export staging two contacts.
const twoRowCSV = "Name,Email\nMaria Perez,maria@example.com\nNoah Chen,noah@example.com\n"

// uploadDocument is the mutation the graph upload tests post.
const uploadDocument = `mutation($file: Upload!) {
	importUpload(file: $file) { id filename state rowCount columns mapping { column field } }
}`

// newGraphHandler returns the composed graph handler acting as the given user.
func newGraphHandler(t *testing.T, p *importer.Plugin, pool *pgxpool.Pool, user uuid.UUID) http.Handler {
	t.Helper()
	whatsappPlugin, err := whatsapp.Register(sdk.Deps{DatabaseURL: "postgres://graph:graph@localhost:1/graph"})
	if err != nil {
		t.Fatalf("whatsapp.Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = whatsappPlugin.Stop(t.Context()) })
	fieldsPlugin, err := fields.Register(sdk.Deps{DatabaseURL: "postgres://graph:graph@localhost:1/graph"})
	if err != nil {
		t.Fatalf("fields.Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = fieldsPlugin.Stop(t.Context()) })
	root, err := graphroot.FromPlugins(&graphres.Resolver{
		Version:  "9.9.9",
		Contacts: postgres.NewContactStore(pool),
	}, []sdk.Plugin{whatsappPlugin, p, fieldsPlugin})
	if err != nil {
		t.Fatalf("FromPlugins() error = %v, want nil", err)
	}
	srv := handler.New(graphres.ExecutableSchema(root))
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.MultipartForm{})
	srv.SetErrorPresenter(graphres.PresentError)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := sdk.WithRequestScope(r.Context(), sdk.NewRequestScope())
		if user != uuid.Nil {
			ctx = sdk.WithUser(ctx, user)
		}
		srv.ServeHTTP(w, r.WithContext(ctx))
	})
}

// graphResponse is a graph response body with raw data and typed errors.
type graphResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message    string         `json:"message"`
		Extensions map[string]any `json:"extensions"`
	} `json:"errors"`
}

// decodeGraph reads a graph response body.
func decodeGraph(t *testing.T, recorder *httptest.ResponseRecorder) graphResponse {
	t.Helper()
	var response graphResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decoding graph response: %v", err)
	}
	return response
}

// firstCode returns the first error's extensions code of a graph response.
func firstCode(t *testing.T, response graphResponse) string {
	t.Helper()
	if len(response.Errors) == 0 {
		t.Fatalf("errors are empty, want at least one: %s", response.Data)
	}
	code, _ := response.Errors[0].Extensions["code"].(string)
	return code
}

// postGraph posts a JSON graph operation with variables.
func postGraph(
	t *testing.T, graphHandler http.Handler, query string, variables map[string]any,
) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		t.Fatalf("encoding graph body: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/graphql", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	graphHandler.ServeHTTP(recorder, request)
	return recorder
}

// graphUpload posts one CSV file through the graph multipart transport.
func graphUpload(t *testing.T, graphHandler http.Handler, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	operations, err := json.Marshal(map[string]any{
		"query":     uploadDocument,
		"variables": map[string]any{"file": nil},
	})
	if err != nil {
		t.Fatalf("encoding operations: %v", err)
	}
	if err := writer.WriteField("operations", string(operations)); err != nil {
		t.Fatalf("writing operations: %v", err)
	}
	if err := writer.WriteField("map", `{"0":["variables.file"]}`); err != nil {
		t.Fatalf("writing map: %v", err)
	}
	part, err := writer.CreateFormFile("0", "contacts.csv")
	if err != nil {
		t.Fatalf("creating file part: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("writing file part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing writer: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/graphql", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	graphHandler.ServeHTTP(recorder, request)
	return recorder
}

// graphImportJob is one import node of a graph response.
type graphImportJob struct {
	ID            string
	UserID        string
	Filename      string
	State         string
	RowCount      int
	ImportedCount int
	SkippedCount  int
	FailedCount   int
	Columns       []string
	Mapping       []struct {
		Column int
		Field  string
	}
	Rows []struct {
		Position int
		Cells    []string
		Outcome  string
	}
	Contacts []struct {
		ContactID string
		Name      string
		RowID     string
	}
}

// stagedUpload uploads content and returns the staged import.
func stagedUpload(t *testing.T, graphHandler http.Handler, content string) graphImportJob {
	t.Helper()
	response := decodeGraph(t, graphUpload(t, graphHandler, []byte(content)))
	if len(response.Errors) != 0 {
		t.Fatalf("upload errors = %v, want none", response.Errors)
	}
	var payload struct {
		ImportUpload graphImportJob
	}
	if err := json.Unmarshal(response.Data, &payload); err != nil {
		t.Fatalf("decoding upload payload: %v", err)
	}
	return payload.ImportUpload
}

func TestGraphImportUploadStagesTheFile(t *testing.T) {
	t.Parallel()

	p, pool := newUploadPlugin(t)
	uploader := uuid.Must(uuid.NewV7())
	graphHandler := newGraphHandler(t, p, pool, uploader)

	staged := stagedUpload(t, graphHandler, twoRowCSV)

	if staged.Filename != "contacts.csv" || staged.State != "ready" || staged.RowCount != 2 {
		t.Errorf("import = %+v, want contacts.csv staged ready with 2 rows", staged)
	}
	if len(staged.Columns) != 2 || staged.Columns[0] != "Name" || staged.Columns[1] != "Email" {
		t.Errorf("columns = %v, want the CSV header", staged.Columns)
	}
	if len(staged.Mapping) != 0 {
		t.Errorf("mapping = %v, want none before assignments", staged.Mapping)
	}

	listed := decodeGraph(t, postGraph(t, graphHandler,
		`query($id: UUID!) { imports { id } importJob(id: $id) { id userId } }`,
		map[string]any{"id": staged.ID}))
	if len(listed.Errors) != 0 {
		t.Fatalf("read back errors = %v, want none", listed.Errors)
	}
	var payload struct {
		Imports   []struct{ ID string }
		ImportJob graphImportJob
	}
	if err := json.Unmarshal(listed.Data, &payload); err != nil {
		t.Fatalf("decoding read back: %v", err)
	}
	if len(payload.Imports) != 1 || payload.Imports[0].ID != staged.ID {
		t.Errorf("imports = %+v, want the staged import listed", payload.Imports)
	}
	if payload.ImportJob.UserID != uploader.String() {
		t.Errorf("userId = %q, want the uploader %s", payload.ImportJob.UserID, uploader)
	}
}

func TestGraphImportMappingAndRowsRoundTrip(t *testing.T) {
	t.Parallel()

	p, pool := newUploadPlugin(t)
	graphHandler := newGraphHandler(t, p, pool, uuid.Must(uuid.NewV7()))
	staged := stagedUpload(t, graphHandler, twoRowCSV)

	mapped := decodeGraph(t, postGraph(t, graphHandler,
		`mutation($id: UUID!) { importSetMapping(id: $id, assignments: [
			{column: 1, field: "email"}, {column: 0, field: "name"}
		]) { mapping { column field } } }`,
		map[string]any{"id": staged.ID}))
	if len(mapped.Errors) != 0 {
		t.Fatalf("mapping errors = %v, want none", mapped.Errors)
	}
	var mappedPayload struct {
		ImportSetMapping graphImportJob
	}
	if err := json.Unmarshal(mapped.Data, &mappedPayload); err != nil {
		t.Fatalf("decoding mapping payload: %v", err)
	}
	assignments := mappedPayload.ImportSetMapping.Mapping
	if len(assignments) != 2 || assignments[0].Column != 0 || assignments[0].Field != "name" ||
		assignments[1].Column != 1 || assignments[1].Field != "email" {
		t.Errorf("mapping = %+v, want name and email in column order", assignments)
	}

	rows := decodeGraph(t, postGraph(t, graphHandler,
		`query($id: UUID!) { importJob(id: $id) { rows { position cells outcome } } }`,
		map[string]any{"id": staged.ID}))
	var rowsPayload struct {
		ImportJob graphImportJob
	}
	if err := json.Unmarshal(rows.Data, &rowsPayload); err != nil {
		t.Fatalf("decoding rows payload: %v", err)
	}
	if len(rowsPayload.ImportJob.Rows) != 2 {
		t.Fatalf("rows = %d, want both staged rows", len(rowsPayload.ImportJob.Rows))
	}
	first := rowsPayload.ImportJob.Rows[0]
	if first.Position != 1 || first.Outcome != "pending" || first.Cells[0] != "Maria Perez" {
		t.Errorf("first row = %+v, want Maria Perez pending at position 1", first)
	}

	limited := decodeGraph(t, postGraph(t, graphHandler,
		`query($id: UUID!) { importJob(id: $id) { rows(limit: 1) { position } } }`,
		map[string]any{"id": staged.ID}))
	var limitedPayload struct {
		ImportJob graphImportJob
	}
	if err := json.Unmarshal(limited.Data, &limitedPayload); err != nil {
		t.Fatalf("decoding limited payload: %v", err)
	}
	if len(limitedPayload.ImportJob.Rows) != 1 {
		t.Errorf("rows(limit: 1) = %d rows, want 1", len(limitedPayload.ImportJob.Rows))
	}

	invalid := decodeGraph(t, postGraph(t, graphHandler,
		`query($id: UUID!) { importJob(id: $id) { rows(limit: 0) { position } } }`,
		map[string]any{"id": staged.ID}))
	if got := firstCode(t, invalid); got != "VALIDATION" {
		t.Errorf("rows(limit: 0) code = %q, want VALIDATION", got)
	}
}

func TestGraphImportCommitCreatesContacts(t *testing.T) {
	t.Parallel()

	p, pool, _, events := newCommittingPlugin(t)
	graphHandler := newGraphHandler(t, p, pool, uuid.Must(uuid.NewV7()))
	staged := stagedUpload(t, graphHandler, twoRowCSV)
	mustGraphMapping(t, graphHandler, staged.ID)

	committed := decodeGraph(t, postGraph(t, graphHandler,
		`mutation($id: UUID!) { importCommit(id: $id) { id imported skipped failed } }`,
		map[string]any{"id": staged.ID}))
	if len(committed.Errors) != 0 {
		t.Fatalf("commit errors = %v, want none", committed.Errors)
	}
	var commitPayload struct {
		ImportCommit struct {
			ID       string
			Imported int
			Skipped  int
			Failed   int
		}
	}
	if err := json.Unmarshal(committed.Data, &commitPayload); err != nil {
		t.Fatalf("decoding commit payload: %v", err)
	}
	counts := commitPayload.ImportCommit
	if counts.Imported != 2 || counts.Skipped != 0 || counts.Failed != 0 {
		t.Errorf("counts = %+v, want both rows imported", counts)
	}

	read := decodeGraph(t, postGraph(t, graphHandler,
		`query($id: UUID!) { importJob(id: $id) {
			state importedCount contacts { contactId name rowId }
		} }`,
		map[string]any{"id": staged.ID}))
	var readPayload struct {
		ImportJob graphImportJob
	}
	if err := json.Unmarshal(read.Data, &readPayload); err != nil {
		t.Fatalf("decoding read payload: %v", err)
	}
	job := readPayload.ImportJob
	if job.State != "committed" || job.ImportedCount != 2 || len(job.Contacts) != 2 {
		t.Fatalf("job = %+v, want committed with 2 linked contacts", job)
	}
	if job.Contacts[0].Name != "Maria Perez" {
		t.Errorf("first contact = %+v, want Maria Perez in row order", job.Contacts[0])
	}

	if len(events.names) != 1 || events.names[0] != "import.completed" {
		t.Errorf("events = %v, want import.completed announced once", events.names)
	}
}

// mustGraphMapping assigns name and email through the graph.
func mustGraphMapping(t *testing.T, graphHandler http.Handler, id string) {
	t.Helper()
	response := decodeGraph(t, postGraph(t, graphHandler,
		`mutation($id: UUID!) { importSetMapping(id: $id, assignments: [
			{column: 0, field: "name"}, {column: 1, field: "email"}
		]) { id } }`,
		map[string]any{"id": id}))
	if len(response.Errors) != 0 {
		t.Fatalf("mapping errors = %v, want none", response.Errors)
	}
}

func TestGraphImportUploadCapsSurfaceAsValidation(t *testing.T) {
	t.Parallel()

	p, pool := newUploadPlugin(t)
	graphHandler := newGraphHandler(t, p, pool, uuid.Must(uuid.NewV7()))

	tooManyRows := "Name\n" + strings.Repeat("Maria Perez\n", 5001)
	oversized := bytes.Repeat([]byte("a"), 5<<20+1)
	binaryJunk := []byte{0x00, 0x01, 0xFF, 0x00}

	rejections := map[string][]byte{
		"too many rows":      []byte(tooManyRows),
		"oversized file":     oversized,
		"unsupported format": binaryJunk,
	}
	for name, content := range rejections {
		response := decodeGraph(t, graphUpload(t, graphHandler, content))
		if got := firstCode(t, response); got != "VALIDATION" {
			t.Errorf("%s code = %q, want VALIDATION", name, got)
		}
	}
}

func TestGraphImportUploadAcceptsATwoMiBFile(t *testing.T) {
	t.Parallel()

	p, pool := newUploadPlugin(t)
	graphHandler := newGraphHandler(t, p, pool, uuid.Must(uuid.NewV7()))
	wideCell := strings.Repeat("x", 550)
	var content strings.Builder
	content.WriteString("Name,Note\n")
	for i := 0; i < 4000; i++ {
		fmt.Fprintf(&content, "Maria Perez,%s\n", wideCell)
	}

	staged := stagedUpload(t, graphHandler, content.String())

	if staged.State != "ready" || staged.RowCount != 4000 {
		t.Errorf("import = %+v, want the 2 MiB file staged ready", staged)
	}
}

func TestGraphImportUploadRequiresAUser(t *testing.T) {
	t.Parallel()

	p, pool := newUploadPlugin(t)
	graphHandler := newGraphHandler(t, p, pool, uuid.Nil)

	response := decodeGraph(t, graphUpload(t, graphHandler, []byte(twoRowCSV)))

	if got := firstCode(t, response); got != "UNAUTHENTICATED" {
		t.Errorf("code = %q, want UNAUTHENTICATED", got)
	}
}

func TestGraphImportErrorsAreClassified(t *testing.T) {
	t.Parallel()

	p, pool, _, _ := newCommittingPlugin(t)
	graphHandler := newGraphHandler(t, p, pool, uuid.Must(uuid.NewV7()))
	staged := stagedUpload(t, graphHandler, twoRowCSV)
	unknownImportID := uuid.Must(uuid.NewV7()).String()

	t.Run("unknown import", func(t *testing.T) {
		response := decodeGraph(t, postGraph(t, graphHandler,
			`query($id: UUID!) { importJob(id: $id) { id } }`,
			map[string]any{"id": unknownImportID}))
		if got := firstCode(t, response); got != "NOT_FOUND" {
			t.Errorf("code = %q, want NOT_FOUND", got)
		}
	})

	t.Run("unknown mapping field", func(t *testing.T) {
		response := decodeGraph(t, postGraph(t, graphHandler,
			`mutation($id: UUID!) { importSetMapping(id: $id, assignments: [
				{column: 0, field: "birthday"}
			]) { id } }`,
			map[string]any{"id": staged.ID}))
		if got := firstCode(t, response); got != "VALIDATION" {
			t.Errorf("code = %q, want VALIDATION", got)
		}
	})

	t.Run("commit without a mapping", func(t *testing.T) {
		unmapped := stagedUpload(t, graphHandler, twoRowCSV)
		response := decodeGraph(t, postGraph(t, graphHandler,
			`mutation($id: UUID!) { importCommit(id: $id) { id } }`,
			map[string]any{"id": unmapped.ID}))
		if got := firstCode(t, response); got != "VALIDATION" {
			t.Errorf("code = %q, want VALIDATION", got)
		}
	})

	t.Run("second commit and late mapping", func(t *testing.T) {
		mustGraphMapping(t, graphHandler, staged.ID)
		first := decodeGraph(t, postGraph(t, graphHandler,
			`mutation($id: UUID!) { importCommit(id: $id) { imported } }`,
			map[string]any{"id": staged.ID}))
		if len(first.Errors) != 0 {
			t.Fatalf("first commit errors = %v, want none", first.Errors)
		}
		second := decodeGraph(t, postGraph(t, graphHandler,
			`mutation($id: UUID!) { importCommit(id: $id) { imported } }`,
			map[string]any{"id": staged.ID}))
		if got := firstCode(t, second); got != "CONFLICT" {
			t.Errorf("second commit code = %q, want CONFLICT", got)
		}
		late := decodeGraph(t, postGraph(t, graphHandler,
			`mutation($id: UUID!) { importSetMapping(id: $id, assignments: [
				{column: 0, field: "name"}
			]) { id } }`,
			map[string]any{"id": staged.ID}))
		if got := firstCode(t, late); got != "CONFLICT" {
			t.Errorf("late mapping code = %q, want CONFLICT", got)
		}
	})
}

func TestGraphImportFieldsListsTheMappableFields(t *testing.T) {
	t.Parallel()

	p, pool, _, _ := newCommittingPlugin(t)
	graphHandler := newGraphHandler(t, p, pool, uuid.Must(uuid.NewV7()))

	response := decodeGraph(t, postGraph(t, graphHandler,
		`{ importFields { name label required } }`, nil))

	if len(response.Errors) != 0 {
		t.Fatalf("errors = %v, want none", response.Errors)
	}
	var payload struct {
		ImportFields []struct {
			Name     string
			Label    string
			Required bool
		}
	}
	if err := json.Unmarshal(response.Data, &payload); err != nil {
		t.Fatalf("decoding fields payload: %v", err)
	}
	if len(payload.ImportFields) == 0 || payload.ImportFields[0].Name != "name" {
		t.Errorf("importFields = %+v, want the name field first", payload.ImportFields)
	}
}

func TestGraphImportMutationsRejectAnUnknownImport(t *testing.T) {
	t.Parallel()

	p, pool, _, _ := newCommittingPlugin(t)
	graphHandler := newGraphHandler(t, p, pool, uuid.Must(uuid.NewV7()))
	unknownImportID := uuid.Must(uuid.NewV7()).String()

	mapping := decodeGraph(t, postGraph(t, graphHandler,
		`mutation($id: UUID!) { importSetMapping(id: $id, assignments: [{column: 0, field: "name"}]) { id } }`,
		map[string]any{"id": unknownImportID}))
	if got := firstCode(t, mapping); got != "NOT_FOUND" {
		t.Errorf("mapping code = %q, want NOT_FOUND", got)
	}

	commit := decodeGraph(t, postGraph(t, graphHandler,
		`mutation($id: UUID!) { importCommit(id: $id) { id } }`,
		map[string]any{"id": unknownImportID}))
	if got := firstCode(t, commit); got != "NOT_FOUND" {
		t.Errorf("commit code = %q, want NOT_FOUND", got)
	}
}

// brokenReader fails every read with a stream error.
type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("stream interrupted") }

func (brokenReader) Seek(int64, int) (int64, error) { return 0, nil }

func TestGraphImportUploadReadFailuresPropagate(t *testing.T) {
	t.Parallel()

	p, _, _, _ := newCommittingPlugin(t)
	uploaderCtx := sdk.WithUser(t.Context(), uuid.Must(uuid.NewV7()))

	_, err := p.MutationResolvers().ImportUpload(uploaderCtx,
		graphql.Upload{File: brokenReader{}, Filename: "contacts.csv"})

	if err == nil || !strings.Contains(err.Error(), "read upload") {
		t.Errorf("ImportUpload(broken reader) error = %v, want the read upload failure", err)
	}
}

// blockImportsUpdates installs a trigger rejecting imports updates, wholly or per state.
func blockImportsUpdates(t *testing.T, pool *pgxpool.Pool, condition string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`CREATE OR REPLACE FUNCTION plugin_importer.block_writes() RETURNS trigger
		LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'imports table blocked'; END $$`,
	); err != nil {
		t.Fatalf("creating the blocking function: %v", err)
	}
	if _, err := pool.Exec(t.Context(),
		`CREATE TRIGGER block_imports_updates BEFORE UPDATE ON plugin_importer.imports
		FOR EACH ROW `+condition+` EXECUTE FUNCTION plugin_importer.block_writes()`,
	); err != nil {
		t.Fatalf("creating the blocking trigger: %v", err)
	}
}

// unblockImportsUpdates removes the blocking trigger.
func unblockImportsUpdates(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`DROP TRIGGER block_imports_updates ON plugin_importer.imports`); err != nil {
		t.Fatalf("dropping the blocking trigger: %v", err)
	}
}

func TestGraphImportStoreFailuresSurface(t *testing.T) {
	t.Parallel()

	p, pool, contacts, _ := newCommittingPlugin(t)
	graphHandler := newGraphHandler(t, p, pool, uuid.Must(uuid.NewV7()))
	staged := stagedUpload(t, graphHandler, twoRowCSV)
	commitDocument := `mutation($id: UUID!) { importCommit(id: $id) { id } }`

	blockImportsUpdates(t, pool, "")
	mapping := decodeGraph(t, postGraph(t, graphHandler,
		`mutation($id: UUID!) { importSetMapping(id: $id, assignments: [
			{column: 0, field: "name"}, {column: 1, field: "email"}
		]) { id } }`,
		map[string]any{"id": staged.ID}))
	if got := firstCode(t, mapping); got != "INTERNAL" {
		t.Errorf("blocked mapping code = %q, want INTERNAL", got)
	}
	claim := decodeGraph(t, postGraph(t, graphHandler, commitDocument, map[string]any{"id": staged.ID}))
	if got := firstCode(t, claim); got != "INTERNAL" {
		t.Errorf("blocked claim code = %q, want INTERNAL", got)
	}
	unblockImportsUpdates(t, pool)

	mustGraphMapping(t, graphHandler, staged.ID)

	contacts.createErr = errors.New("directory down")
	settle := decodeGraph(t, postGraph(t, graphHandler, commitDocument, map[string]any{"id": staged.ID}))
	if got := firstCode(t, settle); got != "INTERNAL" {
		t.Errorf("failed settle code = %q, want INTERNAL", got)
	}
	contacts.createErr = nil

	blockImportsUpdates(t, pool, "WHEN (NEW.state = 'committed')")
	finish := decodeGraph(t, postGraph(t, graphHandler, commitDocument, map[string]any{"id": staged.ID}))
	if got := firstCode(t, finish); got != "INTERNAL" {
		t.Errorf("blocked finish code = %q, want INTERNAL", got)
	}
	unblockImportsUpdates(t, pool)

	if _, err := pool.Exec(t.Context(), `DROP SCHEMA plugin_importer CASCADE`); err != nil {
		t.Fatalf("dropping the importer schema: %v", err)
	}
	listing := decodeGraph(t, postGraph(t, graphHandler, `{ imports { id } }`, nil))
	if got := firstCode(t, listing); got != "INTERNAL" {
		t.Errorf("imports listing code = %q, want INTERNAL", got)
	}
	lookup := decodeGraph(t, postGraph(t, graphHandler,
		`query($id: UUID!) { importJob(id: $id) { id } }`, map[string]any{"id": staged.ID}))
	if got := firstCode(t, lookup); got != "INTERNAL" {
		t.Errorf("import lookup code = %q, want INTERNAL", got)
	}
	upload := decodeGraph(t, graphUpload(t, graphHandler, []byte(twoRowCSV)))
	if got := firstCode(t, upload); got != "INTERNAL" {
		t.Errorf("upload code = %q, want INTERNAL", got)
	}
	stagedJob := &model.ImportJob{ID: uuid.MustParse(staged.ID)}
	if _, err := p.ImportJobResolvers().Rows(t.Context(), stagedJob, nil); err == nil {
		t.Error("Rows() on a dropped schema error = nil, want error")
	}
	if _, err := p.ImportJobResolvers().Contacts(t.Context(), stagedJob); err == nil {
		t.Error("Contacts() on a dropped schema error = nil, want error")
	}
}
