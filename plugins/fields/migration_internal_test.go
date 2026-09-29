// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"database/sql"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"

	"github.com/gopherium/alphone/sdk"
)

// entryIDsVersion is the migration that gives every stored repeater entry an id.
const entryIDsVersion = 6

// inheritedNamesVersion is the migration that moves every definition off a name every JavaScript object inherits.
const inheritedNamesVersion = 7

// fieldPositionsVersion is the migration that gives every definition a stored place in the order.
const fieldPositionsVersion = 8

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

// rolledBackBefore returns a plugin and its database rolled back to the schema before the given migration.
func rolledBackBefore(t *testing.T, version int64) (*Plugin, *sql.DB, *goose.Provider) {
	t.Helper()
	p := newMigratedPlugin(t)
	db := stdlib.OpenDBFromPool(p.pool)
	t.Cleanup(func() { _ = db.Close() })
	provider := fieldsProvider(t, db)
	if _, err := provider.DownTo(t.Context(), version-1); err != nil {
		t.Fatalf("rolling back to the schema before migration %d: %v", version, err)
	}
	return p, db, provider
}

// beforeEntryIDs returns a plugin and its database rolled back to the schema before entry ids.
func beforeEntryIDs(t *testing.T) (*Plugin, *sql.DB, *goose.Provider) {
	t.Helper()
	return rolledBackBefore(t, entryIDsVersion)
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

// beforeInheritedNames returns a plugin and its database rolled back to the schema before inherited names move.
func beforeInheritedNames(t *testing.T) (*Plugin, *sql.DB, *goose.Provider) {
	t.Helper()
	return rolledBackBefore(t, inheritedNamesVersion)
}

// definitionsIn returns whether each definition a tenant holds is archived, by name.
func definitionsIn(t *testing.T, db *sql.DB, tenant uuid.UUID) map[string]bool {
	t.Helper()
	rows, err := db.QueryContext(t.Context(),
		"SELECT name, archived_at IS NOT NULL FROM plugin_fields.definitions WHERE tenant_id = $1", tenant)
	if err != nil {
		t.Fatalf("reading the definitions: %v", err)
	}
	defer func() { _ = rows.Close() }()
	held := map[string]bool{}
	for rows.Next() {
		var name string
		var archived bool
		if err := rows.Scan(&name, &archived); err != nil {
			t.Fatalf("scanning a definition: %v", err)
		}
		held[name] = archived
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the definitions: %v", err)
	}
	return held
}

// moveInheritedNames applies the migration that moves definitions off inherited names.
func moveInheritedNames(t *testing.T, provider *goose.Provider) {
	t.Helper()
	if _, err := provider.UpTo(t.Context(), inheritedNamesVersion); err != nil {
		t.Fatalf("moving inherited names: %v", err)
	}
}

func TestInheritedNamesMigrationMovesAFieldAndItsValues(t *testing.T) {
	t.Parallel()

	_, db, provider := beforeInheritedNames(t)
	home := sdk.TenantOrDefault(t.Context())
	storedDefinition(t, db, home, "constructor", "NUMBER", "[]", false)
	storedDefinition(t, db, home, "nickname", "TEXT", "[]", false)
	maria := contactHolding(t, db, home, `{"constructor": 5, "nickname": "Mari"}`)

	moveInheritedNames(t, provider)

	want := map[string]bool{"constructor2": false, "nickname": false}
	if held := definitionsIn(t, db, home); !reflect.DeepEqual(held, want) {
		t.Errorf("definitions = %v, want constructor moved to constructor2", held)
	}
	if held := heldValues(t, db, maria); !reflect.DeepEqual(held, decoded(t, `{"constructor2": 5, "nickname": "Mari"}`)) {
		t.Errorf("values = %#v, want the value moved with its field", held)
	}
}

func TestInheritedNamesMigrationStepsPastANameTheWorkspaceHolds(t *testing.T) {
	t.Parallel()

	_, db, provider := beforeInheritedNames(t)
	home := sdk.TenantOrDefault(t.Context())
	storedDefinition(t, db, home, "constructor", "NUMBER", "[]", false)
	storedDefinition(t, db, home, "constructor2", "TEXT", "[]", false)
	maria := contactHolding(t, db, home, `{"constructor": 5, "constructor2": "Kept"}`)

	moveInheritedNames(t, provider)

	want := map[string]bool{"constructor2": false, "constructor3": false}
	if held := definitionsIn(t, db, home); !reflect.DeepEqual(held, want) {
		t.Errorf("definitions = %v, want constructor moved past the held constructor2", held)
	}
	moved := decoded(t, `{"constructor2": "Kept", "constructor3": 5}`)
	if held := heldValues(t, db, maria); !reflect.DeepEqual(held, moved) {
		t.Errorf("values = %#v, want the moved value under constructor3 and constructor2 kept", held)
	}
}

func TestInheritedNamesMigrationNumbersEachWorkspaceOnItsOwn(t *testing.T) {
	t.Parallel()

	p, db, provider := beforeInheritedNames(t)
	home := sdk.TenantOrDefault(t.Context())
	acme := sdk.TenantOrDefault(inTenant(t, p))
	storedDefinition(t, db, home, "valueOf", "NUMBER", "[]", false)
	storedDefinition(t, db, home, "valueOf2", "NUMBER", "[]", false)
	storedDefinition(t, db, acme, "valueOf", "NUMBER", "[]", false)

	moveInheritedNames(t, provider)

	want := map[string]bool{"valueOf2": false, "valueOf3": false}
	if held := definitionsIn(t, db, home); !reflect.DeepEqual(held, want) {
		t.Errorf("home definitions = %v, want valueOf moved to valueOf3", held)
	}
	if held := definitionsIn(t, db, acme); !reflect.DeepEqual(held, map[string]bool{"valueOf2": false}) {
		t.Errorf("acme definitions = %v, want valueOf moved to valueOf2", held)
	}
}

func TestInheritedNamesMigrationMovesArchivedFieldsToo(t *testing.T) {
	t.Parallel()

	_, db, provider := beforeInheritedNames(t)
	home := sdk.TenantOrDefault(t.Context())
	storedDefinition(t, db, home, "toString", "TEXT", "[]", true)
	storedDefinition(t, db, home, "hasOwnProperty", "BOOLEAN", "[]", false)
	maria := contactHolding(t, db, home, `{"toString": "a", "hasOwnProperty": true}`)

	moveInheritedNames(t, provider)

	want := map[string]bool{"toString2": true, "hasOwnProperty2": false}
	if held := definitionsIn(t, db, home); !reflect.DeepEqual(held, want) {
		t.Errorf("definitions = %v, want both moved, the archived one still archived", held)
	}
	moved := decoded(t, `{"toString2": "a", "hasOwnProperty2": true}`)
	if held := heldValues(t, db, maria); !reflect.DeepEqual(held, moved) {
		t.Errorf("values = %#v, want both values moved with their fields", held)
	}
}

// valuesVersion returns the row version of one contact's values row.
func valuesVersion(t *testing.T, db *sql.DB, contactID uuid.UUID) string {
	t.Helper()
	var version string
	if err := db.QueryRowContext(t.Context(),
		"SELECT xmin::text FROM plugin_fields.contact_values WHERE contact_id = $1", contactID).Scan(&version); err != nil {
		t.Fatalf("reading the row version: %v", err)
	}
	return version
}

func TestInheritedNamesMigrationLeavesRowsWithoutTheNamesUnwritten(t *testing.T) {
	t.Parallel()

	_, db, provider := beforeInheritedNames(t)
	home := sdk.TenantOrDefault(t.Context())
	storedDefinition(t, db, home, "valueOf", "NUMBER", "[]", false)
	rosa := contactHolding(t, db, home, `{"nickname": "Rosa"}`)
	before := valuesVersion(t, db, rosa)

	moveInheritedNames(t, provider)

	if after := valuesVersion(t, db, rosa); after != before {
		t.Errorf("row version = %s, want %s, the row holds no moved name", after, before)
	}
}

func TestInheritedNamesMigrationClearsStrayValuesUnderTheNewName(t *testing.T) {
	t.Parallel()

	_, db, provider := beforeInheritedNames(t)
	home := sdk.TenantOrDefault(t.Context())
	storedDefinition(t, db, home, "constructor", "NUMBER", "[]", false)
	stray := contactHolding(t, db, home, `{"constructor2": "Left behind", "nickname": "Rosa"}`)

	moveInheritedNames(t, provider)

	if held := heldValues(t, db, stray); !reflect.DeepEqual(held, decoded(t, `{"nickname": "Rosa"}`)) {
		t.Errorf("values = %#v, want the stray constructor2 value cleared", held)
	}
}

func TestInheritedNamesMigrationLeavesOtherFieldsAlone(t *testing.T) {
	t.Parallel()

	p, db, provider := beforeInheritedNames(t)
	home := sdk.TenantOrDefault(t.Context())
	acme := sdk.TenantOrDefault(inTenant(t, p))
	storedDefinition(t, db, home, "nickname", "TEXT", "[]", false)
	storedDefinition(t, db, acme, "constructor", "NUMBER", "[]", false)
	literal := `{"nickname": "Rosa", "constructor2": "Kept"}`
	rosa := contactHolding(t, db, home, literal)

	moveInheritedNames(t, provider)

	if held := definitionsIn(t, db, home); !reflect.DeepEqual(held, map[string]bool{"nickname": false}) {
		t.Errorf("definitions = %v, want the home workspace untouched", held)
	}
	if held := heldValues(t, db, rosa); !reflect.DeepEqual(held, decoded(t, literal)) {
		t.Errorf("values = %#v, want the home values left as stored", held)
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

// definedAt stores one live text definition in a tenant as created at the given time.
func definedAt(t *testing.T, db *sql.DB, tenant uuid.UUID, name string, createdAt time.Time) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(),
		`INSERT INTO plugin_fields.definitions (id, name, label, kind, created_at, tenant_id)
		VALUES ($1, $2, $2, 'TEXT', $3, $4)`, uuid.Must(uuid.NewV7()), name, createdAt, tenant); err != nil {
		t.Fatalf("storing the definition %s: %v", name, err)
	}
}

// namesByPosition returns the names of a tenant's definitions in the order their stored positions give.
func namesByPosition(t *testing.T, db *sql.DB, tenant uuid.UUID) []string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(),
		"SELECT name FROM plugin_fields.definitions WHERE tenant_id = $1 ORDER BY position", tenant)
	if err != nil {
		t.Fatalf("reading the positions: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scanning a name: %v", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the positions: %v", err)
	}
	return names
}

