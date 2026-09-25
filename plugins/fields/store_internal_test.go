// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/peterldowns/pgtestdb"

	"github.com/gopherium/alphone/internal/testdb"
	"github.com/gopherium/alphone/sdk"
)

// newMigratedPlugin returns a plugin over a database carrying the fields schema.
func newMigratedPlugin(t *testing.T) *Plugin {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	cfg := pgtestdb.Custom(t, testdb.Config(), testdb.Migrator())
	p, err := Register(sdk.Deps{DatabaseURL: cfg.URL()})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = p.Stop(t.Context()) })
	if err := p.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	if err := p.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	return p
}

func TestStoreRoundTripsADefinition(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	definition := defined(t, "birthDate", "DATE")

	if err := p.store.define(t.Context(), definition); err != nil {
		t.Fatalf("define() error = %v, want nil", err)
	}

	held, err := p.store.liveDefinitions(t.Context())
	if err != nil {
		t.Fatalf("liveDefinitions() error = %v, want nil", err)
	}
	if len(held) != 1 {
		t.Fatalf("definitions = %d, want 1", len(held))
	}
	if held[0].Name != "birthDate" || held[0].Kind != kindDate || held[0].Label != "Label" {
		t.Errorf("definition = %+v, want the stored row", held[0])
	}
	if held[0].ArchivedAt != nil {
		t.Error("archivedAt is set, want a live definition")
	}
}

func TestStoreRefusesADuplicateName(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	if err := p.store.define(t.Context(), defined(t, "birthDate", "DATE")); err != nil {
		t.Fatalf("define() error = %v, want nil", err)
	}

	err := p.store.define(t.Context(), defined(t, "birthDate", "TEXT"))

	if !errors.Is(err, errNameTaken) {
		t.Errorf("error = %v, want errNameTaken", err)
	}
}

func TestStoreRevivesAnArchivedDefinition(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	original := defined(t, "birthDate", "DATE")
	if err := p.store.define(t.Context(), original); err != nil {
		t.Fatalf("define() error = %v, want nil", err)
	}
	if err := p.store.archive(t.Context(), original.ID); err != nil {
		t.Fatalf("archive() error = %v, want nil", err)
	}

	if err := p.store.define(t.Context(), defined(t, "birthDate", "DATE")); err != nil {
		t.Fatalf("define() error = %v, want the archived definition revived", err)
	}

	live, err := p.store.liveDefinitions(t.Context())
	if err != nil {
		t.Fatalf("liveDefinitions() error = %v, want nil", err)
	}
	if len(live) != 1 {
		t.Fatalf("live definitions = %d, want the revived one", len(live))
	}
	if live[0].ID != original.ID {
		t.Errorf("id = %s, want the original %s kept so stored values stay reachable", live[0].ID, original.ID)
	}
}

func TestStoreClearsStrayValuesWhenATenantFirstDefinesANameAnotherTenantHolds(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	acme := inTenant(t, p)
	contactID := seedContact(t, p, "Maria Perez")
	definedField(t, p, t.Context(), "shoeSize")
	stray := map[string]any{"shoeSize": "44", "nickname": "kept"}
	if err := p.store.writeValues(acme, contactID, stray); err != nil {
		t.Fatalf("writeValues() in Acme error = %v, want nil", err)
	}
	if err := p.store.writeValues(t.Context(), contactID, map[string]any{"shoeSize": "38"}); err != nil {
		t.Fatalf("writeValues() elsewhere error = %v, want nil", err)
	}

	if err := p.store.define(acme, defined(t, "shoeSize", "NUMBER")); err != nil {
		t.Fatalf("define() error = %v, want nil", err)
	}

	if held := storedValues(t, p, acme, contactID); held["shoeSize"] != nil || held["nickname"] != "kept" {
		t.Errorf("Acme's values = %+v, want only the stray shoeSize cleared", held)
	}
	if held := storedValues(t, p, t.Context(), contactID); held["shoeSize"] != "38" {
		t.Errorf("the other tenant's values = %+v, want its shoeSize untouched", held)
	}
}

