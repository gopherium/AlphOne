// SPDX-License-Identifier: Elastic-2.0

package whatsapp_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/plugins/whatsapp"
)

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func eventBody(wamid, waID, name, timestamp, text string) []byte {
	return fmt.Appendf(nil, `{
		"object": "whatsapp_business_account",
		"entry": [{"id": "0", "changes": [{"field": "messages", "value": {
			"messaging_product": "whatsapp",
			"contacts": [{"wa_id": %q, "profile": {"name": %q}}],
			"messages": [{"from": %q, "id": %q, "timestamp": %q, "type": "text", "text": {"body": %q}}]
		}}]}]
	}`, waID, name, waID, wamid, timestamp, text)
}

func postEvent(t *testing.T, routes http.Handler, signature string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	if signature != "" {
		request.Header.Set("X-Hub-Signature-256", signature)
	}
	recorder := httptest.NewRecorder()
	routes.ServeHTTP(recorder, request)
	return recorder
}

func newIngestingPlugin(t *testing.T) (*whatsapp.Plugin, *pgxpool.Pool) {
	t.Helper()
	cfg := newTestDatabase(t)
	pool := newAssertionPool(t, cfg.URL())
	resolver := resolverBridge{resolver: contact.NewResolver(postgres.NewContactStore(pool))}
	p := newPlugin(t, cfg.URL(), resolver, map[string]string{
		"ALPHONE_WHATSAPP_APP_SECRET": "app-secret",
	})
	if err := p.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	return p, pool
}

func countRows(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("counting %s: %v", table, err)
	}
	return count
}

func TestWebhookEventsRejectsInvalidSignatures(t *testing.T) {
	t.Parallel()

	body := eventBody("wamid.1", "184467235", "María Pérez", "1751791000", "hola")

	tests := map[string]struct {
		configuredSecret string
		signature        string
	}{
		"missing signature":   {configuredSecret: "app-secret", signature: ""},
		"wrong signature":     {configuredSecret: "app-secret", signature: sign("other-secret", body)},
		"unconfigured secret": {configuredSecret: "", signature: sign("", body)},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			routes := newPlugin(t, "", nil, map[string]string{
				"ALPHONE_WHATSAPP_APP_SECRET": tc.configuredSecret,
			}).Routes()

			recorder := postEvent(t, routes, tc.signature, body)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
			}
		})
	}
}

func TestWebhookEventsRejectsMalformedPayloads(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		body []byte
	}{
		"garbage json": {body: []byte(`{"entry":`)},
		"oversized":    {body: []byte(strings.Repeat("x", 1<<20+1))},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			routes := newPlugin(t, "", nil, map[string]string{
				"ALPHONE_WHATSAPP_APP_SECRET": "app-secret",
			}).Routes()

			recorder := postEvent(t, routes, sign("app-secret", tc.body), tc.body)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestWebhookEventsIngestTextMessages(t *testing.T) {
	t.Parallel()

	p, pool := newIngestingPlugin(t)
	routes := p.Routes()
	first := eventBody("wamid.1", "184467235", "María Pérez", "1751791000", "hola")

	if recorder := postEvent(t, routes, sign("app-secret", first), first); recorder.Code != http.StatusOK {
		t.Fatalf("first event status = %d, want %d", recorder.Code, http.StatusOK)
	}

	var contactName, content string
	row := pool.QueryRow(t.Context(), `
		SELECT c.name, m.content
		FROM plugin_whatsapp.messages m
		JOIN plugin_whatsapp.conversations conv ON conv.id = m.conversation_id
		JOIN core.contacts c ON c.id = conv.contact_id
		WHERE m.external_id = 'wamid.1'`)
	if err := row.Scan(&contactName, &content); err != nil {
		t.Fatalf("loading ingested message: %v", err)
	}
	if contactName != "María Pérez" || content != "hola" {
		t.Errorf("ingested (%q, %q), want (%q, %q)", contactName, content, "María Pérez", "hola")
	}

	if recorder := postEvent(t, routes, sign("app-secret", first), first); recorder.Code != http.StatusOK {
		t.Fatalf("duplicate event status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := countRows(t, pool, "plugin_whatsapp.messages"); got != 1 {
		t.Errorf("messages after duplicate delivery = %d, want 1", got)
	}

	second := eventBody("wamid.2", "184467235", "María Pérez", "1751791100", "¿cómo estás?")
	if recorder := postEvent(t, routes, sign("app-secret", second), second); recorder.Code != http.StatusOK {
		t.Fatalf("second event status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := countRows(t, pool, "plugin_whatsapp.conversations"); got != 1 {
		t.Errorf("conversations after second message = %d, want 1 thread", got)
	}
	if got := countRows(t, pool, "plugin_whatsapp.messages"); got != 2 {
		t.Errorf("messages after second message = %d, want 2", got)
	}
	if got := countRows(t, pool, "core.contacts"); got != 1 {
		t.Errorf("contacts = %d, want 1", got)
	}
}

func TestWebhookEventsIngestMediaMessages(t *testing.T) {
	t.Parallel()

	p, pool := newIngestingPlugin(t)
	routes := p.Routes()
	body := []byte(`{
		"object": "whatsapp_business_account",
		"entry": [{"id": "0", "changes": [{"field": "messages", "value": {
			"messaging_product": "whatsapp",
			"contacts": [{"wa_id": "184467235", "profile": {"name": "María Pérez"}}],
			"messages": [{"from": "184467235", "id": "wamid.img", "timestamp": "1751791000", "type": "image",
				"image": {"id": "MEDIA1", "mime_type": "image/jpeg", "sha256": "c2hh", "caption": "la factura"}}]
		}}]}]
	}`)

	recorder := postEvent(t, routes, sign("app-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var content, contentType string
	row := pool.QueryRow(t.Context(),
		`SELECT content, content_type FROM plugin_whatsapp.messages WHERE external_id = 'wamid.img'`)
	if err := row.Scan(&content, &contentType); err != nil {
		t.Fatalf("loading ingested message: %v", err)
	}
	if content != "la factura" || contentType != "image" {
		t.Errorf("ingested (%q, %q), want (%q, %q)", content, contentType, "la factura", "image")
	}
}

func TestWebhookEventsSkipStatusOnlyEvents(t *testing.T) {
	t.Parallel()

	p, pool := newIngestingPlugin(t)
	body := []byte(`{
		"object": "whatsapp_business_account",
		"entry": [{"id": "0", "changes": [{"field": "messages", "value": {
			"messaging_product": "whatsapp",
			"statuses": [{"id": "wamid.9", "status": "delivered"}]
		}}]}]
	}`)

	recorder := postEvent(t, p.Routes(), sign("app-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := countRows(t, pool, "plugin_whatsapp.conversations"); got != 0 {
		t.Errorf("conversations = %d, want 0 for status-only events", got)
	}
}

func TestWebhookEventsReportIngestFailure(t *testing.T) {
	t.Parallel()

	p, pool := newIngestingPlugin(t)
	pool.Close()
	body := eventBody("wamid.1", "184467235", "María Pérez", "1751791000", "hola")

	recorder := postEvent(t, p.Routes(), sign("app-secret", body), body)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d so Meta retries", recorder.Code, http.StatusInternalServerError)
	}
}