// placeFields applies the migration that gives every definition a stored position.
func placeFields(t *testing.T, provider *goose.Provider) {
	t.Helper()
	if _, err := provider.UpTo(t.Context(), fieldPositionsVersion); err != nil {
		t.Fatalf("placing the fields: %v", err)
	}
}

func TestFieldPositionsMigrationKeepsTheCreationOrderOfEachWorkspace(t *testing.T) {
	t.Parallel()

	p, db, provider := rolledBackBefore(t, fieldPositionsVersion)
	home := sdk.TenantOrDefault(t.Context())
	acme := sdk.TenantOrDefault(inTenant(t, p))
	now := time.Now()
	definedAt(t, db, home, "shoeSize", now.Add(-time.Minute))
	definedAt(t, db, acme, "nickname", now.Add(-2*time.Minute))
	definedAt(t, db, home, "birthDate", now.Add(-3*time.Minute))
	definedAt(t, db, acme, "loyalty", now)

	placeFields(t, provider)

	if held := namesByPosition(t, db, home); !slices.Equal(held, []string{"birthDate", "shoeSize"}) {
		t.Errorf("home order = %v, want the oldest definition first", held)
	}
	if held := namesByPosition(t, db, acme); !slices.Equal(held, []string{"nickname", "loyalty"}) {
		t.Errorf("acme order = %v, want the oldest definition first", held)
	}
}

