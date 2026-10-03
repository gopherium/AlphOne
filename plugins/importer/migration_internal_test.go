// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"database/sql"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"

	"github.com/gopherium/alphone/sdk"
)

// reopenVersion is the migration that returns every import left committing without a mapping to ready.
const reopenVersion = 3

// reasonCodesVersion is the migration that stores every row reason as a code beside its values.
const reasonCodesVersion = 4

// importerProvider returns a goose provider over the plugin's migrations and its own version table.
func importerProvider(t *testing.T, db *sql.DB) *goose.Provider {
	t.Helper()
	store, err := database.NewStore(database.DialectPostgres, "plugin_importer.goose_db_version")
	if err != nil {
		t.Fatalf("building the version store: %v", err)
	}
	provider, err := goose.NewProvider("", db, migrationSource, goose.WithStore(store))
	if err != nil {
		t.Fatalf("building the provider: %v", err)
	}
	return provider
}

// beforeMigration returns a database rolled back to the schema before the given migration, beside its provider.
func beforeMigration(t *testing.T, version int64) (*sql.DB, *goose.Provider) {
	t.Helper()
	db := stdlib.OpenDBFromPool(newMigratedPool(t))
	t.Cleanup(func() { _ = db.Close() })
	provider := importerProvider(t, db)
	if _, err := provider.DownTo(t.Context(), version-1); err != nil {
		t.Fatalf("rolling back to the schema before migration %d: %v", version, err)
	}
	return db, provider
}

// beforeReopen returns a database rolled back to the schema before the reopen migration, beside its provider.
func beforeReopen(t *testing.T) (*sql.DB, *goose.Provider) {
	t.Helper()
	return beforeMigration(t, reopenVersion)
}

// storedImport stores one import in the given state holding the given mapping and returns its id.
func storedImport(t *testing.T, db *sql.DB, state, assignments string) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := db.ExecContext(t.Context(),
		`INSERT INTO plugin_importer.imports (id, user_id, filename, columns, mapping, state,
			row_count, imported_count, skipped_count, failed_count, created_at)
		VALUES ($1, $2, 'contacts.csv', ARRAY['Name'], $3::jsonb, $4, 1, 0, 0, 0, now())`,
		id, uuid.Must(uuid.NewV7()), assignments, state); err != nil {
		t.Fatalf("storing the %s import: %v", state, err)
	}
	return id
}

// heldState returns the state one import stands in.
func heldState(t *testing.T, db *sql.DB, id uuid.UUID) string {
	t.Helper()
	var state string
	if err := db.QueryRowContext(t.Context(),
		"SELECT state FROM plugin_importer.imports WHERE id = $1", id).Scan(&state); err != nil {
		t.Fatalf("reading the import state: %v", err)
	}
	return state
}

func TestReopenMigrationReturnsAnImportLeftCommittingWithoutAMappingToReady(t *testing.T) {
	t.Parallel()

	db, provider := beforeReopen(t)
	unmapped := storedImport(t, db, stateCommitting, "{}")
	mapped := storedImport(t, db, stateCommitting, `{"0":"name"}`)
	committed := storedImport(t, db, stateCommitted, `{"0":"name"}`)

	if _, err := provider.UpTo(t.Context(), reopenVersion); err != nil {
		t.Fatalf("applying the reopen migration: %v", err)
	}

	if held := heldState(t, db, unmapped); held != stateReady {
		t.Errorf("unmapped import state = %q, want %q", held, stateReady)
	}
	if held := heldState(t, db, mapped); held != stateCommitting {
		t.Errorf("mapped import state = %q, want %q", held, stateCommitting)
	}
	if held := heldState(t, db, committed); held != stateCommitted {
		t.Errorf("committed import state = %q, want %q", held, stateCommitted)
	}
}

// sentence returns a pointer to the reason text a row stores.
func sentence(text string) *string {
	return &text
}

// coded returns the stored reason object a code and its values decode to.
func coded(code string, meta map[string]any) map[string]any {
	if meta == nil {
		meta = map[string]any{}
	}
	return map[string]any{"code": code, "meta": meta}
}

// storedReasonRow stores one staged row of the import carrying the given reason text and returns its id.
func storedReasonRow(t *testing.T, db *sql.DB, importID uuid.UUID, position int, reason *string) uuid.UUID {
	t.Helper()
	return storedRow(t, db, importID, position, "$4", reason)
}

// storedCodedRow stores one staged row of the import carrying the given reason object and returns its id.
func storedCodedRow(t *testing.T, db *sql.DB, importID uuid.UUID, position int, reason string) uuid.UUID {
	t.Helper()
	return storedRow(t, db, importID, position, "$4::jsonb", reason)
}

