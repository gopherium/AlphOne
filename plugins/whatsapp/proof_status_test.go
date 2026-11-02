// SPDX-License-Identifier: Elastic-2.0

package whatsapp_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/sdk"
)

// seedOutboundOn stores one outbound message on a conversation, in the tenant of that conversation.
func seedOutboundOn(t *testing.T, pool *pgxpool.Pool, conversationID uuid.UUID, wamid string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO plugin_whatsapp.messages (id, conversation_id, external_id, direction, content,
			content_type, sent_at, raw, created_at, tenant_id)
		SELECT $1, id, $3, 'outbound', 'hello', 'text', now(), '{}', now(), tenant_id
		FROM plugin_whatsapp.conversations WHERE id = $2`,
		uuid.Must(uuid.NewV7()), conversationID, wamid,
	); err != nil {
		t.Fatalf("inserting outbound message: %v", err)
	}
}

// statusChange returns one change carrying a delivery status that arrived on number.
func statusChange(number, wamid string) string {
	return fmt.Sprintf(`{"field":"messages","value":{"metadata":{"phone_number_id":%q},`+
		`"statuses":[{"id":%q,"status":"delivered","timestamp":"1751791000","recipient_id":"184467235"}]}}`,
		number, wamid)
}

// wantStatus fails the test unless the message holds the delivery status.
func wantStatus(t *testing.T, pool *pgxpool.Pool, wamid, want string) {
	t.Helper()
	if status := messageStatus(t, pool, wamid); status == nil || *status != want {
		t.Errorf("the status of %s = %v, want %s", wamid, status, want)
	}
}

// wantNoStatus fails the test unless the message holds no delivery status.
func wantNoStatus(t *testing.T, pool *pgxpool.Pool, wamid string) {
	t.Helper()
	if status := messageStatus(t, pool, wamid); status != nil {
		t.Errorf("the status of %s = %q, want it left alone", wamid, *status)
	}
}

func TestAStatusOnAnotherTenantsNumberMovesNothing(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, nil)
	acme, globex := seedTenant(t, pool), seedGlobex(t, pool)
	connectNumber(t, p, acme, numberWithSecret("5550001", "acme-secret"))
	connectNumber(t, p, globex, numberWithSecret("5550002", "globex-secret"))
	seedOutboundRow(t, pool, "wamid.out.theirs", globex)
	seedOutboundRow(t, pool, "wamid.out.mine", acme)
	body := deliveryOf(statusChange("5550002", "wamid.out.theirs"), statusChange("5550001", "wamid.out.mine"))

	recorder := postEvent(t, p.Routes(), sign("acme-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	wantNoStatus(t, pool, "wamid.out.theirs")
	wantStatus(t, pool, "wamid.out.mine", "delivered")
}

func TestAStatusStaysOnTheNumberThatSignedIt(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, nil)
	acme := seedTenant(t, pool)
	first, second := "5550001", "5550002"
	connectNumber(t, p, acme, numberWithSecret(first, "first-secret"))
	connectNumber(t, p, acme, numberWithSecret(second, "second-secret"))
	onSecond := conversationOnNumber{tenantID: acme, number: &second, customer: "184467235"}
	seedOutboundOn(t, pool, seedNumberedConversation(t, pool, onSecond), "wamid.out.second")

	forged := numberedStatusBody(first, "wamid.out.second", "read")
	recorder := postEvent(t, p.Routes(), sign("first-secret", forged), forged)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d as for an unknown message", recorder.Code, http.StatusOK)
	}
	if status := messageStatus(t, pool, "wamid.out.second"); status != nil {
		t.Fatalf("the message status = %q, want it left alone by another number", *status)
	}

	own := numberedStatusBody(second, "wamid.out.second", "read")
	if recorder := postEvent(t, p.Routes(), sign("second-secret", own), own); recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	wantStatus(t, pool, "wamid.out.second", "read")
}

func TestAnEnvironmentStatusMovesAMessageOnItsOwnConversations(t *testing.T) {
	t.Parallel()

	env := "5550009"
	tests := map[string]*string{
		"a conversation holding no number":             nil,
		"a conversation still carrying the env number": &env,
	}
	for testName, number := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			p, pool := newRoutingPlugin(t, map[string]string{"ALPHONE_WHATSAPP_PHONE_NUMBER_ID": env})
			conversation := conversationOnNumber{tenantID: sdk.DefaultTenantID, number: number, customer: "184467235"}
			seedOutboundOn(t, pool, seedNumberedConversation(t, pool, conversation), "wamid.out.env")
			body := numberedStatusBody(env, "wamid.out.env", "delivered")

			recorder := postEvent(t, p.Routes(), sign("app-secret", body), body)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
			}
			wantStatus(t, pool, "wamid.out.env", "delivered")
		})
	}
}

func TestAnEnvironmentStatusLeavesAMessageOnAStoredNumberAlone(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, map[string]string{"ALPHONE_WHATSAPP_PHONE_NUMBER_ID": "5550009"})
	stored := "5550001"
	connectNumber(t, p, sdk.DefaultTenantID, numberWithSecret(stored, "stored-secret"))
	onStored := conversationOnNumber{tenantID: sdk.DefaultTenantID, number: &stored, customer: "184467235"}
	conversationID := seedNumberedConversation(t, pool, onStored)
	seedOutboundOn(t, pool, conversationID, "wamid.out.stored")
	body := numberedStatusBody("5550009", "wamid.out.stored", "delivered")

	recorder := postEvent(t, p.Routes(), sign("app-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	wantNoStatus(t, pool, "wamid.out.stored")
}

func TestAStoredNumberMovesAMessageOnAConversationOfItsTenantHoldingNoNumber(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, nil)
	acme, globex := seedTenant(t, pool), seedGlobex(t, pool)
	connectNumber(t, p, acme, numberWithSecret("5550001", "acme-secret"))
	older := conversationOnNumber{tenantID: acme, number: nil, customer: "184467235"}
	theirs := conversationOnNumber{tenantID: globex, number: nil, customer: "184467236"}
	seedOutboundOn(t, pool, seedNumberedConversation(t, pool, older), "wamid.out.older")
	seedOutboundOn(t, pool, seedNumberedConversation(t, pool, theirs), "wamid.out.theirs")
	body := deliveryOf(statusChange("5550001", "wamid.out.older"), statusChange("5550001", "wamid.out.theirs"))

	recorder := postEvent(t, p.Routes(), sign("acme-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	wantStatus(t, pool, "wamid.out.older", "delivered")
	wantNoStatus(t, pool, "wamid.out.theirs")
}
