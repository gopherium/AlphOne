// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

func TestSeedDefinesTheDemoFields(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)

	if err := p.Seed(t.Context()); err != nil {
		t.Fatalf("Seed() error = %v, want nil", err)
	}

	held, err := p.store.liveDefinitions(t.Context())
	if err != nil {
		t.Fatalf("liveDefinitions() error = %v, want nil", err)
	}
	if len(held) != 2 || held[0].Name != "birthDate" || held[0].Kind != kindDate {
		t.Fatalf("definitions = %+v, want birthDate of kind DATE first", held)
	}
	columns := []SubField{
		{Name: "date", Label: "Date", Kind: kindDate},
		{Name: "comment", Label: "Comment", Kind: kindLongText},
	}
	if held[1].Name != "history" || held[1].Kind != kindRepeater || !slices.Equal(held[1].SubFields, columns) {
		t.Errorf("definition = %+v, want the history repeater of a date and a comment", held[1])
	}
}

func TestSeedRunsTwiceWithoutDuplicating(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)

	for range 2 {
		if err := p.Seed(t.Context()); err != nil {
			t.Fatalf("Seed() error = %v, want nil", err)
		}
	}

	held, err := p.store.liveDefinitions(t.Context())
	if err != nil {
		t.Fatalf("liveDefinitions() error = %v, want nil", err)
	}
	if len(held) != 2 {
		t.Errorf("definitions = %d, want each demo field stored once", len(held))
	}
}

func TestSeedRevivesAnArchivedDemoField(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	if err := p.Seed(t.Context()); err != nil {
		t.Fatalf("Seed() error = %v, want nil", err)
	}
	held, err := p.store.liveDefinitions(t.Context())
	if err != nil {
		t.Fatalf("liveDefinitions() error = %v, want nil", err)
	}
	if err := p.store.archive(t.Context(), held[0].ID); err != nil {
		t.Fatalf("archive() error = %v, want nil", err)
	}

	if err := p.Seed(t.Context()); err != nil {
		t.Fatalf("second Seed() error = %v, want nil", err)
	}

	live, err := p.store.liveDefinitions(t.Context())
	if err != nil {
		t.Fatalf("liveDefinitions() error = %v, want nil", err)
	}
	if names := namesOf(live); !slices.Equal(names, []string{"history", "birthDate"}) {
		t.Errorf("live = %v, want the archived demo field revived at the end", names)
	}
}

// seedDemoContact stores a contact named Maria Perez holding the demo email.
func seedDemoContact(t *testing.T, p *Plugin) uuid.UUID {
	t.Helper()
	id := seedContact(t, p, "Maria Perez")
	if _, err := p.pool.Exec(t.Context(),
		`INSERT INTO core.contact_identities (id, contact_id, channel, identifier, display_name, created_at)
		VALUES ($1, $2, 'email', $3, '', now())`, uuid.Must(uuid.NewV7()), id, seedContactEmail); err != nil {
		t.Fatalf("seeding the contact's email: %v", err)
	}
	return id
}

func TestSeedWritesTheValuesOntoTheContactHoldingTheDemoEmail(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	namesake := seedContact(t, p, "Maria Perez")
	maria := seedDemoContact(t, p)

	if err := p.Seed(t.Context()); err != nil {
		t.Fatalf("Seed() error = %v, want nil", err)
	}

	held, err := p.store.valuesFor(t.Context(), []uuid.UUID{namesake, maria})
	if err != nil {
		t.Fatalf("valuesFor() error = %v, want nil", err)
	}
	if _, written := held[namesake]["history"]; written {
		t.Errorf("history landed on an older contact of the same name, want the one holding the demo email")
	}
	if held[maria]["history"] == nil {
		t.Errorf("values = %#v, want the history on the contact holding the demo email", held[maria])
	}
}

