// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

// refusalOf returns the graph refusal err carries, failing the test when it carries none.
func refusalOf(t *testing.T, err error) sdk.GraphError {
	t.Helper()
	var raised sdk.GraphError
	if !errors.As(err, &raised) {
		t.Fatalf("error = %v, want a graph refusal", err)
	}
	return raised
}

// wantRefusal fails unless err is a graph refusal of the given code and reason.
func wantRefusal(t *testing.T, err error, code, reason string) {
	t.Helper()
	if raised := refusalOf(t, err); raised.Code != code || raised.Reason != reason {
		t.Errorf("refusal = %s %s, want %s %s", raised.Code, raised.Reason, code, reason)
	}
}

// withHistory returns resolvers over a plugin holding a history repeater, and a contact to write on.
func withHistory(t *testing.T) (*Plugin, MutationResolvers, uuid.UUID) {
	t.Helper()
	p := newMigratedPlugin(t)
	define(t, p, historyOf(historyColumns...))
	return p, MutationResolvers{plugin: p}, seedContact(t, p, "Maria Perez")
}

// answeredID returns the id an answered entry carries.
func answeredID(t *testing.T, answered any) uuid.UUID {
	t.Helper()
	entry, isEntry := answered.(map[string]any)
	if !isEntry {
		t.Fatalf("answer = %#v, want an entry", answered)
	}
	return idOf(t, entry)
}

func TestAddContactFieldEntryAnswersTheStoredEntry(t *testing.T) {
	t.Parallel()

	p, resolvers, contactID := withHistory(t)

	answered, err := resolvers.AddContactFieldEntry(t.Context(), contactID, "history",
		map[string]any{"date": "2026-09-01", "comment": "First call."})

	if err != nil {
		t.Fatalf("AddContactFieldEntry() error = %v, want nil", err)
	}
	entry := answered.(map[string]any)
	if answeredID(t, answered).Version() != 7 || entry["date"] != "2026-09-01" || entry["comment"] != "First call." {
		t.Errorf("answer = %#v, want the stored entry under its new id", answered)
	}
	wantHistory(t, p, t.Context(), contactID, "First call.")
}

func TestAddContactFieldEntryRefusesAFieldThatIsNotARepeater(t *testing.T) {
	t.Parallel()

	p, resolvers, contactID := withHistory(t)
	define(t, p, defined(t, "birthDate", "DATE"))

	_, err := resolvers.AddContactFieldEntry(t.Context(), contactID, "birthDate", map[string]any{"date": "2026-09-01"})

	wantRefusal(t, err, "VALIDATION", "field_not_a_repeater")
}

func TestAddContactFieldEntryRefusesAFieldNoLiveDefinitionHolds(t *testing.T) {
	t.Parallel()

	p, resolvers, contactID := withHistory(t)
	listed, err := p.store.liveDefinitions(t.Context())
	if err != nil {
		t.Fatalf("liveDefinitions() error = %v, want nil", err)
	}
	if err := p.store.archive(t.Context(), listed[0].ID); err != nil {
		t.Fatalf("archive() error = %v, want nil", err)
	}
	p.catalog.forget(t.Context())

	for _, name := range []string{"history", "neverDefined"} {
		_, err := resolvers.AddContactFieldEntry(t.Context(), contactID, name, map[string]any{"comment": "call"})

		wantRefusal(t, err, "VALIDATION", "field_unknown")
	}
}

func TestAddContactFieldEntryRefusesAnEntryItCannotCheck(t *testing.T) {
	t.Parallel()

	_, resolvers, contactID := withHistory(t)
	cases := map[string]struct {
		entry  any
		reason string
	}{
		"every cell blank":     {map[string]any{"comment": "  "}, "field_entry_empty"},
		"a value of a kind":    {map[string]any{"date": "not a date"}, "value_kind_mismatch"},
		"an unknown sub field": {map[string]any{"mood": "happy"}, "field_unknown"},
		"a value not an entry": {"a text", "value_kind_mismatch"},
	}
	for name, held := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := resolvers.AddContactFieldEntry(t.Context(), contactID, "history", held.entry)

			wantRefusal(t, err, "VALIDATION", held.reason)
		})
	}
}

