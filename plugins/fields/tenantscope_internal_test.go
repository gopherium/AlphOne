// SPDX-License-Identifier: AGPL-3.0-or-later

package fields

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

// inTenant returns a context serving one seeded tenant named Acme.
func inTenant(t *testing.T, p *Plugin) context.Context {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(t.Context(),
		"INSERT INTO core.tenants (id, name) VALUES ($1, 'Acme')", id); err != nil {
		t.Fatalf("seeding the tenant: %v", err)
	}
	return sdk.WithTenant(t.Context(), id)
}

// definedField stores one live definition in the tenant the context serves.
func definedField(t *testing.T, p *Plugin, ctx context.Context, name string) {
	t.Helper()
	held := Definition{
		ID: uuid.Must(uuid.NewV7()), Name: name, Label: name, Kind: "TEXT", CreatedAt: time.Now(),
	}
	if err := p.store.define(ctx, held); err != nil {
		t.Fatalf("defining %s: %v", name, err)
	}
}

func TestADefinitionListingStaysInsideItsTenant(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	acme := inTenant(t, p)
	definedField(t, p, acme, "birthday")
	definedField(t, p, t.Context(), "shoeSize")

	listed, err := p.store.liveDefinitions(acme)

	if err != nil {
		t.Fatalf("liveDefinitions() error = %v, want nil", err)
	}
	if len(listed) != 1 || listed[0].Name != "birthday" {
		t.Errorf("liveDefinitions() = %+v, want only the tenant's own field", listed)
	}
}

func TestArchivingStaysInsideItsTenant(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	acme := inTenant(t, p)
	held := Definition{
		ID: uuid.Must(uuid.NewV7()), Name: "birthday", Label: "Birthday",
		Kind: "TEXT", CreatedAt: time.Now(),
	}
	if err := p.store.define(acme, held); err != nil {
		t.Fatalf("define() error = %v, want nil", err)
	}

	if err := p.store.archive(t.Context(), held.ID); err == nil {
		t.Error("archive() from another tenant succeeded, want the definition withheld")
	}
}

func TestAValueBagIsNotWrittenFromAnotherTenant(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	acme := inTenant(t, p)
	contactID := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(t.Context(),
		"INSERT INTO core.contacts (id, name, created_at) VALUES ($1, $2, now())",
		contactID, "Maria Perez"); err != nil {
		t.Fatalf("seeding the contact: %v", err)
	}
	if err := p.store.writeValues(acme, contactID, map[string]any{"birthday": "1990-01-01"}); err != nil {
		t.Fatalf("writeValues() in Acme error = %v, want nil", err)
	}

	if err := p.store.writeValues(t.Context(), contactID, map[string]any{"birthday": "2000-12-31"}); err != nil {
		t.Fatalf("writeValues() elsewhere error = %v, want its own bag admitted", err)
	}

	held, err := p.store.valuesFor(acme, []uuid.UUID{contactID})
	if err != nil {
		t.Fatalf("valuesFor() error = %v, want nil", err)
	}
	if held[contactID]["birthday"] != "1990-01-01" {
		t.Errorf("the Acme bag = %+v, want it untouched by the other tenant", held[contactID])
	}
}

func TestAValueBagStaysInsideItsTenant(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	acme := inTenant(t, p)
	contactID := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(t.Context(),
		"INSERT INTO core.contacts (id, name, created_at) VALUES ($1, $2, now())",
		contactID, "Maria Perez"); err != nil {
		t.Fatalf("seeding the contact: %v", err)
	}
	if err := p.store.writeValues(acme, contactID, map[string]any{"birthday": "1990-01-01"}); err != nil {
		t.Fatalf("writeValues() error = %v, want nil", err)
	}

	held, err := p.store.valuesFor(t.Context(), []uuid.UUID{contactID})

	if err != nil {
		t.Fatalf("valuesFor() error = %v, want nil", err)
	}
	if len(held) != 0 {
		t.Errorf("valuesFor() = %+v from another tenant, want the bag withheld", held)
	}
}
