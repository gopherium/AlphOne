// SPDX-License-Identifier: Elastic-2.0

package fields_test

import (
	"reflect"
	"strings"
	"testing"

	gqlclient "github.com/99designs/gqlgen/client"
	"github.com/google/uuid"
)

// addEntryMutation adds one entry to a contact's repeater through the graph.
const addEntryMutation = `mutation($id: UUID!, $entry: JSON!) {
	addContactFieldEntry(contactId: $id, field: "history", entry: $entry)
}`

// defineHistory declares the history repeater of a date and a comment through the graph.
func defineHistory(t *testing.T, client *gqlclient.Client) {
	t.Helper()
	var created struct {
		DefineField struct {
			ID string `json:"id"`
		}
	}
	client.MustPost(`mutation { defineField(name: "history", label: "History", kind: REPEATER, subFields: [
		{name: "date", label: "Date", kind: DATE}, {name: "comment", label: "Comment", kind: LONGTEXT}]) { id } }`,
		&created)
}

// addedEntry adds one entry through the graph and answers it as the graph did.
func addedEntry(t *testing.T, client *gqlclient.Client, contactID uuid.UUID, entry map[string]any) map[string]any {
	t.Helper()
	var added struct {
		AddContactFieldEntry map[string]any
	}
	client.MustPost(addEntryMutation, &added,
		gqlclient.Var("id", contactID.String()), gqlclient.Var("entry", entry))
	return added.AddContactFieldEntry
}

// historyRead reads a contact's history through the graph.
func historyRead(t *testing.T, client *gqlclient.Client, contactID uuid.UUID) any {
	t.Helper()
	var read struct {
		Contact struct {
			Field any `json:"field"`
		}
	}
	client.MustPost(`query($id: UUID!) { contact(id: $id) { field(name: "history") } }`, &read,
		gqlclient.Var("id", contactID.String()))
	return read.Contact.Field
}

func TestGraphKeepsRepeaterEntriesOneAtATime(t *testing.T) {
	t.Parallel()

	client, contactID := newValuesClient(t)
	defineHistory(t, client)
	older := addedEntry(t, client, contactID, map[string]any{"date": "2026-09-01", "comment": "First call."})
	newer := addedEntry(t, client, contactID, map[string]any{"comment": "Sent the offer."})

	var updated struct {
		UpdateContactFieldEntry map[string]any
	}
	client.MustPost(`mutation($id: UUID!, $entryId: UUID!, $entry: JSON!) {
		updateContactFieldEntry(contactId: $id, field: "history", entryId: $entryId, entry: $entry)
	}`, &updated, gqlclient.Var("id", contactID.String()), gqlclient.Var("entryId", older["id"]),
		gqlclient.Var("entry", map[string]any{"date": "2026-09-02", "comment": "First call, moved."}))
	var deleted struct {
		DeleteContactFieldEntry bool
	}
	client.MustPost(`mutation($id: UUID!, $entryId: UUID!) {
		deleteContactFieldEntry(contactId: $id, field: "history", entryId: $entryId)
	}`, &deleted, gqlclient.Var("id", contactID.String()), gqlclient.Var("entryId", newer["id"]))

	want := []any{map[string]any{"id": older["id"], "date": "2026-09-02", "comment": "First call, moved."}}
	if held := historyRead(t, client, contactID); !reflect.DeepEqual(held, want) {
		t.Errorf("history = %#v, want the edited entry alone under its own id", held)
	}
	if updated.UpdateContactFieldEntry["id"] != older["id"] || !deleted.DeleteContactFieldEntry {
		t.Errorf("update = %#v, delete = %t, want the edit answered and the delete confirmed",
			updated.UpdateContactFieldEntry, deleted.DeleteContactFieldEntry)
	}
}

func TestGraphAnswersEntriesNewestFirst(t *testing.T) {
	t.Parallel()

	client, contactID := newValuesClient(t)
	defineHistory(t, client)
	first := addedEntry(t, client, contactID, map[string]any{"comment": "First call."})
	second := addedEntry(t, client, contactID, map[string]any{"comment": "Sent the offer."})

	held, _ := historyRead(t, client, contactID).([]any)

	if len(held) != 2 || !reflect.DeepEqual(held[0], second) || !reflect.DeepEqual(held[1], first) {
		t.Errorf("history = %#v, want the newest entry first, each as its add answered", held)
	}
}

func TestGraphAnswersAFullListAsAConflictNamingTheCap(t *testing.T) {
	t.Parallel()

	client := newFieldsClientWith(t, func(name string) string {
		if name == "ALPHONE_FIELDS_ENTRIES_MAX" {
			return "1"
		}
		return ""
	})
	contactID := seedGraphContact(t, client, "Maria Perez")
	defineHistory(t, client)
	addedEntry(t, client, contactID, map[string]any{"comment": "First call."})

	var added struct {
		AddContactFieldEntry map[string]any
	}
	err := client.Post(addEntryMutation, &added,
		gqlclient.Var("id", contactID.String()), gqlclient.Var("entry", map[string]any{"comment": "Sent the offer."}))

	for _, held := range []string{`"code":"CONFLICT"`, `"reason":"field_entries_full"`, `"max":1`} {
		if err == nil || !strings.Contains(err.Error(), held) {
			t.Errorf("error = %v, want it to carry %s", err, held)
		}
	}
}

func TestGraphAnswersAnAddToAnUnknownContactAsNotFound(t *testing.T) {
	t.Parallel()

	client, _ := newValuesClient(t)
	defineHistory(t, client)

	var added struct {
		AddContactFieldEntry map[string]any
	}
	err := client.Post(addEntryMutation, &added,
		gqlclient.Var("id", uuid.Must(uuid.NewV7()).String()), gqlclient.Var("entry", map[string]any{"comment": "x"}))

	for _, held := range []string{`"code":"NOT_FOUND"`, `"reason":"contact_not_found"`} {
		if err == nil || !strings.Contains(err.Error(), held) {
			t.Errorf("error = %v, want it to carry %s", err, held)
		}
	}
}
