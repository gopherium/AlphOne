// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/google/uuid"
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
	if len(live) != 2 || live[0].Name != "birthDate" {
		t.Errorf("live = %+v, want the archived demo field revived", live)
	}
}

func TestSeedWritesTheValuesOntoTheDemoContact(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	maria := seedContact(t, p, seedContactName)

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
	rows := []any{
		map[string]any{"date": "2026-09-01", "comment": "First call about the yearly plan."},
		map[string]any{"date": "2026-09-10", "comment": "Sent the offer and booked a follow-up call."},
	}
	if !reflect.DeepEqual(held[maria]["history"], rows) {
		t.Errorf("history = %#v, want the two demo entries in order", held[maria]["history"])
	}
}

func TestSeedKeepsWhatTheDemoContactAlreadyHolds(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	maria := seedContact(t, p, seedContactName)
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
	seedContact(t, p, seedContactName)
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