func TestAddContactFieldEntryRefusesAContactTheTenantDoesNotHold(t *testing.T) {
	t.Parallel()

	_, resolvers, _ := withHistory(t)

	_, err := resolvers.AddContactFieldEntry(t.Context(), uuid.Must(uuid.NewV7()), "history",
		map[string]any{"comment": "call"})

	wantRefusal(t, err, "NOT_FOUND", "contact_not_found")
}

func TestAddContactFieldEntryRefusesAFullListNamingTheCap(t *testing.T) {
	t.Parallel()

	p, resolvers, contactID := withHistory(t)
	p.entriesMax = 1
	if _, err := resolvers.AddContactFieldEntry(t.Context(), contactID, "history",
		map[string]any{"comment": "first"}); err != nil {
		t.Fatalf("AddContactFieldEntry() error = %v, want the first entry stored", err)
	}

	_, err := resolvers.AddContactFieldEntry(t.Context(), contactID, "history", map[string]any{"comment": "second"})

	wantRefusal(t, err, "CONFLICT", "field_entries_full")
	if meta := refusalOf(t, err).Meta; meta["max"] != 1 {
		t.Errorf("meta = %v, want the cap named", meta)
	}
	wantHistory(t, p, t.Context(), contactID, "first")
}

func TestUpdateContactFieldEntryAnswersTheEditedEntryInItsPlace(t *testing.T) {
	t.Parallel()

	p, resolvers, contactID := withHistory(t)
	older := added(t, p, t.Context(), contactID, "older")
	added(t, p, t.Context(), contactID, "newer")
	olderID := idOf(t, older[0])

	answered, err := resolvers.UpdateContactFieldEntry(t.Context(), contactID, "history", olderID,
		map[string]any{"id": olderID.String(), "comment": "older, edited"})

	if err != nil {
		t.Fatalf("UpdateContactFieldEntry() error = %v, want nil", err)
	}
	if answeredID(t, answered) != olderID || answered.(map[string]any)["comment"] != "older, edited" {
		t.Errorf("answer = %#v, want the edited entry under its own id", answered)
	}
	wantHistory(t, p, t.Context(), contactID, "newer", "older, edited")
}

func TestUpdateContactFieldEntryRefusesAnEntryItCannotFind(t *testing.T) {
	t.Parallel()

	_, resolvers, contactID := withHistory(t)

	_, err := resolvers.UpdateContactFieldEntry(t.Context(), contactID, "history", uuid.Must(uuid.NewV7()),
		map[string]any{"comment": "edited"})

	wantRefusal(t, err, "NOT_FOUND", "field_entry_not_found")
}

func TestUpdateContactFieldEntryRefusesABlankEntryAndKeepsTheOld(t *testing.T) {
	t.Parallel()

	p, resolvers, contactID := withHistory(t)
	stored := added(t, p, t.Context(), contactID, "kept")

	_, err := resolvers.UpdateContactFieldEntry(t.Context(), contactID, "history", idOf(t, stored[0]),
		map[string]any{"comment": ""})

	wantRefusal(t, err, "VALIDATION", "field_entry_empty")
	wantHistory(t, p, t.Context(), contactID, "kept")
}

func TestDeleteContactFieldEntryRemovesTheEntry(t *testing.T) {
	t.Parallel()

	p, resolvers, contactID := withHistory(t)
	stored := added(t, p, t.Context(), contactID, "gone", "kept")

	deleted, err := resolvers.DeleteContactFieldEntry(t.Context(), contactID, "history", idOf(t, stored[1]))

	if err != nil || !deleted {
		t.Fatalf("DeleteContactFieldEntry() = %v, %v, want true, nil", deleted, err)
	}
	wantHistory(t, p, t.Context(), contactID, "kept")
}

func TestDeleteContactFieldEntryRefusesAnEntryItCannotFind(t *testing.T) {
	t.Parallel()

	_, resolvers, contactID := withHistory(t)

	_, err := resolvers.DeleteContactFieldEntry(t.Context(), contactID, "history", uuid.Must(uuid.NewV7()))

	wantRefusal(t, err, "NOT_FOUND", "field_entry_not_found")
}

