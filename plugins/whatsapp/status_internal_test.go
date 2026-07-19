// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func seedOutboundMessage(t *testing.T, p *Plugin, externalID string) uuid.UUID {
	t.Helper()
	ctx := t.Context()
	now := time.Now().UTC()
	contactID := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(ctx,
		`INSERT INTO core.contacts (id, name, created_at) VALUES ($1, $2, $3)`,
		contactID, "María Pérez", now,
	); err != nil {
		t.Fatalf("inserting contact: %v", err)
	}
	conversationID := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(ctx,
		`INSERT INTO plugin_whatsapp.conversations (id, contact_id, channel, external_id, status,
			last_activity_at, created_at)
		VALUES ($1, $2, 'whatsapp', $3, 'open', $4, $4)`,
		conversationID, contactID, externalID, now,
	); err != nil {
		t.Fatalf("inserting conversation: %v", err)
	}
	if _, err := p.pool.Exec(ctx,
		`INSERT INTO plugin_whatsapp.messages (id, conversation_id, external_id, direction, content,
			content_type, sent_at, raw, created_at)
		VALUES ($1, $2, $3, 'outbound', 'hola', 'text', $4, '{}', $4)`,
		uuid.Must(uuid.NewV7()), conversationID, externalID, now,
	); err != nil {
		t.Fatalf("inserting outbound message: %v", err)
	}
	return conversationID
}

func messageStatusOf(t *testing.T, p *Plugin, wamid string) (*string, *string) {
	t.Helper()
	var status, detail *string
	err := p.pool.QueryRow(t.Context(),
		`SELECT status, status_detail FROM plugin_whatsapp.messages WHERE external_id = $1`, wamid,
	).Scan(&status, &detail)
	if err != nil {
		t.Fatalf("loading message status: %v", err)
	}
	return status, detail
}

func drainEvents(subscription chan event) {
	for {
		select {
		case <-subscription:
		default:
			return
		}
	}
}

func applyStatusOK(t *testing.T, p *Plugin, u statusUpdate) {
	t.Helper()
	if err := p.applyStatus(t.Context(), u); err != nil {
		t.Fatalf("applyStatus(%+v) error = %v, want nil", u, err)
	}
}

func assertStatus(t *testing.T, p *Plugin, wamid, want string) {
	t.Helper()
	status, _ := messageStatusOf(t, p, wamid)
	if status == nil || *status != want {
		t.Fatalf("status = %v, want %q", status, want)
	}
}

func TestApplyStatusAdvancesThroughTheRanks(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	conversationID := seedOutboundMessage(t, p, "wamid.out.ranks")
	subscription := p.events.subscribe()
	defer p.events.unsubscribe(subscription)

	for _, status := range []string{"sent", "delivered", "read"} {
		applyStatusOK(t, p, statusUpdate{wamid: "wamid.out.ranks", status: status})
		assertStatus(t, p, "wamid.out.ranks", status)
		select {
		case got := <-subscription:
			if got.Conversation != conversationID {
				t.Fatalf("broadcast conversation = %s, want %s", got.Conversation, conversationID)
			}
		default:
			t.Fatalf("no broadcast after applying %q", status)
		}
	}
}

func TestApplyStatusKeepsHigherRanks(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	seedOutboundMessage(t, p, "wamid.out.keep")
	applyStatusOK(t, p, statusUpdate{wamid: "wamid.out.keep", status: "read"})
	subscription := p.events.subscribe()
	defer p.events.unsubscribe(subscription)

	applyStatusOK(t, p, statusUpdate{wamid: "wamid.out.keep", status: "delivered"})

	assertStatus(t, p, "wamid.out.keep", "read")
	select {
	case <-subscription:
		t.Fatal("stale delivered update broadcast an event, want none")
	default:
	}
}

func TestApplyStatusIgnoresDuplicates(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	seedOutboundMessage(t, p, "wamid.out.dup")
	applyStatusOK(t, p, statusUpdate{wamid: "wamid.out.dup", status: "delivered"})
	subscription := p.events.subscribe()
	defer p.events.unsubscribe(subscription)

	applyStatusOK(t, p, statusUpdate{wamid: "wamid.out.dup", status: "delivered"})

	assertStatus(t, p, "wamid.out.dup", "delivered")
	select {
	case <-subscription:
		t.Fatal("duplicate delivered update broadcast an event, want none")
	default:
	}
}

func TestApplyStatusPlayedOutranksRead(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	seedOutboundMessage(t, p, "wamid.out.voice")
	applyStatusOK(t, p, statusUpdate{wamid: "wamid.out.voice", status: "read"})

	applyStatusOK(t, p, statusUpdate{wamid: "wamid.out.voice", status: "played"})

	assertStatus(t, p, "wamid.out.voice", "played")
}

