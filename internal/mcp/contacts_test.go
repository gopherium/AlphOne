// SPDX-License-Identifier: Elastic-2.0

package mcp

import (
	"strings"
	"testing"
)

// searchGraph answers a contact search with one busy and one free contact.
const searchGraph = `{"data":{"contacts":{"edges":[
	{"node":{"id":"c1","name":"Maria Perez",
		"identities":[{"channel":"whatsapp","identifier":"184467235","displayName":"Maria"}],
		"tasks":{"edges":[{"node":{"id":"t1"}}]}}},
	{"node":{"id":"c2","name":"Ada Lovelace","identities":[],"tasks":{"edges":[]}}}
]}}}`

func TestContactsMarksWhoHoldsOpenWork(t *testing.T) {
	t.Parallel()

	run := &tools{graph: answering(searchGraph)}

	result, out, err := run.contacts(t.Context(), ContactsInput{Query: "a"})

	if err != nil {
		t.Fatalf("contacts() error = %v, want nil", err)
	}
	if result != nil {
		t.Errorf("result = %v, want nil so the SDK mirrors the data", result)
	}
	if len(out.Contacts) != 2 {
		t.Fatalf("contacts = %d, want 2", len(out.Contacts))
	}
	busy, free := out.Contacts[0], out.Contacts[1]
	if !busy.HasOpenTasks {
		t.Errorf("%q has_open_tasks = false, want true", busy.Name)
	}
	if free.HasOpenTasks {
		t.Errorf("%q has_open_tasks = true, want false", free.Name)
	}
	if len(busy.Identities) != 1 || busy.Identities[0].Identifier != "184467235" {
		t.Errorf("identities = %+v, want the whatsapp address", busy.Identities)
	}
	if busy.Identities[0].DisplayName != "Maria" {
		t.Errorf("display name = %q, want Maria", busy.Identities[0].DisplayName)
	}
}

func TestContactsAnswersEmptyListsNotNull(t *testing.T) {
	t.Parallel()

	run := &tools{graph: answering(`{"data":{"contacts":{"edges":[]}}}`)}

	_, out, err := run.contacts(t.Context(), ContactsInput{})

	if err != nil {
		t.Fatalf("contacts() error = %v, want nil", err)
	}
	if out.Contacts == nil {
		t.Error("contacts = nil, want an empty list")
	}
}

func TestContactsCarriesAnEmptyIdentityListNotNull(t *testing.T) {
	t.Parallel()

	run := &tools{graph: answering(
		`{"data":{"contacts":{"edges":[{"node":{"id":"c1","name":"Ada Lovelace"}}]}}}`)}

	_, out, err := run.contacts(t.Context(), ContactsInput{})

	if err != nil {
		t.Fatalf("contacts() error = %v, want nil", err)
	}
	if out.Contacts[0].Identities == nil {
		t.Error("identities = nil, want an empty list")
	}
}

func TestContactsSendsTheQueryAndLimit(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		in    ContactsInput
		wants []string
	}{
		"the default page": {
			in:    ContactsInput{},
			wants: []string{`"first":20`},
		},
		"a search term": {
			in:    ContactsInput{Query: "184 467"},
			wants: []string{`"q":"184 467"`},
		},
		"a chosen limit": {
			in:    ContactsInput{Limit: 5},
			wants: []string{`"first":5`},
		},
		"a limit above the ceiling is clamped": {
			in:    ContactsInput{Limit: 500},
			wants: []string{`"first":50`},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var body string
			run := &tools{graph: recording(&body, `{"data":{"contacts":{"edges":[]}}}`)}

			if _, _, err := run.contacts(t.Context(), tc.in); err != nil {
				t.Fatalf("contacts() error = %v, want nil", err)
			}

			for _, want := range tc.wants {
				if !strings.Contains(body, want) {
					t.Errorf("request = %s, want it to carry %s", body, want)
				}
			}
		})
	}
}

func TestContactReadsOneContactInFull(t *testing.T) {
	t.Parallel()

	run := &tools{graph: answering(`{"data":{"contact":{
		"id":"c1","name":"Maria Perez","createdAt":"2026-08-01T10:00:00Z",
		"identities":[{"channel":"whatsapp","identifier":"184467235","displayName":""}],
		"tasks":{"edges":[{"node":{"id":"t1","title":"Call Maria Perez back",
			"dueOn":"2026-08-11","status":"open","priority":1}}]}
	}}}`)}

	_, out, err := run.contact(t.Context(), ContactInput{ContactID: "c1"})

	if err != nil {
		t.Fatalf("contact() error = %v, want nil", err)
	}
	if out.Name != "Maria Perez" || out.CreatedAt != "2026-08-01T10:00:00Z" {
		t.Errorf("contact = %+v, want the named contact", out)
	}
	if len(out.Identities) != 1 || out.Identities[0].Channel != "whatsapp" {
		t.Errorf("identities = %+v, want the whatsapp address", out.Identities)
	}
	if len(out.OpenTasks) != 1 || out.OpenTasks[0].Title != "Call Maria Perez back" {
		t.Errorf("open tasks = %+v, want the linked task", out.OpenTasks)
	}
}

func TestContactRefusesAnIdNoContactHolds(t *testing.T) {
	t.Parallel()

	run := &tools{graph: answering(`{"data":{"contact":null}}`)}

	_, _, err := run.contact(t.Context(), ContactInput{ContactID: "c1"})

	if err == nil {
		t.Fatal("contact() error = nil, want the missing contact refused")
	}
	if !strings.Contains(err.Error(), "no contact") {
		t.Errorf("error = %v, want it to say no contact was held", err)
	}
}

func TestContactReportsAGraphRefusal(t *testing.T) {
	t.Parallel()

	run := &tools{graph: answering(`{"errors":[{"message":"scalar: invalid value"}]}`)}

	_, _, err := run.contact(t.Context(), ContactInput{ContactID: "not-a-uuid"})

	if err == nil || !strings.Contains(err.Error(), "invalid value") {
		t.Errorf("error = %v, want the refusal", err)
	}
}

func TestContactsReportsAGraphRefusal(t *testing.T) {
	t.Parallel()

	run := &tools{graph: answering(`{"errors":[{"message":"refused","extensions":{"code":"VALIDATION"}}]}`)}

	_, _, err := run.contacts(t.Context(), ContactsInput{})

	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Errorf("error = %v, want the refusal", err)
	}
}