func TestStoreKeepsTheValuesOfARevivedDefinition(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	contactID := seedContact(t, p, "Maria Perez")
	original := defined(t, "birthDate", "DATE")
	if err := p.store.define(t.Context(), original); err != nil {
		t.Fatalf("define() error = %v, want nil", err)
	}
	if err := p.store.writeValues(t.Context(), contactID, map[string]any{"birthDate": "1990-04-17"}); err != nil {
		t.Fatalf("writeValues() error = %v, want nil", err)
	}
	if err := p.store.archive(t.Context(), original.ID); err != nil {
		t.Fatalf("archive() error = %v, want nil", err)
	}

	if err := p.store.define(t.Context(), defined(t, "birthDate", "DATE")); err != nil {
		t.Fatalf("define() error = %v, want the archived definition revived", err)
	}

	if held := storedValues(t, p, t.Context(), contactID); held["birthDate"] != "1990-04-17" {
		t.Errorf("values = %+v, want the revived field's value kept", held)
	}
}

// awaitLockedDefine waits until a define statement is blocked on a row lock.
func awaitLockedDefine(t *testing.T, p *Plugin) {
	t.Helper()
	const query = `SELECT count(*) FROM pg_stat_activity
		WHERE datname = current_database() AND wait_event_type = 'Lock' AND query LIKE 'WITH stored AS%'`
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var waiting int
		if err := p.pool.QueryRow(t.Context(), query).Scan(&waiting); err != nil {
			t.Fatalf("reading the waiting statements: %v", err)
		}
		if waiting > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no define statement waited on the racing insert")
}

func TestStoreKeepsALiveValueWhenARacingDefineIsRefused(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	contactID := seedContact(t, p, "Maria Perez")
	if err := p.store.writeValues(t.Context(), contactID, map[string]any{"loyalty": "stray"}); err != nil {
		t.Fatalf("writeValues() error = %v, want nil", err)
	}
	racing, err := p.pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("beginning the racing define: %v", err)
	}
	t.Cleanup(func() { _ = racing.Rollback(context.Background()) })
	if _, err := racing.Exec(t.Context(), `INSERT INTO plugin_fields.definitions
		(id, name, label, kind, created_at, tenant_id) VALUES ($1, 'loyalty', 'Loyalty', 'TEXT', now(), $2)`,
		uuid.Must(uuid.NewV7()), sdk.DefaultTenantID); err != nil {
		t.Fatalf("racing insert: %v", err)
	}
	if _, err := racing.Exec(t.Context(), `UPDATE plugin_fields.contact_values
		SET values = '{"loyalty":"kept"}'::jsonb WHERE contact_id = $1`, contactID); err != nil {
		t.Fatalf("racing value: %v", err)
	}
	second := defined(t, "loyalty", "TEXT")
	done := make(chan error, 1)
	go func() { done <- p.store.define(context.Background(), second) }()

	awaitLockedDefine(t, p)
	if err := racing.Commit(t.Context()); err != nil {
		t.Fatalf("committing the racing define: %v", err)
	}

	if err := <-done; !errors.Is(err, errNameTaken) {
		t.Fatalf("define() error = %v, want errNameTaken", err)
	}
	if held := storedValues(t, p, t.Context(), contactID); held["loyalty"] != "kept" {
		t.Errorf("values = %+v, want the racing define's value kept", held)
	}
}

func TestStoreClearsNothingWhenItRefusesADefinition(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	contactID := seedContact(t, p, "Maria Perez")
	if err := p.store.define(t.Context(), defined(t, "birthDate", "DATE")); err != nil {
		t.Fatalf("define() error = %v, want nil", err)
	}
	if err := p.store.writeValues(t.Context(), contactID, map[string]any{"birthDate": "1990-04-17"}); err != nil {
		t.Fatalf("writeValues() error = %v, want nil", err)
	}

	if err := p.store.define(t.Context(), defined(t, "birthDate", "DATE")); !errors.Is(err, errNameTaken) {
		t.Fatalf("define() error = %v, want errNameTaken", err)
	}

	if held := storedValues(t, p, t.Context(), contactID); held["birthDate"] != "1990-04-17" {
		t.Errorf("values = %+v, want the live field's value kept", held)
	}
}

