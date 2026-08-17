// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"strings"
	"testing"
	"time"

	"github.com/gopherium/alphone/internal/apitoken"
)

// agentScopes is the grant the agents guide tells an operator to mint.
const agentScopes = "contacts:read tasks:read"

// engineScopes is the grant the n8n and automation guides tell an operator to mint.
const engineScopes = "contacts:read meta:read tasks:write webhooks:write"

func TestEngineScopesReachEveryDocumentedOperation(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}

	w := newWorld(t)
	ctx := t.Context()
	secret, err := w.mintScopedSecretFor(ctx, w.ownerID, "engine", apitoken.ParseScopes(engineScopes))
	if err != nil {
		t.Fatalf("minting the engine token: %v", err)
	}
	w.scopedSecret = secret

	documented := map[string]string{
		"the credential test": `{"query":"query NodeCredentialTest { version }"}`,
		"listing webhooks":    `{"query":"{ webhooks { id url events } }"}`,
		"registering one": `{"query":"mutation { createWebhook(url: \"https://example.com/hook\",` +
			` events: [\"task.created\"]) { webhook { id } } }"}`,
		"creating a task": `{"query":"mutation { createTask(input: {title: \"Call the supplier\",` +
			` dueOn: \"2026-08-20\"}) { task { id } } }"}`,
		"looking a contact up": `{"query":"{ contacts(first: 1) { edges { node { id } } } }"}`,
	}
	for step, document := range documented {
		if err := w.postGraphScoped(ctx, document); err != nil {
			t.Fatalf("posting %s: %v", step, err)
		}
		if err := w.answeredWithoutRefusal(); err != nil {
			t.Errorf("%s was refused under %q: %v", step, engineScopes, err)
		}
	}
}

func TestAgentScopesReachEveryTool(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}

	w := newWorld(t)
	ctx := t.Context()
	contactID, err := w.seedContact(ctx, "Maria Perez")
	if err != nil {
		t.Fatalf("seeding the contact: %v", err)
	}
	if err := w.seedTask(ctx, "Call the supplier", time.Now().UTC(), "open", contactID); err != nil {
		t.Fatalf("seeding the task: %v", err)
	}
	secret, err := w.mintScopedSecretFor(ctx, w.ownerID, "agent", apitoken.ParseScopes(agentScopes))
	if err != nil {
		t.Fatalf("minting the agent token: %v", err)
	}
	if err := w.connect(ctx, secret); err != nil {
		t.Fatalf("connecting the agent: %v", err)
	}
	if w.session == nil {
		t.Fatalf("the agent never connected: %v", w.connErr)
	}

	calls := map[string]map[string]any{
		"workload_summary": {},
		"list_my_tasks":    {},
		"find_contacts":    {"query": "Maria"},
		"get_contact":      {"contact_id": contactID.String()},
	}
	for name, arguments := range calls {
		if err := w.callTool(ctx, name, arguments); err != nil {
			t.Fatalf("calling %s: %v", name, err)
		}
		if w.called.IsError {
			t.Errorf("%s was refused under %q: %s", name, agentScopes, contentText(w.called))
		}
	}
}

func TestTheGateChecksRootFieldsOnlyNotNestedTraversal(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}

	w := newWorld(t)
	ctx := t.Context()
	contactID, err := w.seedContact(ctx, "Maria Perez")
	if err != nil {
		t.Fatalf("seeding the contact: %v", err)
	}
	if err := w.seedTask(ctx, "Call the supplier", time.Now().UTC(), "open", contactID); err != nil {
		t.Fatalf("seeding the task: %v", err)
	}
	w.scopedSecret, err = w.mintScopedSecretFor(ctx, w.ownerID, "narrow", apitoken.ParseScopes("contacts:read"))
	if err != nil {
		t.Fatalf("minting the narrow token: %v", err)
	}

	if err := w.postGraphScoped(ctx,
		`{"query":"{ contact(id: \"`+contactID.String()+
			`\") { name tasks(first: 5) { edges { node { title } } } } }"}`); err != nil {
		t.Fatalf("posting the traversal: %v", err)
	}

	if err := w.answeredWithoutRefusal(); err != nil {
		t.Fatalf("the traversal was refused: %v", err)
	}
	if !strings.Contains(string(w.answered), "Call the supplier") {
		t.Errorf("answer = %s, want the nested task reached without tasks:read", w.answered)
	}
}