// storedRow stores one staged row of the import whose reason the given expression reads and returns its id.
func storedRow(t *testing.T, db *sql.DB, importID uuid.UUID, position int, expression string, reason any) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := db.ExecContext(t.Context(),
		`INSERT INTO plugin_importer.import_rows (id, import_id, position, cells, outcome, reason)
		VALUES ($1, $2, $3, '[]'::jsonb, 'failed', `+expression+`)`,
		id, importID, position, reason); err != nil {
		t.Fatalf("storing the row at position %d: %v", position, err)
	}
	return id
}

// heldReasonObject returns the reason one staged row holds, decoded from its stored object.
func heldReasonObject(t *testing.T, db *sql.DB, id uuid.UUID) map[string]any {
	t.Helper()
	var raw []byte
	if err := db.QueryRowContext(t.Context(),
		"SELECT reason FROM plugin_importer.import_rows WHERE id = $1", id).Scan(&raw); err != nil {
		t.Fatalf("reading the row reason: %v", err)
	}
	if raw == nil {
		return nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decoding the row reason %s: %v", raw, err)
	}
	return decoded
}

// heldReasonText returns the reason text one staged row holds.
func heldReasonText(t *testing.T, db *sql.DB, id uuid.UUID) *string {
	t.Helper()
	var text *string
	if err := db.QueryRowContext(t.Context(),
		"SELECT reason FROM plugin_importer.import_rows WHERE id = $1", id).Scan(&text); err != nil {
		t.Fatalf("reading the row reason: %v", err)
	}
	return text
}

func TestReasonCodesMigrationReadsEveryStoredSentenceAsACode(t *testing.T) {
	t.Parallel()

	db, provider := beforeMigration(t, reasonCodesVersion)
	parent := storedImport(t, db, stateCommitted, `{"0":"name"}`)
	tests := []struct {
		name   string
		stored *string
		want   map[string]any
	}{
		{"no reason", nil, nil},
		{"an empty reason", sentence(""), nil},
		{"a row without a name or a detail", sentence("the row carries no name or no contact detail"),
			coded("row_incomplete", nil)},
		{"a detail AlphOne cannot use", sentence("the row holds a name or a contact detail AlphOne cannot use"),
			coded("contact_details_invalid", nil)},
		{"the demo sentence naming no owner", sentence("the contact detail already belongs to a contact"),
			coded("identity_taken", nil)},
		{"a named owner", sentence("the contact detail already belongs to Maria Perez"),
			coded("identity_taken_by", map[string]any{"ownerName": "Maria Perez"})},
		{"a short row", sentence("the row holds 1 cells, the header lists 3"),
			coded("row_cell_count_mismatch", map[string]any{"cells": float64(1), "columns": float64(3)})},
		{"a bare quote", sentence(`the row is malformed: parse error on line 2, column 6: bare " in non-quoted-field`),
			coded("row_quote_misplaced", map[string]any{"line": float64(2)})},
		{"a quote across lines", sentence(`the row is malformed: record on line 2; parse error on line 3, column 2: ` +
			`extraneous or missing " in quoted-field`),
			coded("row_quote_misplaced", map[string]any{"line": float64(3)})},
		{"a workbook error", sentence("the row is malformed: XML syntax error on line 4: unexpected EOF"),
			coded("row_malformed", nil)},
		{"a value of another kind", sentence("fields: the value does not match the kind its definition declares: " +
			"birthDate expects DATE"),
			coded("value_kind_mismatch", map[string]any{"field": "birthDate", "kind": "DATE"})},
		{"a value of another kind without its prefix", sentence("the value does not match the kind its definition " +
			"declares: shoeSize expects NUMBER"),
			coded("value_kind_mismatch", map[string]any{"field": "shoeSize", "kind": "NUMBER"})},
		{"fields no live field holds", sentence("no live field holds birthDate, shoeSize"),
			coded("field_unknown", map[string]any{"fields": []any{"birthDate", "shoeSize"}})},
		{"a name no definition holds", sentence("fields: no live definition holds that name: neverDefined"),
			coded("field_unknown", map[string]any{"fields": []any{"neverDefined"}})},
		{"names no definition holds without the prefix", sentence("no live definition holds that name: " +
			"alsoUndefined, neverDefined"),
			coded("field_unknown", map[string]any{"fields": []any{"alsoUndefined", "neverDefined"}})},
		{"an unknown sentence", sentence("the provider will not store this row"),
			coded("legacy_text", map[string]any{"text": "the provider will not store this row"})},
	}
	ids := make([]uuid.UUID, len(tests))
	for i, tc := range tests {
		ids[i] = storedReasonRow(t, db, parent, i+1, tc.stored)
	}

	if _, err := provider.UpTo(t.Context(), reasonCodesVersion); err != nil {
		t.Fatalf("applying the reason codes migration: %v", err)
	}

	for i, tc := range tests {
		if got := heldReasonObject(t, db, ids[i]); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: reason = %#v, want %#v", tc.name, got, tc.want)
		}
	}
}

