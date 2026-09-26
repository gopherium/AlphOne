// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"database/sql"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"

	"github.com/gopherium/alphone/sdk"
)

// entryIDsVersion is the migration that gives every stored repeater entry an id.
const entryIDsVersion = 6

// fieldsProvider returns a goose provider over the plugin's migrations and its own version table.
func fieldsProvider(t *testing.T, db *sql.DB) *goose.Provider {
	t.Helper()
	store, err := database.NewStore(database.DialectPostgres, "plugin_fields.goose_db_version")
	if err != nil {
		t.Fatalf("building the version store: %v", err)
	}
	provider, err := goose.NewProvider("", db, migrationSource, goose.WithStore(store))
	if err != nil {
		t.Fatalf("building the provider: %v", err)
	}
	return provider
}

// beforeEntryIDs returns a plugin and its database rolled back to the schema before entry ids.
func beforeEntryIDs(t *testing.T) (*Plugin, *sql.DB, *goose.Provider) {
	t.Helper()
	p := newMigratedPlugin(t)
	db := stdlib.OpenDBFromPool(p.pool)
	t.Cleanup(func() { _ = db.Close() })
	provider := fieldsProvider(t, db)
	if _, err := provider.DownTo(t.Context(), entryIDsVersion-1); err != nil {
		t.Fatalf("rolling back to the schema before entry ids: %v", err)
	}
	return p, db, provider
}

// storedDefinition stores one definition row in a tenant as the schema before entry ids holds it.
func storedDefinition(t *testing.T, db *sql.DB, tenant uuid.UUID, name, kind, subFields string, archived bool) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(),
		`INSERT INTO plugin_fields.definitions (id, name, label, kind, sub_fields, archived_at, created_at, tenant_id)
		VALUES ($1, $2, $2, $3, $4::jsonb, CASE WHEN $5 THEN now() END, now(), $6)`,
		uuid.Must(uuid.NewV7()), name, kind, subFields, archived, tenant); err != nil {
		t.Fatalf("storing the definition %s: %v", name, err)
	}
}

// contactHolding stores one contact in a tenant holding the given values.
func contactHolding(t *testing.T, db *sql.DB, tenant uuid.UUID, values string) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := db.ExecContext(t.Context(),
		"INSERT INTO core.contacts (id, name, created_at, tenant_id) VALUES ($1, 'Maria Perez', now(), $2)",
		id, tenant); err != nil {
		t.Fatalf("storing the contact: %v", err)
	}
	if _, err := db.ExecContext(t.Context(),
		"INSERT INTO plugin_fields.contact_values (contact_id, values, tenant_id) VALUES ($1, $2::jsonb, $3)",
		id, values, tenant); err != nil {
		t.Fatalf("storing the values: %v", err)
	}
	return id
}

// heldValues reads the values one contact holds.
func heldValues(t *testing.T, db *sql.DB, contactID uuid.UUID) map[string]any {
	t.Helper()
	var raw []byte
	if err := db.QueryRowContext(t.Context(),
		"SELECT values FROM plugin_fields.contact_values WHERE contact_id = $1", contactID).Scan(&raw); err != nil {
		t.Fatalf("reading the values: %v", err)
	}
	var values map[string]any
	if err := json.Unmarshal(raw, &values); err != nil {
		t.Fatalf("decoding %s: %v", raw, err)
	}
	return values
}

// decoded turns a JSON literal into the value the database answers for it.
func decoded(t *testing.T, literal string) map[string]any {
	t.Helper()
	var values map[string]any
	if err := json.Unmarshal([]byte(literal), &values); err != nil {
		t.Fatalf("decoding %s: %v", literal, err)
	}
	return values
}

// splitIDs returns the ids of a stored list and its entries with their ids dropped.
func splitIDs(t *testing.T, list any) ([]string, []any) {
	t.Helper()
	entries, isList := list.([]any)
	if !isList {
		t.Fatalf("stored list = %#v, want a list", list)
	}
	ids := make([]string, 0, len(entries))
	cells := make([]any, 0, len(entries))
	for _, entry := range entries {
		held, isObject := entry.(map[string]any)
		if !isObject {
			t.Fatalf("entry = %#v, want an object", entry)
		}
		id, _ := held[entryIDKey].(string)
		if _, err := uuid.Parse(id); err != nil {
			t.Errorf("entry %#v holds no id, want one minted", held)
		}
		ids = append(ids, id)
		rest := map[string]any{}
		for key, value := range held {
			if key != entryIDKey {
				rest[key] = value
			}
		}
		cells = append(cells, rest)
	}
	return ids, cells
}