func TestApplyStatusFailedOverridesWithDetail(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	seedOutboundMessage(t, p, "wamid.out.fail")
	applyStatusOK(t, p, statusUpdate{wamid: "wamid.out.fail", status: "read"})
	subscription := p.events.subscribe()
	defer p.events.unsubscribe(subscription)

	applyStatusOK(t, p, statusUpdate{
		wamid: "wamid.out.fail", status: "failed", detail: "131047 Re-engagement message",
	})

	assertStatus(t, p, "wamid.out.fail", "failed")
	_, detail := messageStatusOf(t, p, "wamid.out.fail")
	if detail == nil || *detail != "131047 Re-engagement message" {
		t.Fatalf("status_detail = %v, want the failure detail", detail)
	}
	drainEvents(subscription)

	applyStatusOK(t, p, statusUpdate{wamid: "wamid.out.fail", status: "delivered"})
	assertStatus(t, p, "wamid.out.fail", "failed")
	applyStatusOK(t, p, statusUpdate{wamid: "wamid.out.fail", status: "failed", detail: "131047 again"})
	assertStatus(t, p, "wamid.out.fail", "failed")
	select {
	case <-subscription:
		t.Fatal("post-failure updates broadcast an event, want none")
	default:
	}
}

func TestApplyStatusIgnoresUnknownStatuses(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	seedOutboundMessage(t, p, "wamid.out.warn")
	subscription := p.events.subscribe()
	defer p.events.unsubscribe(subscription)

	applyStatusOK(t, p, statusUpdate{wamid: "wamid.out.warn", status: "warning"})

	status, _ := messageStatusOf(t, p, "wamid.out.warn")
	if status != nil {
		t.Fatalf("status = %q, want NULL for an unknown status kind", *status)
	}
	select {
	case <-subscription:
		t.Fatal("unknown status kind broadcast an event, want none")
	default:
	}
}

func TestApplyStatusIgnoresUnknownWamids(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	subscription := p.events.subscribe()
	defer p.events.unsubscribe(subscription)

	applyStatusOK(t, p, statusUpdate{wamid: "wamid.ghost", status: "delivered"})

	select {
	case <-subscription:
		t.Fatal("unknown wamid broadcast an event, want none")
	default:
	}
}

func TestApplyStatusGuardsInboundMessages(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	_, _ = seedMessage(t, p, "wamid.in.guard")

	applyStatusOK(t, p, statusUpdate{wamid: "wamid.in.guard", status: "delivered"})

	status, _ := messageStatusOf(t, p, "wamid.in.guard")
	if status != nil {
		t.Fatalf("inbound status = %q, want NULL", *status)
	}
}

func TestMessagesListCarriesDeliveryStatus(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	conversationID := seedOutboundMessage(t, p, "wamid.out.list")
	applyStatusOK(t, p, statusUpdate{
		wamid: "wamid.out.list", status: "failed", detail: "131047 Re-engagement message",
	})

	rows, err := p.store.listMessages(t.Context(), conversationID, 50)
	if err != nil {
		t.Fatalf("listMessages() error = %v, want nil", err)
	}

	if got, want := len(rows), 1; got != want {
		t.Fatalf("len(rows) = %d, want %d", got, want)
	}
	if rows[0].Status == nil || *rows[0].Status != "failed" {
		t.Errorf("row Status = %v, want failed", rows[0].Status)
	}
	if rows[0].StatusDetail == nil || *rows[0].StatusDetail != "131047 Re-engagement message" {
		t.Errorf("row StatusDetail = %v, want the failure detail", rows[0].StatusDetail)
	}

	request := httptest.NewRequest(http.MethodGet, "/conversations/"+conversationID.String()+"/messages", nil)
	recorder := httptest.NewRecorder()
	p.Routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var payload []struct {
		Status       *string `json:"status"`
		StatusDetail *string `json:"status_detail"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decoding messages: %v", err)
	}
	if len(payload) != 1 || payload[0].Status == nil || *payload[0].Status != "failed" {
		t.Fatalf("payload status = %+v, want failed", payload)
	}
	if payload[0].StatusDetail == nil || *payload[0].StatusDetail != "131047 Re-engagement message" {
		t.Errorf("payload status_detail = %v, want the failure detail", payload[0].StatusDetail)
	}

	inboundConversation, _ := seedMessage(t, p, "wamid.in.nullstatus")
	inboundRows, err := p.store.listMessages(t.Context(), inboundConversation, 50)
	if err != nil {
		t.Fatalf("listMessages() for the inbound thread error = %v, want nil", err)
	}
	if len(inboundRows) != 1 || inboundRows[0].Status != nil {
		t.Fatalf("inbound row Status = %+v, want nil", inboundRows)
	}
}

func TestApplyStatusReportsStoreFailure(t *testing.T) {
	t.Parallel()

	p := &Plugin{store: &store{pool: newUnreachablePool(t)}, events: newBroadcaster()}

	if err := p.applyStatus(t.Context(), statusUpdate{wamid: "wamid.x", status: "delivered"}); err == nil {
		t.Fatal("applyStatus() error = nil, want a store failure")
	}
}