func TestFieldPositionsMigrationPlacesALaterDefinitionLast(t *testing.T) {
	t.Parallel()

	_, db, provider := rolledBackBefore(t, fieldPositionsVersion)
	home := sdk.TenantOrDefault(t.Context())
	now := time.Now()
	definedAt(t, db, home, "birthDate", now)
	definedAt(t, db, home, "shoeSize", now.Add(-time.Minute))
	placeFields(t, provider)

	storedDefinition(t, db, home, "nickname", "TEXT", "[]", false)

	if held := namesByPosition(t, db, home); !slices.Equal(held, []string{"shoeSize", "birthDate", "nickname"}) {
		t.Errorf("order = %v, want the definition stored after the migration last", held)
	}
}

// positionsHeld reports whether the definitions table holds the position column and its sequence.
func positionsHeld(t *testing.T, db *sql.DB) (bool, bool) {
	t.Helper()
	var column, sequence bool
	if err := db.QueryRowContext(t.Context(), `SELECT
		EXISTS (SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'plugin_fields' AND table_name = 'definitions' AND column_name = 'position'),
		to_regclass('plugin_fields.definitions_position_seq') IS NOT NULL`).Scan(&column, &sequence); err != nil {
		t.Fatalf("reading the schema: %v", err)
	}
	return column, sequence
}

func TestFieldPositionsMigrationGoesDownAndUpAgain(t *testing.T) {
	t.Parallel()

	_, db, provider := rolledBackBefore(t, fieldPositionsVersion)
	definedAt(t, db, sdk.TenantOrDefault(t.Context()), "birthDate", time.Now())
	placeFields(t, provider)
	if column, sequence := positionsHeld(t, db); !column || !sequence {
		t.Fatalf("column = %t, sequence = %t after the migration, want both held", column, sequence)
	}

	if _, err := provider.DownTo(t.Context(), fieldPositionsVersion-1); err != nil {
		t.Fatalf("rolling the positions back: %v", err)
	}

	if column, sequence := positionsHeld(t, db); column || sequence {
		t.Errorf("column = %t, sequence = %t after rolling back, want both gone", column, sequence)
	}
	placeFields(t, provider)
}