func TestReasonCodesMigrationRollsBackToTheSentences(t *testing.T) {
	t.Parallel()

	db, provider := beforeMigration(t, reasonCodesVersion)
	parent := storedImport(t, db, stateCommitted, `{"0":"name"}`)
	lossless := []*string{
		nil,
		sentence("the row carries no name or no contact detail"),
		sentence("the row holds a name or a contact detail AlphOne cannot use"),
		sentence("the contact detail already belongs to a contact"),
		sentence("the contact detail already belongs to Maria Perez"),
		sentence("the row holds 4 cells, the header lists 3"),
		sentence("the row is malformed: parse error on line 2"),
		sentence("the row is malformed: unreadable"),
		sentence("fields: the value does not match the kind its definition declares: birthDate expects DATE"),
		sentence("no live field holds birthDate, shoeSize"),
		sentence("the provider will not store this row"),
	}
	ids := make([]uuid.UUID, len(lossless))
	for i, stored := range lossless {
		ids[i] = storedReasonRow(t, db, parent, i+1, stored)
	}
	if _, err := provider.UpTo(t.Context(), reasonCodesVersion); err != nil {
		t.Fatalf("applying the reason codes migration: %v", err)
	}
	refused := storedCodedRow(t, db, parent, len(lossless)+1, `{"code":"field_text_refused","meta":{}}`)
	unknown := storedCodedRow(t, db, parent, len(lossless)+2, `{"code":"row_from_the_future","meta":{}}`)

	if _, err := provider.DownTo(t.Context(), reasonCodesVersion-1); err != nil {
		t.Fatalf("rolling back the reason codes migration: %v", err)
	}

	for i, want := range lossless {
		if got := heldReasonText(t, db, ids[i]); !reflect.DeepEqual(got, want) {
			t.Errorf("row %d reason = %v, want %v", i+1, describe(got), describe(want))
		}
	}
	if got := heldReasonText(t, db, refused); got == nil || *got != "a field does not accept the value this row holds" {
		t.Errorf("refused row reason = %v, want the field text sentence", describe(got))
	}
	if got := heldReasonText(t, db, unknown); got == nil || *got != "row_from_the_future" {
		t.Errorf("unknown row reason = %v, want the code as stored", describe(got))
	}
}

func TestReasonCodesMigrationReadsTheSentencesItRollsBackToAsTheirCodes(t *testing.T) {
	t.Parallel()

	db, provider := beforeMigration(t, reasonCodesVersion)
	parent := storedImport(t, db, stateCommitted, `{"0":"name"}`)
	if _, err := provider.UpTo(t.Context(), reasonCodesVersion); err != nil {
		t.Fatalf("applying the reason codes migration: %v", err)
	}
	values := map[string]map[string]any{
		reasonCellCountMismatch:   {"cells": float64(4), "columns": float64(3)},
		reasonQuoteMisplaced:      {"line": float64(2)},
		reasonIdentityTakenBy:     {"ownerName": "Maria Perez"},
		sdk.FieldTextKindMismatch: {"field": "birthDate", "kind": "DATE"},
		sdk.FieldTextFieldUnknown: {"fields": []any{"birthDate", "shoeSize"}},
	}
	ids := make([]uuid.UUID, len(rowReasonCodes))
	for i, code := range rowReasonCodes {
		stored, err := json.Marshal(coded(code, values[code]))
		if err != nil {
			t.Fatalf("encoding the %s reason: %v", code, err)
		}
		ids[i] = storedCodedRow(t, db, parent, i+1, string(stored))
	}

	if _, err := provider.DownTo(t.Context(), reasonCodesVersion-1); err != nil {
		t.Fatalf("rolling back the reason codes migration: %v", err)
	}
	if _, err := provider.UpTo(t.Context(), reasonCodesVersion); err != nil {
		t.Fatalf("applying the reason codes migration again: %v", err)
	}

	for i, code := range rowReasonCodes {
		if got, want := heldReasonObject(t, db, ids[i]), coded(code, values[code]); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: reason = %#v, want %#v", code, got, want)
		}
	}
}

// describe returns the reason text a failure message shows, naming a NULL as such.
func describe(text *string) string {
	if text == nil {
		return "NULL"
	}
	return *text
}
