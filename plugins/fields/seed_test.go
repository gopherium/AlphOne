// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestSeedDefinesTheDemoField(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)

	if err := p.Seed(t.Context()); err != nil {
		t.Fatalf("Seed() error = %v, want nil", err)
	}

	held, err := p.store.liveDefinitions(t.Context())
	if err != nil {
		t.Fatalf("liveDefinitions() error = %v, want nil", err)
	}
	if len(held) != 1 {
		t.Fatalf("definitions = %d, want the demo field", len(held))
	}
	if held[0].Name != seedFieldName || held[0].Kind != kindDate {
		t.Errorf("definition = %+v, want %s of kind DATE", held[0], seedFieldName)
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
	if len(held) != 1 {
		t.Errorf("definitions = %d, want the demo field stored once", len(held))
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
	if len(live) != 1 || live[0].Name != seedFieldName {
		t.Errorf("live = %+v, want the archived demo field revived", live)
	}
}

func TestSeedWritesTheValueOntoTheDemoContact(t *testing.T) {
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
	if held[maria][seedFieldName] != seedFieldValue {
		t.Errorf("values = %#v, want %s of %s", held[maria], seedFieldName, seedFieldValue)
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
	if len(held) != 1 {
		t.Errorf("definitions = %d, want the field defined even with no contact", len(held))
	}
}

func TestSeedReportsAClosedPool(t *testing.T) {
	t.Parallel()

	p := newClosedPlugin(t)

	if err := p.Seed(t.Context()); err == nil {
		t.Error("Seed() error = nil, want the closed pool reported")
	}
	if err := p.seedValue(t.Context()); err == nil {
		t.Error("seedValue() error = nil, want the contact lookup refused")
	}
}

func TestSeedReportsAFailedCatalogueReload(t *testing.T) {
	t.Parallel()

	p := newWedgedPlugin(t)

	err := p.Seed(t.Context())

	if !errors.Is(err, errCatalogue) {
		t.Errorf("Seed() error = %v, want the reload failure", err)
	}
}