func TestStoreRefusesRevivingUnderADifferentKind(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	original := defined(t, "birthDate", "DATE")
	if err := p.store.define(t.Context(), original); err != nil {
		t.Fatalf("define() error = %v, want nil", err)
	}
	if err := p.store.archive(t.Context(), original.ID); err != nil {
		t.Fatalf("archive() error = %v, want nil", err)
	}

	err := p.store.define(t.Context(), defined(t, "birthDate", "TEXT"))

	if !errors.Is(err, errKindLocked) {
		t.Errorf("error = %v, want errKindLocked", err)
	}
}

// checkViolation is the SQLSTATE of a row a CHECK constraint refuses.
const checkViolation = "23514"

// historyOf builds the history repeater holding the given sub fields.
func historyOf(subFields ...SubField) Definition {
	return Definition{
		ID:        uuid.Must(uuid.NewV7()),
		Name:      "history",
		Label:     "Label",
		Kind:      kindRepeater,
		SubFields: subFields,
		CreatedAt: time.Now().UTC(),
	}
}

// historyColumns are the sub fields of the repeater most store tests keep.
var historyColumns = []SubField{
	{Name: "date", Label: "Date", Kind: kindDate},
	{Name: "comment", Label: "Comment", Kind: kindLongText},
}

// archivedRepeater stores a history repeater and archives it, returning the stored definition.
func archivedRepeater(t *testing.T, p *Plugin) Definition {
	t.Helper()
	original := historyOf(historyColumns...)
	if err := p.store.define(t.Context(), original); err != nil {
		t.Fatalf("define() error = %v, want nil", err)
	}
	if err := p.store.archive(t.Context(), original.ID); err != nil {
		t.Fatalf("archive() error = %v, want nil", err)
	}
	return original
}

func TestStoreRoundTripsARepeatersSubFieldsInOrder(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)

	if err := p.store.define(t.Context(), historyOf(historyColumns...)); err != nil {
		t.Fatalf("define() error = %v, want nil", err)
	}

	held, err := p.store.liveDefinitions(t.Context())
	if err != nil {
		t.Fatalf("liveDefinitions() error = %v, want nil", err)
	}
	if len(held) != 1 || !slices.Equal(held[0].SubFields, historyColumns) {
		t.Errorf("definitions = %+v, want the repeater with its sub fields in order", held)
	}
}

func TestStoreRevivesARepeaterHoldingTheSameSubFields(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	original := archivedRepeater(t, p)
	relabelled := []SubField{
		{Name: "date", Label: "Day", Kind: kindDate},
		{Name: "comment", Label: "Note", Kind: kindLongText},
	}

	if err := p.store.define(t.Context(), historyOf(relabelled...)); err != nil {
		t.Fatalf("define() error = %v, want the archived repeater revived", err)
	}

	live, err := p.store.liveDefinitions(t.Context())
	if err != nil {
		t.Fatalf("liveDefinitions() error = %v, want nil", err)
	}
	if len(live) != 1 || live[0].ID != original.ID || !slices.Equal(live[0].SubFields, relabelled) {
		t.Errorf("definitions = %+v, want the original revived with the new sub labels", live)
	}
}