func TestUpdateAndDeleteRefuseAFieldThatIsNotARepeater(t *testing.T) {
	t.Parallel()

	p, resolvers, contactID := withHistory(t)
	define(t, p, defined(t, "birthDate", "DATE"))
	id := uuid.Must(uuid.NewV7())

	_, updated := resolvers.UpdateContactFieldEntry(t.Context(), contactID, "birthDate", id,
		map[string]any{"date": "2026-09-01"})
	_, deleted := resolvers.DeleteContactFieldEntry(t.Context(), contactID, "birthDate", id)

	wantRefusal(t, updated, "VALIDATION", "field_not_a_repeater")
	wantRefusal(t, deleted, "VALIDATION", "field_not_a_repeater")
}

func TestEntryMutationsCheckTheCallersOwnFields(t *testing.T) {
	t.Parallel()

	p, resolvers, contactID := withHistory(t)
	stored := added(t, p, t.Context(), contactID, "kept")
	acme := inTenant(t, p)

	_, adding := resolvers.AddContactFieldEntry(acme, contactID, "history", map[string]any{"comment": "call"})
	wantRefusal(t, adding, "VALIDATION", "field_unknown")

	if _, err := p.store.define(acme, historyOf(historyColumns...)); err != nil {
		t.Fatalf("define() in Acme error = %v, want nil", err)
	}
	p.catalog.forget(acme)
	_, adding = resolvers.AddContactFieldEntry(acme, contactID, "history", map[string]any{"comment": "call"})
	_, updating := resolvers.UpdateContactFieldEntry(acme, contactID, "history", idOf(t, stored[0]),
		map[string]any{"comment": "edited"})
	_, deleting := resolvers.DeleteContactFieldEntry(acme, contactID, "history", idOf(t, stored[0]))

	wantRefusal(t, adding, "NOT_FOUND", "contact_not_found")
	wantRefusal(t, updating, "NOT_FOUND", "field_entry_not_found")
	wantRefusal(t, deleting, "NOT_FOUND", "field_entry_not_found")
	wantHistory(t, p, t.Context(), contactID, "kept")
}

func TestEntryMutationsReportAFailedCatalogueRead(t *testing.T) {
	t.Parallel()

	resolvers := MutationResolvers{plugin: newWedgedPlugin(t)}
	id := uuid.Must(uuid.NewV7())

	_, adding := resolvers.AddContactFieldEntry(t.Context(), id, "history", map[string]any{"comment": "call"})
	_, updating := resolvers.UpdateContactFieldEntry(t.Context(), id, "history", id, map[string]any{"comment": "call"})
	_, deleting := resolvers.DeleteContactFieldEntry(t.Context(), id, "history", id)

	for name, err := range map[string]error{"add": adding, "update": updating, "delete": deleting} {
		if !errors.Is(err, errCatalogue) {
			t.Errorf("%s error = %v, want the catalogue failure", name, err)
		}
	}
}

func TestEntryMutationsReportAClosedPool(t *testing.T) {
	t.Parallel()

	p := newClosedPlugin(t)
	loader := newFakeLoader()
	loader.held[sdk.DefaultTenantID] = []Definition{historyOf(historyColumns...)}
	p.catalog = newCatalog(loader, 0, 0)
	resolvers := MutationResolvers{plugin: p}
	id := uuid.Must(uuid.NewV7())

	_, adding := resolvers.AddContactFieldEntry(t.Context(), id, "history", map[string]any{"comment": "call"})
	_, updating := resolvers.UpdateContactFieldEntry(t.Context(), id, "history", id, map[string]any{"comment": "call"})
	_, deleting := resolvers.DeleteContactFieldEntry(t.Context(), id, "history", id)

	for name, err := range map[string]error{"add": adding, "update": updating, "delete": deleting} {
		var raised sdk.GraphError
		if err == nil || errors.As(err, &raised) {
			t.Errorf("%s error = %v, want the closed pool reported as it is", name, err)
		}
	}
}
