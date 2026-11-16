// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/contact"
)

// countingContactStore is the fake contact store counting the reads of one contact.
type countingContactStore struct {
	*fakeContactStore
	gets atomic.Int32
}

// Get counts the read before the fake store answers it.
func (s *countingContactStore) Get(ctx context.Context, id uuid.UUID) (contact.Contact, error) {
	s.gets.Add(1)
	return s.fakeContactStore.Get(ctx, id)
}

// repeatedFieldBody is a graph request body asking one contact by a variable and by a literal of one value.
func repeatedFieldBody(t *testing.T, id uuid.UUID) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"query": `query($id: UUID!) { byVariable: contact(id: $id) { name } byLiteral: contact(id: "` +
			id.String() + `") { name } }`,
		"variables": map[string]any{"id": id.String()},
	})
	if err != nil {
		t.Fatalf("encoding the request: %v", err)
	}
	return string(body)
}

// rejectedRepeat fails unless the answer is the one rejection of field asked under two names.
func rejectedRepeat(t *testing.T, answered graphResponse, field string) {
	t.Helper()
	if len(answered.Errors) != 1 {
		t.Fatalf("errors = %+v, want the one rejection of %s", answered.Errors, field)
	}
	extensions := answered.Errors[0].Extensions
	meta, _ := extensions["meta"].(map[string]any)
	if extensions["code"] != "VALIDATION" || extensions["reason"] != "repeated_field" ||
		!maps.Equal(meta, map[string]any{"field": field}) {
		t.Errorf("extensions = %v, want VALIDATION with the reason repeated_field naming %s", extensions, field)
	}
	if string(answered.Data) != "null" {
		t.Errorf("data = %s, want none", answered.Data)
	}
}

func TestGraphQLRejectsAFieldAskedUnderTwoNamesBeforeAnyResolverRuns(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	addAda(t, users)
	store := &countingContactStore{fakeContactStore: newFakeContactStore()}
	maria, err := contact.New("Maria Perez")
	if err != nil {
		t.Fatalf("contact.New() error = %v, want nil", err)
	}
	if err := store.Create(t.Context(), maria); err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	srv := newGraphServer(t, graphConfig{Contacts: store, Users: users})
	cookie := loginCookie(t, srv)

	repeated := decodeBody[graphResponse](t, postGraphQL(t, srv, repeatedFieldBody(t, maria.ID), cookie))
	readsBefore := store.gets.Load()
	once := postGraphQL(t, srv, `{"query":"{ contact(id: \"`+maria.ID.String()+`\") { name } }"}`, cookie)

	rejectedRepeat(t, repeated, "Query.contact")
	if readsBefore != 0 {
		t.Errorf("the contact was read %d times before the rejection, want none", readsBefore)
	}
	if !strings.Contains(once.Body.String(), "Maria Perez") || store.gets.Load() != 1 {
		t.Errorf("one read answered %s after %d reads, want Maria Perez read once", once.Body.String(), store.gets.Load())
	}
}

// twoUploadsOperations stages two attached files as two imports under two names.
const twoUploadsOperations = `{"query":"mutation ($file: Upload!, $another: Upload!) { ` +
	`once: importUpload(file: $file) { id } twice: importUpload(file: $another) { id } }",` +
	`"variables":{"file":null,"another":null}}`

// twoUploadsForm returns a multipart post of two files alike in name, size and type but not in bytes.
func twoUploadsForm(t *testing.T) *http.Request {
	t.Helper()
	return graphForm(t, crmOrigin+"/api/graphql",
		formPart{name: "operations", value: twoUploadsOperations},
		formPart{name: "map", value: `{"0":["variables.file"],"1":["variables.another"]}`},
		formPart{name: "0", filename: "contacts.csv", value: "\x00\x01\x02"},
		formPart{name: "1", filename: "contacts.csv", value: "\x00\x01\x03"},
	)
}

func TestGraphQLRunsTwoUploadsAlikeInNameSizeAndTypeUnderTwoNames(t *testing.T) {
	t.Parallel()

	guarded := newGuardedServer(t)

	answered := decodeBody[graphResponse](t,
		guarded.serve(fromBrowser(twoUploadsForm(t), crmOrigin, "same-origin", guarded.cookie)))

	if len(answered.Errors) != 2 {
		t.Fatalf("errors = %+v, want the importer to read each upload once", answered.Errors)
	}
	for _, rejected := range answered.Errors {
		if reason := rejected.Extensions["reason"]; reason != "file_unreadable" {
			t.Errorf("reason = %v, want file_unreadable from the importer reading the upload", reason)
		}
	}
}

// oneUploadUnderTwoVariablesForm returns a multipart post whose map points one file at two variables.
func oneUploadUnderTwoVariablesForm(t *testing.T) *http.Request {
	t.Helper()
	return graphForm(t, crmOrigin+"/api/graphql",
		formPart{name: "operations", value: twoUploadsOperations},
		formPart{name: "map", value: `{"0":["variables.file","variables.another"]}`},
		formPart{name: "0", filename: "contacts.csv", value: "\x00\x01\x02"},
	)
}

func TestGraphQLRejectsOneUploadMappedToTwoVariablesUnderTwoNames(t *testing.T) {
	t.Parallel()

	guarded := newGuardedServer(t)

	answered := decodeBody[graphResponse](t,
		guarded.serve(fromBrowser(oneUploadUnderTwoVariablesForm(t), crmOrigin, "same-origin", guarded.cookie)))

	rejectedRepeat(t, answered, "Mutation.importUpload")
}

// spelledContactID is a contact ID whose hex digits include letters.
const spelledContactID = "0198c0de-0000-7000-8000-00000000abcd"

// twoSpellingsRequest returns a JSON graph post asking one contact under two names by two spellings of its ID.
func twoSpellingsRequest(t *testing.T) *http.Request {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"query": `{ once: contact(id: "` + spelledContactID + `") { name } ` +
			`twice: contact(id: "` + strings.ToUpper(spelledContactID) + `") { name } }`,
	})
	if err != nil {
		t.Fatalf("encoding the request: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, crmOrigin+"/api/graphql", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func TestGraphQLRejectsOneContactAskedUnderTwoSpellingsOfItsID(t *testing.T) {
	t.Parallel()

	guarded := newGuardedServer(t)

	answered := decodeBody[graphResponse](t,
		guarded.serve(fromBrowser(twoSpellingsRequest(t), crmOrigin, "same-origin", guarded.cookie)))

	rejectedRepeat(t, answered, "Query.contact")
}

func TestGraphQLRejectsAFieldAskedUnderTwoNamesBeforeSignIn(t *testing.T) {
	t.Parallel()

	srv := newGraphServer(t, graphConfig{Contacts: newFakeContactStore(), Users: newFakeUserStore()})

	answered := postGraphQL(t, srv, `{"query":"{ once: locale twice: locale }"}`, nil)

	rejectedRepeat(t, decodeBody[graphResponse](t, answered), "Query.locale")
}