const (
	commentColumns = `[{"name": "comment", "label": "Comment", "kind": "LONGTEXT"}]`
	storedHistory  = `{"history": [{"comment": "First call"}, {"comment": "Sent the offer"}],
		"calls": [{"comment": "Rang back"}], "nickname": "Mari"}`
)

func TestEntryIDsMigrationGivesEveryEntryAnIDNewestFirst(t *testing.T) {
	t.Parallel()

	_, db, provider := beforeEntryIDs(t)
	home := sdk.TenantOrDefault(t.Context())
	storedDefinition(t, db, home, "history", "REPEATER", commentColumns, false)
	storedDefinition(t, db, home, "calls", "REPEATER", commentColumns, true)
	storedDefinition(t, db, home, "nickname", "TEXT", "[]", false)
	maria := contactHolding(t, db, home, storedHistory)

	if _, err := provider.UpTo(t.Context(), entryIDsVersion); err != nil {
		t.Fatalf("applying entry ids: %v", err)
	}

	held := heldValues(t, db, maria)
	ids, cells := splitIDs(t, held["history"])
	want := []any{map[string]any{"comment": "Sent the offer"}, map[string]any{"comment": "First call"}}
	if !reflect.DeepEqual(cells, want) {
		t.Errorf("history = %#v, want the stored entries newest first", cells)
	}
	if len(ids) != 2 || ids[0] == ids[1] {
		t.Errorf("ids = %v, want two distinct ids", ids)
	}
	if _, archived := splitIDs(t, held["calls"]); len(archived) != 1 {
		t.Errorf("calls = %#v, want the archived repeater's entry given an id too", held["calls"])
	}
	if held["nickname"] != "Mari" {
		t.Errorf("nickname = %#v, want the plain value untouched", held["nickname"])
	}
}

func TestEntryIDsMigrationLeavesEveryOtherValueAlone(t *testing.T) {
	t.Parallel()

	p, db, provider := beforeEntryIDs(t)
	home := sdk.TenantOrDefault(t.Context())
	acme := sdk.TenantOrDefault(inTenant(t, p))
	storedDefinition(t, db, home, "history", "REPEATER", commentColumns, false)
	storedDefinition(t, db, home, "nickname", "TEXT", "[]", false)
	storedDefinition(t, db, acme, "history", "TEXT", "[]", false)
	literals := []struct {
		tenant uuid.UUID
		values string
	}{
		{home, `{"nickname": "Rosa"}`},
		{home, `{"history": []}`},
		{acme, `{"history": "Called twice"}`},
		{home, `{"history": [{"id": "kept", "comment": "Kept"}]}`},
	}
	stored := make(map[uuid.UUID]string, len(literals))
	for _, held := range literals {
		stored[contactHolding(t, db, held.tenant, held.values)] = held.values
	}

	if _, err := provider.UpTo(t.Context(), entryIDsVersion); err != nil {
		t.Fatalf("applying entry ids: %v", err)
	}

	for contactID, literal := range stored {
		if held := heldValues(t, db, contactID); !reflect.DeepEqual(held, decoded(t, literal)) {
			t.Errorf("values = %#v, want %s left as stored", held, literal)
		}
	}
}

func TestEntryIDsMigrationRestoresTheStoredListsGoingDown(t *testing.T) {
	t.Parallel()

	_, db, provider := beforeEntryIDs(t)
	home := sdk.TenantOrDefault(t.Context())
	storedDefinition(t, db, home, "history", "REPEATER", commentColumns, false)
	storedDefinition(t, db, home, "calls", "REPEATER", commentColumns, true)
	storedDefinition(t, db, home, "nickname", "TEXT", "[]", false)
	maria := contactHolding(t, db, home, storedHistory)
	if _, err := provider.UpTo(t.Context(), entryIDsVersion); err != nil {
		t.Fatalf("applying entry ids: %v", err)
	}

	if _, err := provider.DownTo(t.Context(), entryIDsVersion-1); err != nil {
		t.Fatalf("rolling entry ids back: %v", err)
	}

	if held := heldValues(t, db, maria); !reflect.DeepEqual(held, decoded(t, storedHistory)) {
		t.Errorf("values = %#v, want the lists back oldest first with no ids", held)
	}
}

func TestEntryIDsMigrationStopsAtASubFieldNamedID(t *testing.T) {
	t.Parallel()

	_, db, provider := beforeEntryIDs(t)
	storedDefinition(t, db, sdk.TenantOrDefault(t.Context()), "history", "REPEATER",
		`[{"name": "id", "label": "ID", "kind": "TEXT"}]`, true)

	_, err := provider.UpTo(t.Context(), entryIDsVersion)

	if err == nil || !strings.Contains(err.Error(), "definitions_sub_fields_no_id") {
		t.Errorf("UpTo() error = %v, want the sub field named id to stop the migration", err)
	}
}
