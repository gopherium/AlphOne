// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"bytes"
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

	if _, err := p.store.define(t.Context(), definition); err != nil {
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
	if _, err := p.store.define(t.Context(), defined(t, "birthDate", "DATE")); err != nil {
		t.Fatalf("define() error = %v, want nil", err)
	}

	_, err := p.store.define(t.Context(), defined(t, "birthDate", "TEXT"))

	if !errors.Is(err, errNameTaken) {
		t.Errorf("error = %v, want errNameTaken", err)
	}
}

func TestStoreRevivesAnArchivedDefinition(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	original := defined(t, "birthDate", "DATE")
	if _, err := p.store.define(t.Context(), original); err != nil {
		t.Fatalf("define() error = %v, want nil", err)
	}
	if err := p.store.archive(t.Context(), original.ID); err != nil {
		t.Fatalf("archive() error = %v, want nil", err)
	}

	if _, err := p.store.define(t.Context(), defined(t, "birthDate", "DATE")); err != nil {
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

func TestStoreDefineAnswersTheIDOfARevivedDefinition(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	original := defined(t, "birthDate", "DATE")
	if _, err := p.store.define(t.Context(), original); err != nil {
		t.Fatalf("define() error = %v, want nil", err)
	}
	if err := p.store.archive(t.Context(), original.ID); err != nil {
		t.Fatalf("archive() error = %v, want nil", err)
	}

	id, err := p.store.define(t.Context(), defined(t, "birthDate", "DATE"))

	if err != nil || id != original.ID {
		t.Errorf("define() = %s, %v, want the original id %s", id, err, original.ID)
	}
}

func TestStoreDefineAnswersTheIDOfANewDefinition(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	definition := defined(t, "birthDate", "DATE")

	id, err := p.store.define(t.Context(), definition)

	if err != nil || id != definition.ID {
		t.Errorf("define() = %s, %v, want the new id %s", id, err, definition.ID)
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

	if _, err := p.store.define(acme, defined(t, "shoeSize", "NUMBER")); err != nil {
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
	if _, err := p.store.define(t.Context(), original); err != nil {
		t.Fatalf("define() error = %v, want nil", err)
	}
	if err := p.store.writeValues(t.Context(), contactID, map[string]any{"birthDate": "1990-04-17"}); err != nil {
		t.Fatalf("writeValues() error = %v, want nil", err)
	}
	if err := p.store.archive(t.Context(), original.ID); err != nil {
		t.Fatalf("archive() error = %v, want nil", err)
	}

	if _, err := p.store.define(t.Context(), defined(t, "birthDate", "DATE")); err != nil {
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
	go func() {
		_, err := p.store.define(context.Background(), second)
		done <- err
	}()

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
	if _, err := p.store.define(t.Context(), defined(t, "birthDate", "DATE")); err != nil {
		t.Fatalf("define() error = %v, want nil", err)
	}
	if err := p.store.writeValues(t.Context(), contactID, map[string]any{"birthDate": "1990-04-17"}); err != nil {
		t.Fatalf("writeValues() error = %v, want nil", err)
	}

	if _, err := p.store.define(t.Context(), defined(t, "birthDate", "DATE")); !errors.Is(err, errNameTaken) {
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
	if _, err := p.store.define(t.Context(), original); err != nil {
		t.Fatalf("define() error = %v, want nil", err)
	}
	if err := p.store.archive(t.Context(), original.ID); err != nil {
		t.Fatalf("archive() error = %v, want nil", err)
	}

	_, err := p.store.define(t.Context(), defined(t, "birthDate", "TEXT"))

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
	if _, err := p.store.define(t.Context(), original); err != nil {
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

	if _, err := p.store.define(t.Context(), historyOf(historyColumns...)); err != nil {
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

	if _, err := p.store.define(t.Context(), historyOf(relabelled...)); err != nil {
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

			_, err := p.store.define(t.Context(), historyOf(subFields...))

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

			_, err := p.store.define(t.Context(), definition)

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
	if _, err := p.store.define(t.Context(), definition); err != nil {
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
	if _, err := p.store.define(t.Context(), definition); err != nil {
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

// definedIDs stores one live text definition per name in the tenant the context serves and returns their ids in order.
func definedIDs(t *testing.T, p *Plugin, ctx context.Context, names ...string) []uuid.UUID {
	t.Helper()
	ids := make([]uuid.UUID, 0, len(names))
	for _, name := range names {
		held := defined(t, name, "TEXT")
		if _, err := p.store.define(ctx, held); err != nil {
			t.Fatalf("define(%q) error = %v, want nil", name, err)
		}
		ids = append(ids, held.ID)
	}
	return ids
}

// namesOf returns the names of the given definitions in the order they come.
func namesOf(definitions []Definition) []string {
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.Name)
	}
	return names
}

// listedNames returns the names the live and the full listing of the context's tenant give, in order.
func listedNames(t *testing.T, p *Plugin, ctx context.Context) ([]string, []string) {
	t.Helper()
	live, err := p.store.liveDefinitions(ctx)
	if err != nil {
		t.Fatalf("liveDefinitions() error = %v, want nil", err)
	}
	every, err := p.store.allDefinitions(ctx)
	if err != nil {
		t.Fatalf("allDefinitions() error = %v, want nil", err)
	}
	return namesOf(live), namesOf(every)
}

func TestStoreListsARevivedDefinitionLast(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	ids := definedIDs(t, p, t.Context(), "birthDate", "shoeSize")
	if err := p.store.archive(t.Context(), ids[0]); err != nil {
		t.Fatalf("archive() error = %v, want nil", err)
	}

	if _, err := p.store.define(t.Context(), defined(t, "birthDate", "TEXT")); err != nil {
		t.Fatalf("define() error = %v, want the archived definition revived", err)
	}

	live, every := listedNames(t, p, t.Context())
	if want := []string{"shoeSize", "birthDate"}; !slices.Equal(live, want) || !slices.Equal(every, want) {
		t.Errorf("live = %v, all = %v, want %v with the revived definition last", live, every, want)
	}
}

func TestStoreOrdersTheLiveDefinitions(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	ids := definedIDs(t, p, t.Context(), "birthDate", "shoeSize", "nickname")

	if err := p.store.order(t.Context(), []uuid.UUID{ids[2], ids[0], ids[1]}); err != nil {
		t.Fatalf("order() error = %v, want nil", err)
	}

	live, every := listedNames(t, p, t.Context())
	if want := []string{"nickname", "birthDate", "shoeSize"}; !slices.Equal(live, want) || !slices.Equal(every, want) {
		t.Errorf("live = %v, all = %v, want %v", live, every, want)
	}
}

func TestStoreListsANewDefinitionAfterTheOrderedOnes(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	ids := definedIDs(t, p, t.Context(), "birthDate", "shoeSize")
	if err := p.store.order(t.Context(), []uuid.UUID{ids[1], ids[0]}); err != nil {
		t.Fatalf("order() error = %v, want nil", err)
	}

	definedIDs(t, p, t.Context(), "nickname")

	if live, _ := listedNames(t, p, t.Context()); !slices.Equal(live, []string{"shoeSize", "birthDate", "nickname"}) {
		t.Errorf("live = %v, want the new definition last", live)
	}
}

func TestStoreOrderLeavesTheArchivedDefinitionsOut(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	ids := definedIDs(t, p, t.Context(), "birthDate", "shoeSize", "nickname")
	if err := p.store.archive(t.Context(), ids[1]); err != nil {
		t.Fatalf("archive() error = %v, want nil", err)
	}

	if err := p.store.order(t.Context(), []uuid.UUID{ids[2], ids[0]}); err != nil {
		t.Fatalf("order() error = %v, want the live definitions ordered", err)
	}

	if live, _ := listedNames(t, p, t.Context()); !slices.Equal(live, []string{"nickname", "birthDate"}) {
		t.Errorf("live = %v, want the live definitions in the given order", live)
	}
}

func TestStoreOrderAcceptsAnEmptyListWhenNoDefinitionIsLive(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	ids := definedIDs(t, p, t.Context(), "birthDate")
	if err := p.store.archive(t.Context(), ids[0]); err != nil {
		t.Fatalf("archive() error = %v, want nil", err)
	}

	if err := p.store.order(t.Context(), nil); err != nil {
		t.Errorf("order() error = %v, want the archived definition left out of an empty order", err)
	}
}

func TestStoreOrderRefusesAListThatDoesNotNameEachLiveDefinitionOnce(t *testing.T) {
	t.Parallel()

	cases := map[string]func(t *testing.T, p *Plugin, live []uuid.UUID) []uuid.UUID{
		"a missing id": func(_ *testing.T, _ *Plugin, live []uuid.UUID) []uuid.UUID {
			return live[1:]
		},
		"a duplicate id": func(_ *testing.T, _ *Plugin, live []uuid.UUID) []uuid.UUID {
			return slices.Concat(live, live[:1])
		},
		"a duplicate in place of another id": func(_ *testing.T, _ *Plugin, live []uuid.UUID) []uuid.UUID {
			return []uuid.UUID{live[0], live[0]}
		},
		"an archived id": func(t *testing.T, p *Plugin, live []uuid.UUID) []uuid.UUID {
			archived := definedIDs(t, p, t.Context(), "nickname")
			if err := p.store.archive(t.Context(), archived[0]); err != nil {
				t.Fatalf("archive() error = %v, want nil", err)
			}
			return slices.Concat(live, archived)
		},
		"an unknown id": func(_ *testing.T, _ *Plugin, live []uuid.UUID) []uuid.UUID {
			return slices.Concat(live, []uuid.UUID{uuid.Must(uuid.NewV7())})
		},
		"another tenant's id": func(t *testing.T, p *Plugin, live []uuid.UUID) []uuid.UUID {
			return slices.Concat(live, definedIDs(t, p, inTenant(t, p), "nickname"))
		},
	}
	for name, listed := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p := newMigratedPlugin(t)
			live := definedIDs(t, p, t.Context(), "birthDate", "shoeSize")
			given := listed(t, p, []uuid.UUID{live[1], live[0]})

			err := p.store.order(t.Context(), given)

			if !errors.Is(err, errOrderIncomplete) {
				t.Errorf("order() error = %v, want errOrderIncomplete", err)
			}
			if held, _ := listedNames(t, p, t.Context()); !slices.Equal(held, []string{"birthDate", "shoeSize"}) {
				t.Errorf("live = %v, want the refused order to leave the positions alone", held)
			}
		})
	}
}

func TestStoreOrderWaitsOnARacingArchiveAndRefusesTheArchivedID(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	ids := definedIDs(t, p, t.Context(), "birthDate", "shoeSize")
	racing := holding(t, p, "UPDATE plugin_fields.definitions SET archived_at = now() WHERE id = $1", ids[0])
	done := make(chan error, 1)
	go func() { done <- p.store.order(context.Background(), []uuid.UUID{ids[1], ids[0]}) }()

	awaitLocked(t, p, lockLiveStatement)
	if err := racing.Commit(t.Context()); err != nil {
		t.Fatalf("committing the racing archive: %v", err)
	}

	if err := <-done; !errors.Is(err, errOrderIncomplete) {
		t.Errorf("order() error = %v, want the definition archived meanwhile refused", err)
	}
}

// byID returns the ids sorted the way Postgres compares uuids, byte by byte.
func byID(ids []uuid.UUID) []uuid.UUID {
	sorted := slices.Clone(ids)
	slices.SortFunc(sorted, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
	return sorted
}

// lockedRow reports whether a transaction holds a row lock on the definition of the given id.
func lockedRow(t *testing.T, p *Plugin, id uuid.UUID) bool {
	t.Helper()
	const probe = `SELECT count(*) FROM (
			SELECT id FROM plugin_fields.definitions WHERE id = $1 FOR UPDATE SKIP LOCKED
		) AS free`
	var free int
	if err := p.pool.QueryRow(t.Context(), probe, id).Scan(&free); err != nil {
		t.Fatalf("probing the lock on %s: %v", id, err)
	}
	return free == 0
}

func TestStoreOrderLocksTheLiveDefinitionsByID(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	ids := byID(definedIDs(t, p, t.Context(), "birthDate", "shoeSize"))
	if err := p.store.order(t.Context(), []uuid.UUID{ids[1], ids[0]}); err != nil {
		t.Fatalf("order() error = %v, want nil", err)
	}
	racing := holding(t, p, "SELECT id FROM plugin_fields.definitions WHERE id = $1 FOR UPDATE", ids[1])
	done := make(chan error, 1)
	go func() { done <- p.store.order(context.Background(), ids) }()

	awaitLocked(t, p, lockLiveStatement)
	held := lockedRow(t, p, ids[0])
	if err := racing.Commit(t.Context()); err != nil {
		t.Fatalf("committing the racing lock: %v", err)
	}

	if err := <-done; err != nil {
		t.Fatalf("order() error = %v, want nil", err)
	}
	if !held {
		t.Error("the lower id was free while the order waited on the higher one, want the rows locked by id")
	}
}

func TestStoreListsTheArchivedDefinitionsAfterTheLiveOnes(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	ids := definedIDs(t, p, t.Context(), "birthDate", "shoeSize", "nickname", "jobTitle")
	if err := p.store.archive(t.Context(), ids[0]); err != nil {
		t.Fatalf("archive() error = %v, want nil", err)
	}

	if err := p.store.order(t.Context(), []uuid.UUID{ids[3], ids[2], ids[1]}); err != nil {
		t.Fatalf("order() error = %v, want nil", err)
	}

	_, every := listedNames(t, p, t.Context())
	if want := []string{"jobTitle", "nickname", "shoeSize", "birthDate"}; !slices.Equal(every, want) {
		t.Errorf("all = %v, want %v with the archived definition after the live ones", every, want)
	}
}

// droppedColumn returns a plugin whose definitions table lacks the named column.
func droppedColumn(t *testing.T, column string) *Plugin {
	t.Helper()
	p := newMigratedPlugin(t)
	if _, err := p.pool.Exec(t.Context(), "ALTER TABLE plugin_fields.definitions DROP COLUMN "+column); err != nil {
		t.Fatalf("dropping the column %s: %v", column, err)
	}
	return p
}

func TestStoreOrderReportsDefinitionsItCannotLock(t *testing.T) {
	t.Parallel()

	p := droppedColumn(t, "archived_at")

	err := p.store.order(t.Context(), nil)

	if err == nil || !strings.Contains(err.Error(), "order definitions") || !strings.Contains(err.Error(), "archived_at") {
		t.Errorf("order() error = %v, want the failed lock reported", err)
	}
}

func TestStoreOrderReportsPositionsItCannotWrite(t *testing.T) {
	t.Parallel()

	p := droppedColumn(t, "position")

	err := p.store.order(t.Context(), nil)

	if err == nil || !strings.Contains(err.Error(), "order definitions") || !strings.Contains(err.Error(), "position") {
		t.Errorf("order() error = %v, want the failed write reported", err)
	}
}

func TestStoreListsNewDefinitionsInTheOrderDefined(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if _, err := p.store.define(t.Context(), defined(t, name, "TEXT")); err != nil {
			t.Fatalf("define(%q) error = %v, want nil", name, err)
		}
	}

	held, err := p.store.liveDefinitions(t.Context())

	if err != nil {
		t.Fatalf("liveDefinitions() error = %v, want nil", err)
	}
	for i, want := range []string{"alpha", "beta", "gamma"} {
		if held[i].Name != want {
			t.Errorf("definition %d = %q, want %q in the order defined", i, held[i].Name, want)
		}
	}
}
