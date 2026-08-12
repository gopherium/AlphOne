// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"context"
	"errors"
	"testing"
)

// fakeLoader answers the catalogue with settable definitions.
type fakeLoader struct {
	held []Definition
	err  error
	runs int
}

// liveDefinitions reports the settable definitions.
func (f *fakeLoader) liveDefinitions(context.Context) ([]Definition, error) {
	f.runs++
	return f.held, f.err
}

// defined builds a live definition for a catalogue test.
func defined(t *testing.T, name, declared string) Definition {
	t.Helper()
	definition, err := newDefinition(name, "Label", declared, nil)
	if err != nil {
		t.Fatalf("newDefinition(%q) error = %v, want nil", name, err)
	}
	return definition
}

func TestCatalogStartsEmptyAtVersionZero(t *testing.T) {
	t.Parallel()

	held := newCatalog(&fakeLoader{})

	version, fields := held.snapshot()

	if version != 0 {
		t.Errorf("version = %d, want 0 before the first load", version)
	}
	if len(fields) != 0 {
		t.Errorf("fields = %v, want none before the first load", fields)
	}
}

func TestCatalogServesLoadedDefinitionsAsGraphFields(t *testing.T) {
	t.Parallel()

	loader := &fakeLoader{held: []Definition{defined(t, "birthDate", "DATE")}}
	held := newCatalog(loader)

	if err := held.reload(t.Context()); err != nil {
		t.Fatalf("reload() error = %v, want nil", err)
	}

	version, fields := held.snapshot()
	if version != 1 {
		t.Errorf("version = %d, want 1 after the first load", version)
	}
	if len(fields) != 1 {
		t.Fatalf("fields = %d, want 1", len(fields))
	}
	if fields[0].Entity != "Contact" || fields[0].Name != "birthDate" || fields[0].Type != "Date" {
		t.Errorf("field = %+v, want birthDate answering Date on Contact", fields[0])
	}
}

func TestCatalogBumpsTheVersionOnEveryReload(t *testing.T) {
	t.Parallel()

	held := newCatalog(&fakeLoader{})

	for want := uint64(1); want <= 3; want++ {
		if err := held.reload(t.Context()); err != nil {
			t.Fatalf("reload() error = %v, want nil", err)
		}
		if version, _ := held.snapshot(); version != want {
			t.Errorf("version = %d, want %d", version, want)
		}
	}
}

func TestCatalogKeepsTheLastSnapshotWhenALoadFails(t *testing.T) {
	t.Parallel()

	loader := &fakeLoader{held: []Definition{defined(t, "birthDate", "DATE")}}
	held := newCatalog(loader)
	if err := held.reload(t.Context()); err != nil {
		t.Fatalf("reload() error = %v, want nil", err)
	}
	loader.err = errors.New("catalogue unavailable")

	err := held.reload(t.Context())

	if err == nil {
		t.Fatal("reload() error = nil, want the failure reported")
	}
	version, fields := held.snapshot()
	if version != 1 || len(fields) != 1 {
		t.Errorf("snapshot = %d and %v, want the last good load kept", version, fields)
	}
}

func TestCatalogReportsWhetherANameIsTaken(t *testing.T) {
	t.Parallel()

	loader := &fakeLoader{held: []Definition{defined(t, "birthDate", "DATE")}}
	held := newCatalog(loader)
	if err := held.reload(t.Context()); err != nil {
		t.Fatalf("reload() error = %v, want nil", err)
	}

	if !held.holds("birthDate") {
		t.Error("holds(birthDate) = false, want the loaded name held")
	}
	if held.holds("neverDefined") {
		t.Error("holds(neverDefined) = true, want an unknown name absent")
	}
}