func TestStoreRefusesRevivingARepeaterUnderOtherSubFields(t *testing.T) {
	t.Parallel()

	cases := map[string][]SubField{
		"another name": {
			{Name: "date", Label: "Date", Kind: kindDate},
			{Name: "note", Label: "Comment", Kind: kindLongText},
		},
		"another kind": {
			{Name: "date", Label: "Date", Kind: kindText},
			{Name: "comment", Label: "Comment", Kind: kindLongText},
		},
		"fewer": {{Name: "date", Label: "Date", Kind: kindDate}},
	}
	for name, subFields := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p := newMigratedPlugin(t)
			archivedRepeater(t, p)

			err := p.store.define(t.Context(), historyOf(subFields...))

			if !errors.Is(err, errKindLocked) {
				t.Errorf("error = %v, want errKindLocked", err)
			}
		})
	}
}

func TestStoreRefusesSubFieldsOnlyARepeaterMayHold(t *testing.T) {
	t.Parallel()

	withSubFields := defined(t, "birthDate", "DATE")
	withSubFields.SubFields = historyColumns
	cases := map[string]Definition{
		"a repeater without sub fields": historyOf(),
		"a plain field with sub fields": withSubFields,
	}
	for name, definition := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p := newMigratedPlugin(t)

			err := p.store.define(t.Context(), definition)

			var refused *pgconn.PgError
			if !errors.As(err, &refused) || refused.Code != checkViolation {
				t.Errorf("error = %v, want the database to refuse the row", err)
			}
		})
	}
}

func TestStoreArchiveHidesADefinitionFromTheLiveListing(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	definition := defined(t, "birthDate", "DATE")
	if err := p.store.define(t.Context(), definition); err != nil {
		t.Fatalf("define() error = %v, want nil", err)
	}

	if err := p.store.archive(t.Context(), definition.ID); err != nil {
		t.Fatalf("archive() error = %v, want nil", err)
	}

	live, err := p.store.liveDefinitions(t.Context())
	if err != nil {
		t.Fatalf("liveDefinitions() error = %v, want nil", err)
	}
	if len(live) != 0 {
		t.Errorf("live = %v, want the archived definition hidden", live)
	}
	every, err := p.store.allDefinitions(t.Context())
	if err != nil {
		t.Fatalf("allDefinitions() error = %v, want nil", err)
	}
	if len(every) != 1 || every[0].ArchivedAt == nil {
		t.Errorf("all = %+v, want the archived definition listed with its timestamp", every)
	}
}

func TestStoreArchiveRefusesAnUnknownID(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)

	err := p.store.archive(t.Context(), uuid.Must(uuid.NewV7()))

	if !errors.Is(err, errNoDefinition) {
		t.Errorf("error = %v, want errNoDefinition", err)
	}
}

func TestStoreArchiveIsIdempotentOnALiveRow(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	definition := defined(t, "birthDate", "DATE")
	if err := p.store.define(t.Context(), definition); err != nil {
		t.Fatalf("define() error = %v, want nil", err)
	}
	if err := p.store.archive(t.Context(), definition.ID); err != nil {
		t.Fatalf("first archive() error = %v, want nil", err)
	}

	err := p.store.archive(t.Context(), definition.ID)

	if !errors.Is(err, errNoDefinition) {
		t.Errorf("error = %v, want an already archived definition refused", err)
	}
}

func TestStoreReportsRowsItCannotRead(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)

	_, err := p.store.query(t.Context(), "SELECT 1 WHERE $1::uuid IS NOT NULL")

	if err == nil {
		t.Fatal("query() error = nil, want the unreadable row reported")
	}
	if !strings.Contains(err.Error(), "read definitions") {
		t.Errorf("error = %v, want it to name the read", err)
	}
}

func TestStoreListsDefinitionsByCreation(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if err := p.store.define(t.Context(), defined(t, name, "TEXT")); err != nil {
			t.Fatalf("define(%q) error = %v, want nil", name, err)
		}
	}

	held, err := p.store.liveDefinitions(t.Context())

	if err != nil {
		t.Fatalf("liveDefinitions() error = %v, want nil", err)
	}
	for i, want := range []string{"alpha", "beta", "gamma"} {
		if held[i].Name != want {
			t.Errorf("definition %d = %q, want %q in creation order", i, held[i].Name, want)
		}
	}
}
