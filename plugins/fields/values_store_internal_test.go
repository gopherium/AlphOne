// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"testing"

	"github.com/google/uuid"
)

// seedContact stores a contact the value tests hang their values on.
func seedContact(t *testing.T, p *Plugin, name string) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	_, err := p.pool.Exec(t.Context(),
		`INSERT INTO core.contacts (id, name, created_at) VALUES ($1, $2, now())`, id, name)
	if err != nil {
		t.Fatalf("seeding the contact: %v", err)
	}
	return id
}

func TestValuesRoundTripForOneContact(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	maria := seedContact(t, p, "Maria Perez")

	err := p.store.writeValues(t.Context(), maria, map[string]any{"birthDate": "1990-04-17"})

	if err != nil {
		t.Fatalf("writeValues() error = %v, want nil", err)
	}
	held, err := p.store.valuesFor(t.Context(), []uuid.UUID{maria})
	if err != nil {
		t.Fatalf("valuesFor() error = %v, want nil", err)
	}
	if held[maria]["birthDate"] != "1990-04-17" {
		t.Errorf("values = %#v, want the written date", held[maria])
	}
}

func TestValuesMergeRatherThanReplace(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	maria := seedContact(t, p, "Maria Perez")
	if err := p.store.writeValues(t.Context(), maria, map[string]any{"birthDate": "1990-04-17"}); err != nil {
		t.Fatalf("first writeValues() error = %v, want nil", err)
	}

	if err := p.store.writeValues(t.Context(), maria, map[string]any{"loyaltyPoints": int64(420)}); err != nil {
		t.Fatalf("second writeValues() error = %v, want nil", err)
	}

	held, err := p.store.valuesFor(t.Context(), []uuid.UUID{maria})
	if err != nil {
		t.Fatalf("valuesFor() error = %v, want nil", err)
	}
	if held[maria]["birthDate"] != "1990-04-17" {
		t.Errorf("values = %#v, want the earlier key kept", held[maria])
	}
	if held[maria]["loyaltyPoints"] != float64(420) {
		t.Errorf("values = %#v, want the later key added", held[maria])
	}
}

func TestValuesDropAKeyWrittenNull(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	maria := seedContact(t, p, "Maria Perez")
	if err := p.store.writeValues(t.Context(), maria, map[string]any{"birthDate": "1990-04-17"}); err != nil {
		t.Fatalf("writeValues() error = %v, want nil", err)
	}

	if err := p.store.writeValues(t.Context(), maria, map[string]any{"birthDate": nil}); err != nil {
		t.Fatalf("clearing writeValues() error = %v, want nil", err)
	}

	held, err := p.store.valuesFor(t.Context(), []uuid.UUID{maria})
	if err != nil {
		t.Fatalf("valuesFor() error = %v, want nil", err)
	}
	if _, present := held[maria]["birthDate"]; present {
		t.Errorf("values = %#v, want the cleared key dropped", held[maria])
	}
}

func TestValuesForReadsManyContactsAtOnce(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	maria := seedContact(t, p, "Maria Perez")
	ada := seedContact(t, p, "Ada Lovelace")
	unwritten := seedContact(t, p, "Alan Turing")
	if err := p.store.writeValues(t.Context(), maria, map[string]any{"birthDate": "1990-04-17"}); err != nil {
		t.Fatalf("writeValues(maria) error = %v, want nil", err)
	}
	if err := p.store.writeValues(t.Context(), ada, map[string]any{"birthDate": "1815-12-10"}); err != nil {
		t.Fatalf("writeValues(ada) error = %v, want nil", err)
	}

	held, err := p.store.valuesFor(t.Context(), []uuid.UUID{maria, ada, unwritten})

	if err != nil {
		t.Fatalf("valuesFor() error = %v, want nil", err)
	}
	if held[maria]["birthDate"] != "1990-04-17" || held[ada]["birthDate"] != "1815-12-10" {
		t.Errorf("values = %#v, want both contacts read in one call", held)
	}
	if _, present := held[unwritten]; present {
		t.Errorf("values = %#v, want a contact with no row absent", held)
	}
}

func TestValuesRefuseAContactNoRowHolds(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)

	err := p.store.writeValues(t.Context(), uuid.Must(uuid.NewV7()), map[string]any{"birthDate": "1990-04-17"})

	if err == nil {
		t.Fatal("writeValues() error = nil, want the unknown contact refused")
	}
}

func TestValuesReportAClosedPool(t *testing.T) {
	t.Parallel()

	p := newClosedPlugin(t)

	if err := p.store.writeValues(t.Context(), uuid.Must(uuid.NewV7()), map[string]any{}); err == nil {
		t.Error("writeValues() error = nil, want the closed pool reported")
	}
	if _, err := p.store.valuesFor(t.Context(), []uuid.UUID{uuid.Must(uuid.NewV7())}); err == nil {
		t.Error("valuesFor() error = nil, want the closed pool reported")
	}
}