func TestSeedLeavesAnotherTenantsContactWithTheDemoEmailAlone(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	acme := inTenant(t, p)
	elsewhere := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(t.Context(),
		`INSERT INTO core.contacts (id, name, created_at, tenant_id) VALUES ($1, 'Maria Perez', now(), $2)`,
		elsewhere, sdk.TenantOrDefault(acme)); err != nil {
		t.Fatalf("seeding the other tenant's contact: %v", err)
	}
	if _, err := p.pool.Exec(t.Context(),
		`INSERT INTO core.contact_identities (id, contact_id, channel, identifier, display_name, created_at, tenant_id)
		VALUES ($1, $2, 'email', $3, '', now(), $4)`,
		uuid.Must(uuid.NewV7()), elsewhere, seedContactEmail, sdk.TenantOrDefault(acme)); err != nil {
		t.Fatalf("seeding the other tenant's email: %v", err)
	}
	maria := seedDemoContact(t, p)

	if err := p.Seed(t.Context()); err != nil {
		t.Fatalf("Seed() error = %v, want nil", err)
	}

	held, err := p.store.valuesFor(t.Context(), []uuid.UUID{maria})
	if err != nil {
		t.Fatalf("valuesFor() error = %v, want nil", err)
	}
	if held[maria]["history"] == nil {
		t.Errorf("values = %#v, want the history on the seeding tenant's contact", held[maria])
	}
	var strays int
	if err := p.pool.QueryRow(t.Context(),
		"SELECT count(*) FROM plugin_fields.contact_values WHERE contact_id = $1", elsewhere).Scan(&strays); err != nil {
		t.Fatalf("counting the other tenant's values: %v", err)
	}
	if strays != 0 {
		t.Errorf("the other tenant's contact holds %d value rows, want none", strays)
	}
}

func TestSeedWritesTheValuesOntoTheDemoContact(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	maria := seedDemoContact(t, p)

	if err := p.Seed(t.Context()); err != nil {
		t.Fatalf("Seed() error = %v, want nil", err)
	}

	held, err := p.store.valuesFor(t.Context(), []uuid.UUID{maria})
	if err != nil {
		t.Fatalf("valuesFor() error = %v, want nil", err)
	}
	if held[maria]["birthDate"] != "1990-04-17" {
		t.Errorf("birthDate = %#v, want 1990-04-17", held[maria]["birthDate"])
	}
	ids, cells := splitIDs(t, held[maria]["history"])
	rows := []any{
		map[string]any{"date": "2026-09-18", "comment": "Follow-up call.\nAsked for a second quote."},
		map[string]any{"date": "2026-09-10", "comment": "Sent the offer and booked a follow-up call."},
		map[string]any{"date": "2026-09-01", "comment": "First call about the yearly plan."},
	}
	if !reflect.DeepEqual(cells, rows) {
		t.Errorf("history = %#v, want the three demo entries newest first", cells)
	}
	if len(slices.Compact(slices.Sorted(slices.Values(ids)))) != 3 {
		t.Errorf("ids = %q, want each demo entry under its own id", ids)
	}
}

func TestSeedKeepsTheNewestDemoHistoryACapHolds(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	maria := seedDemoContact(t, p)
	p.entriesMax = 2

	if err := p.Seed(t.Context()); err != nil {
		t.Fatalf("Seed() error = %v, want the demo history cut to a cap of two", err)
	}

	held, err := p.store.valuesFor(t.Context(), []uuid.UUID{maria})
	if err != nil {
		t.Fatalf("valuesFor() error = %v, want nil", err)
	}
	_, cells := splitIDs(t, held[maria]["history"])
	rows := []any{
		map[string]any{"date": "2026-09-18", "comment": "Follow-up call.\nAsked for a second quote."},
		map[string]any{"date": "2026-09-10", "comment": "Sent the offer and booked a follow-up call."},
	}
	if !reflect.DeepEqual(cells, rows) {
		t.Errorf("history = %#v, want the two newest demo entries", cells)
	}
	if held[maria]["birthDate"] != "1990-04-17" {
		t.Errorf("birthDate = %#v, want 1990-04-17", held[maria]["birthDate"])
	}
}

