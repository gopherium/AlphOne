// SPDX-License-Identifier: Elastic-2.0

package importer_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/gopherium/alphone/sdk"
)

func TestSeedFillsTheBirthDateOfEveryImportedContact(t *testing.T) {
	t.Parallel()

	provider := newFieldProvider(birthDate())
	p, pool, contacts := newServedPlugin(t, provider)
	contacts.seed("Ada Lovelace", "ada@example.com")

	if err := p.Seed(t.Context()); err != nil {
		t.Fatalf("Seed() error = %v, want nil", err)
	}

	want := map[string]string{
		"Maria Perez":  "1990-04-17",
		"Grace Hopper": "1906-12-09",
		"Alan Turing":  "1912-06-23",
	}
	for name, day := range want {
		id := contacts.byName(t, name)
		if provider.written[id]["birthDate"] != day {
			t.Errorf("%s birthDate = %q, want %q", name, provider.written[id]["birthDate"], day)
		}
	}
	if len(provider.written) != len(want) {
		t.Errorf("written = %v, want only the imported contacts filled", provider.written)
	}
	var columns []string
	var fourth string
	if err := pool.QueryRow(t.Context(),
		"SELECT columns, mapping->>'3' FROM plugin_importer.imports").Scan(&columns, &fourth); err != nil {
		t.Fatalf("reading the seeded mapping: %v", err)
	}
	if slices.Compare(columns, []string{"Name", "Email", "Phone", "Birth date"}) != 0 {
		t.Errorf("columns = %v, want the demo header to carry the field", columns)
	}
	if fourth != "birthDate" {
		t.Errorf("mapping of column 3 = %q, want birthDate", fourth)
	}
}

func TestSeedLeavesASkippedRowsContactUntouched(t *testing.T) {
	t.Parallel()

	provider := newFieldProvider(birthDate())
	p, _, contacts := newServedPlugin(t, provider)
	ada := contacts.seed("Ada Lovelace", "ada@example.com")

	if err := p.Seed(t.Context()); err != nil {
		t.Fatalf("Seed() error = %v, want nil", err)
	}

	if _, written := provider.written[ada.ID]; written {
		t.Errorf("written = %v, want the claimed contact left alone", provider.written)
	}
}

func TestSeedRefusesWhenTheProvidersLostTheDemoField(t *testing.T) {
	t.Parallel()

	provider := newFieldProvider(sdk.ContactField{Name: "loyaltyPoints", Label: "Loyalty points"})
	p, _, _ := newServedPlugin(t, provider)

	err := p.Seed(t.Context())

	if err == nil {
		t.Fatal("Seed() error = nil, want the missing demo field refused")
	}
	if !strings.Contains(err.Error(), "birthDate") {
		t.Errorf("error = %v, want it to name the field the demo import needs", err)
	}
}

func TestSeedStoresTheCoreColumnsWithoutAProvider(t *testing.T) {
	t.Parallel()

	p, pool, contacts := newServedPlugin(t)
	contacts.seed("Ada Lovelace", "ada@example.com")

	if err := p.Seed(t.Context()); err != nil {
		t.Fatalf("Seed() error = %v, want the demo import seeded without any provider", err)
	}

	var columns []string
	if err := pool.QueryRow(t.Context(),
		"SELECT columns FROM plugin_importer.imports").Scan(&columns); err != nil {
		t.Fatalf("reading the seeded columns: %v", err)
	}
	if slices.Compare(columns, []string{"Name", "Email", "Phone"}) != 0 {
		t.Errorf("columns = %v, want the core columns alone", columns)
	}
}

func TestSeedReportsAProviderOutage(t *testing.T) {
	t.Parallel()

	provider := newFieldProvider(birthDate())
	provider.listErr = errProvider
	p, _, _ := newServedPlugin(t, provider)

	if err := p.Seed(t.Context()); !errors.Is(err, errProvider) {
		t.Errorf("error = %v, want the outage reported", err)
	}
}

func TestSeedReportsAFieldWriteFailure(t *testing.T) {
	t.Parallel()

	provider := newFieldProvider(birthDate())
	provider.writeErr = errProvider
	p, _, _ := newServedPlugin(t, provider)

	if err := p.Seed(t.Context()); !errors.Is(err, errProvider) {
		t.Errorf("error = %v, want the failed field write reported", err)
	}
}
