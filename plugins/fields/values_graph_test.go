// SPDX-License-Identifier: Elastic-2.0

package fields_test

import (
	"strings"
	"testing"

	gqlclient "github.com/99designs/gqlgen/client"
	"github.com/google/uuid"
)

func TestGraphWritesAndReadsAValue(t *testing.T) {
	t.Parallel()

	client, contactID := newValuesClient(t)
	defineDate(t, client)

	var written struct{ WriteContactFields bool }
	client.MustPost(`mutation($id: UUID!, $values: JSON!) { writeContactFields(contactId: $id, values: $values) }`,
		&written, gqlclient.Var("id", contactID.String()),
		gqlclient.Var("values", map[string]any{"birthDate": "1990-04-17"}))

	if !written.WriteContactFields {
		t.Fatal("writeContactFields = false, want true")
	}
	var read struct {
		Contact struct {
			Field any `json:"field"`
		}
	}
	client.MustPost(`query($id: UUID!) { contact(id: $id) { field(name: "birthDate") } }`, &read,
		gqlclient.Var("id", contactID.String()))
	if read.Contact.Field != "1990-04-17" {
		t.Errorf("field = %#v, want the written date", read.Contact.Field)
	}
}

func TestGraphWritesAndReadsANumber(t *testing.T) {
	t.Parallel()

	client, contactID := newValuesClient(t)
	var defined struct {
		DefineField struct {
			ID string `json:"id"`
		}
	}
	client.MustPost(
		`mutation { defineField(name: "loyaltyPoints", label: "Loyalty points", kind: NUMBER) { id } }`,
		&defined)

	var written struct{ WriteContactFields bool }
	client.MustPost(`mutation($id: UUID!, $values: JSON!) { writeContactFields(contactId: $id, values: $values) }`,
		&written, gqlclient.Var("id", contactID.String()),
		gqlclient.Var("values", map[string]any{"loyaltyPoints": 420}))

	if !written.WriteContactFields {
		t.Fatal("writeContactFields = false, want the number stored")
	}
	var read struct {
		Contact struct {
			Field any `json:"field"`
		}
	}
	client.MustPost(`query($id: UUID!) { contact(id: $id) { field(name: "loyaltyPoints") } }`, &read,
		gqlclient.Var("id", contactID.String()))
	if read.Contact.Field != float64(420) {
		t.Errorf("field = %#v, want the written number", read.Contact.Field)
	}
}

func TestGraphAnswersNullForAnUnwrittenField(t *testing.T) {
	t.Parallel()

	client, contactID := newValuesClient(t)
	defineDate(t, client)

	var read struct {
		Contact struct {
			Field any `json:"field"`
		}
	}
	client.MustPost(`query($id: UUID!) { contact(id: $id) { field(name: "birthDate") } }`, &read,
		gqlclient.Var("id", contactID.String()))

	if read.Contact.Field != nil {
		t.Errorf("field = %#v, want null before anything is written", read.Contact.Field)
	}
}

func TestGraphRefusesAWriteNamingNoDefinition(t *testing.T) {
	t.Parallel()

	client, contactID := newValuesClient(t)

	var written struct{ WriteContactFields bool }
	err := client.Post(
		`mutation($id: UUID!, $values: JSON!) { writeContactFields(contactId: $id, values: $values) }`,
		&written, gqlclient.Var("id", contactID.String()),
		gqlclient.Var("values", map[string]any{"neverDefined": "x"}))

	if err == nil || !strings.Contains(err.Error(), "neverDefined") {
		t.Errorf("error = %v, want the undefined key named", err)
	}
}

func TestGraphRefusesAWriteOfTheWrongKind(t *testing.T) {
	t.Parallel()

	client, contactID := newValuesClient(t)
	defineDate(t, client)

	var written struct{ WriteContactFields bool }
	err := client.Post(
		`mutation($id: UUID!, $values: JSON!) { writeContactFields(contactId: $id, values: $values) }`,
		&written, gqlclient.Var("id", contactID.String()),
		gqlclient.Var("values", map[string]any{"birthDate": "not a date"}))

	if err == nil || !strings.Contains(err.Error(), "does not match the kind") {
		t.Errorf("error = %v, want the wrong kind refused", err)
	}
}

func TestGraphRefusesValuesThatAreNotAnObject(t *testing.T) {
	t.Parallel()

	client, contactID := newValuesClient(t)

	var written struct{ WriteContactFields bool }
	err := client.Post(
		`mutation($id: UUID!, $values: JSON!) { writeContactFields(contactId: $id, values: $values) }`,
		&written, gqlclient.Var("id", contactID.String()), gqlclient.Var("values", "plain text"))

	if err == nil || !strings.Contains(err.Error(), "object of field names") {
		t.Errorf("error = %v, want a non object refused", err)
	}
}

func TestGraphReadsManyContactsInOneBatch(t *testing.T) {
	t.Parallel()

	client, first := newValuesClient(t)
	defineDate(t, client)
	second := seedGraphContact(t, client, "Ada Lovelace")
	for id, day := range map[uuid.UUID]string{first: "1990-04-17", second: "1815-12-10"} {
		var written struct{ WriteContactFields bool }
		client.MustPost(
			`mutation($id: UUID!, $values: JSON!) { writeContactFields(contactId: $id, values: $values) }`,
			&written, gqlclient.Var("id", id.String()),
			gqlclient.Var("values", map[string]any{"birthDate": day}))
	}

	var listed struct {
		Contacts struct {
			Edges []struct {
				Node struct {
					Name  string `json:"name"`
					Field any    `json:"field"`
				} `json:"node"`
			} `json:"edges"`
		}
	}
	client.MustPost(`{ contacts(first: 10) { edges { node { name field(name: "birthDate") } } } }`, &listed)

	if len(listed.Contacts.Edges) != 2 {
		t.Fatalf("contacts = %d, want 2", len(listed.Contacts.Edges))
	}
	want := map[string]any{"Maria Perez": "1990-04-17", "Ada Lovelace": "1815-12-10"}
	for _, edge := range listed.Contacts.Edges {
		if edge.Node.Field != want[edge.Node.Name] {
			t.Errorf("%s field = %#v, want %#v", edge.Node.Name, edge.Node.Field, want[edge.Node.Name])
		}
	}
}