func TestSeedReportsAHistoryItCannotStore(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	seedDemoContact(t, p)
	if _, err := p.pool.Exec(t.Context(),
		"ALTER TABLE plugin_fields.contact_values ADD CONSTRAINT no_history CHECK (NOT values ? 'history')"); err != nil {
		t.Fatalf("refusing stored histories: %v", err)
	}

	err := p.Seed(t.Context())

	if err == nil || !strings.Contains(err.Error(), "seed history") {
		t.Errorf("Seed() error = %v, want the refused history reported", err)
	}
}

func TestSeedKeepsWhatTheDemoContactAlreadyHolds(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	maria := seedDemoContact(t, p)
	if err := p.Seed(t.Context()); err != nil {
		t.Fatalf("first Seed() error = %v, want nil", err)
	}
	edited := []map[string]any{{"date": "2026-09-20", "comment": "Called back."}}
	if err := p.store.writeValues(t.Context(), maria, map[string]any{"history": edited}); err != nil {
		t.Fatalf("writeValues() error = %v, want nil", err)
	}

	if err := p.Seed(t.Context()); err != nil {
		t.Fatalf("second Seed() error = %v, want nil", err)
	}

	held, err := p.store.valuesFor(t.Context(), []uuid.UUID{maria})
	if err != nil {
		t.Fatalf("valuesFor() error = %v, want nil", err)
	}
	kept := []any{map[string]any{"date": "2026-09-20", "comment": "Called back."}}
	if !reflect.DeepEqual(held[maria]["history"], kept) {
		t.Errorf("history = %#v, want the edited history kept through a second seed", held[maria]["history"])
	}
	if held[maria]["birthDate"] != "1990-04-17" {
		t.Errorf("birthDate = %#v, want the demo date still there", held[maria]["birthDate"])
	}
}

func TestSeedReportsValuesItCannotRead(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	seedDemoContact(t, p)
	if _, err := p.pool.Exec(t.Context(), "DROP TABLE plugin_fields.contact_values"); err != nil {
		t.Fatalf("dropping the values table: %v", err)
	}

	if err := p.seedValues(t.Context()); err == nil {
		t.Error("seedValues() error = nil, want the unread values reported")
	}
}

func TestSeedSkipsTheValueWithoutTheDemoContact(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)

	if err := p.Seed(t.Context()); err != nil {
		t.Fatalf("Seed() error = %v, want nil", err)
	}

	held, err := p.store.liveDefinitions(t.Context())
	if err != nil {
		t.Fatalf("liveDefinitions() error = %v, want nil", err)
	}
	if len(held) != 2 {
		t.Errorf("definitions = %d, want the fields defined even with no contact", len(held))
	}
}

func TestSeedReportsADemoFieldItCannotDefine(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	archived := labelled(t, "history", "History", "TEXT")
	define(t, p, archived)
	if err := p.store.archive(t.Context(), archived.ID); err != nil {
		t.Fatalf("archive() error = %v, want nil", err)
	}

	err := p.Seed(t.Context())

	if !errors.Is(err, errKindLocked) {
		t.Errorf("Seed() error = %v, want the archived text field of that name reported in the way", err)
	}
}

func TestSeedReportsAClosedPool(t *testing.T) {
	t.Parallel()

	p := newClosedPlugin(t)

	if err := p.Seed(t.Context()); err == nil {
		t.Error("Seed() error = nil, want the closed pool reported")
	}
	if err := p.seedValues(t.Context()); err == nil {
		t.Error("seedValues() error = nil, want the contact lookup refused")
	}
}

func TestSeedRenewsTheDefaultTenantsFields(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	mustView(t, p.catalog, t.Context())

	if err := p.Seed(t.Context()); err != nil {
		t.Fatalf("Seed() error = %v, want nil", err)
	}

	held := mustView(t, p.catalog, t.Context()).kinds
	if held["birthDate"] != kindDate || held["history"] != kindRepeater {
		t.Errorf("the default tenant's fields = %v, want both seeded fields known", held)
	}
}
