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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/plugins/whatsapp"
	"github.com/gopherium/alphone/sdk"
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
		"ALPHONE_WHATSAPP_APP_SECRET":      "app-secret",
		"ALPHONE_WHATSAPP_PHONE_NUMBER_ID": "555000111",
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

	body := eventBody("wamid.1", "184467235", "María Pérez", "1751791000", "hello")

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
	first := eventBody("wamid.1", "184467235", "María Pérez", "1751791000", "hello")

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
	if contactName != "María Pérez" || content != "hello" {
		t.Errorf("ingested (%q, %q), want (%q, %q)", contactName, content, "María Pérez", "hello")
	}

	if recorder := postEvent(t, routes, sign("app-secret", first), first); recorder.Code != http.StatusOK {
		t.Fatalf("duplicate event status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := countRows(t, pool, "plugin_whatsapp.messages"); got != 1 {
		t.Errorf("messages after duplicate delivery = %d, want 1", got)
	}

	second := eventBody("wamid.2", "184467235", "María Pérez", "1751791100", "how are you?")
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
				"image": {"id": "MEDIA1", "mime_type": "image/jpeg", "sha256": "c2hh", "caption": "the invoice"}}]
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
	if content != "the invoice" || contentType != "image" {
		t.Errorf("ingested (%q, %q), want (%q, %q)", content, contentType, "the invoice", "image")
	}
}

func statusEventBody(wamid, status string) []byte {
	return fmt.Appendf(nil, `{
		"object": "whatsapp_business_account",
		"entry": [{"id": "0", "changes": [{"field": "messages", "value": {
			"messaging_product": "whatsapp",
			"statuses": [{"id": %q, "status": %q, "timestamp": "1751791000",
				"recipient_id": "184467235"}]
		}}]}]
	}`, wamid, status)
}

func seedOutboundRow(t *testing.T, pool *pgxpool.Pool, wamid string, standing uuid.UUID) {
	t.Helper()
	ctx := t.Context()
	contactID := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(ctx,
		`INSERT INTO core.contacts (id, name, created_at, tenant_id) VALUES ($1, 'María Pérez', now(), $2)`,
		contactID, standing,
	); err != nil {
		t.Fatalf("inserting contact: %v", err)
	}
	conversationID := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(ctx,
		`INSERT INTO plugin_whatsapp.conversations (id, contact_id, channel, external_id, status,
			last_activity_at, created_at, tenant_id)
		VALUES ($1, $2, 'whatsapp', $3, 'open', now(), now(), $4)`,
		conversationID, contactID, wamid, standing,
	); err != nil {
		t.Fatalf("inserting conversation: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO plugin_whatsapp.messages (id, conversation_id, external_id, direction, content,
			content_type, sent_at, raw, created_at, tenant_id)
		VALUES ($1, $2, $3, 'outbound', 'hello', 'text', now(), '{}', now(), $4)`,
		uuid.Must(uuid.NewV7()), conversationID, wamid, standing,
	); err != nil {
		t.Fatalf("inserting outbound message: %v", err)
	}
}

func messageStatus(t *testing.T, pool *pgxpool.Pool, wamid string) *string {
	t.Helper()
	var status *string
	err := pool.QueryRow(t.Context(),
		`SELECT status FROM plugin_whatsapp.messages WHERE external_id = $1`, wamid,
	).Scan(&status)
	if err != nil {
		t.Fatalf("loading message status: %v", err)
	}
	return status
}

func TestWebhookEventsTolerateUnknownStatusWamids(t *testing.T) {
	t.Parallel()

	p, pool := newIngestingPlugin(t)
	body := statusEventBody("wamid.9", "delivered")

	recorder := postEvent(t, p.Routes(), sign("app-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := countRows(t, pool, "plugin_whatsapp.conversations"); got != 0 {
		t.Errorf("conversations = %d, want 0 for unknown status wamids", got)
	}
}

func TestWebhookEventsApplyDeliveryStatuses(t *testing.T) {
	t.Parallel()

	p, pool := newIngestingPlugin(t)
	seedOutboundRow(t, pool, "wamid.out.e2e", sdk.DefaultTenantID)
	body := statusEventBody("wamid.out.e2e", "delivered")

	recorder := postEvent(t, p.Routes(), sign("app-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	status := messageStatus(t, pool, "wamid.out.e2e")
	if status == nil || *status != "delivered" {
		t.Fatalf("message status = %v, want delivered", status)
	}
}

func TestWebhookEventsReportStatusFailure(t *testing.T) {
	t.Parallel()

	p, pool := newIngestingPlugin(t)
	seedOutboundRow(t, pool, "wamid.out.down", sdk.DefaultTenantID)
	if err := p.Stop(t.Context()); err != nil {
		t.Fatalf("Stop() error = %v, want nil", err)
	}
	body := statusEventBody("wamid.out.down", "delivered")

	recorder := postEvent(t, p.Routes(), sign("app-secret", body), body)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d so Meta retries", recorder.Code, http.StatusInternalServerError)
	}
}

func TestWebhookEventsReportIngestFailure(t *testing.T) {
	t.Parallel()

	p, pool := newIngestingPlugin(t)
	pool.Close()
	body := eventBody("wamid.1", "184467235", "María Pérez", "1751791000", "hello")

	recorder := postEvent(t, p.Routes(), sign("app-secret", body), body)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d so Meta retries", recorder.Code, http.StatusInternalServerError)
	}
}

// routingEnv returns the plugin environment for a routing test, overridden by extra.
func routingEnv(extra map[string]string) map[string]string {
	env := map[string]string{
		"ALPHONE_WHATSAPP_APP_SECRET":      "app-secret",
		"ALPHONE_WHATSAPP_CREDENTIALS_KEY": strings.Repeat("ab", 32),
	}
	for key, value := range extra {
		env[key] = value
	}
	return env
}

// newRoutingPlugin returns a migrated plugin holding a sealing key, beside its pool.
func newRoutingPlugin(t *testing.T, extra map[string]string) (*whatsapp.Plugin, *pgxpool.Pool) {
	t.Helper()
	cfg := newTestDatabase(t)
	pool := newAssertionPool(t, cfg.URL())
	resolver := resolverBridge{resolver: contact.NewResolver(postgres.NewContactStore(pool))}
	p := newPlugin(t, cfg.URL(), resolver, routingEnv(extra))
	if err := p.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	return p, pool
}

// seedTenant stores one tenant named Acme and returns its id.
func seedTenant(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO core.tenants (id, name) VALUES ($1, 'Acme')`, id); err != nil {
		t.Fatalf("seeding the tenant: %v", err)
	}
	return id
}

// numberedEventBody returns an inbound message payload naming the number it arrived on.
func numberedEventBody(phoneNumberID, wamid string) []byte {
	return fmt.Appendf(nil, `{
		"object": "whatsapp_business_account",
		"entry": [{"id": "0", "changes": [{"field": "messages", "value": {
			"messaging_product": "whatsapp",
			"metadata": {"display_phone_number": "5550000", "phone_number_id": %q},
			"contacts": [{"wa_id": "184467235", "profile": {"name": "Maria Perez"}}],
			"messages": [{"from": "184467235", "id": %q, "timestamp": "1751791000", "type": "text",
				"text": {"body": "hello"}}]
		}}]}]
	}`, phoneNumberID, wamid)
}

// numberedStatusBody returns a delivery status payload naming the number it arrived on.
func numberedStatusBody(phoneNumberID, wamid, status string) []byte {
	return fmt.Appendf(nil, `{
		"object": "whatsapp_business_account",
		"entry": [{"id": "0", "changes": [{"field": "messages", "value": {
			"messaging_product": "whatsapp",
			"metadata": {"display_phone_number": "5550000", "phone_number_id": %q},
			"statuses": [{"id": %q, "status": %q, "timestamp": "1751791000",
				"recipient_id": "184467235"}]
		}}]}]
	}`, phoneNumberID, wamid, status)
}

// conversationTenant returns the tenant of the single stored conversation.
func conversationTenant(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	var tenantID uuid.UUID
	if err := pool.QueryRow(t.Context(),
		`SELECT tenant_id FROM plugin_whatsapp.conversations`).Scan(&tenantID); err != nil {
		t.Fatalf("loading the conversation tenant: %v", err)
	}
	return tenantID
}

func TestWebhookEventsLandInTheNumbersTenant(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, nil)
	acme := seedTenant(t, pool)
	if err := p.SetCredentials(sdk.WithTenant(t.Context(), acme), "5550001", "EAAG-acme-token"); err != nil {
		t.Fatalf("SetCredentials() error = %v, want nil", err)
	}
	body := numberedEventBody("5550001", "wamid.routed")

	recorder := postEvent(t, p.Routes(), sign("app-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if held := conversationTenant(t, pool); held != acme {
		t.Errorf("conversation tenant = %s, want the number's tenant", held)
	}
	var contactTenant uuid.UUID
	if err := pool.QueryRow(t.Context(),
		`SELECT tenant_id FROM core.contacts`).Scan(&contactTenant); err != nil {
		t.Fatalf("loading the contact tenant: %v", err)
	}
	if contactTenant != acme {
		t.Errorf("contact tenant = %s, want the number's tenant", contactTenant)
	}
}

func TestWebhookEventsDropAnUnknownNumber(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, nil)
	body := numberedEventBody("5559999", "wamid.stray")

	recorder := postEvent(t, p.Routes(), sign("app-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d so Meta stops retrying", recorder.Code, http.StatusOK)
	}
	if got := countRows(t, pool, "plugin_whatsapp.conversations"); got != 0 {
		t.Errorf("conversations = %d, want 0 for an unknown number", got)
	}
	if got := countRows(t, pool, "core.contacts"); got != 0 {
		t.Errorf("contacts = %d, want 0 for an unknown number", got)
	}
}

func TestWebhookEventsForTheEnvNumberLandInTheDefaultTenant(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, map[string]string{"ALPHONE_WHATSAPP_PHONE_NUMBER_ID": "5550009"})
	body := numberedEventBody("5550009", "wamid.env")

	recorder := postEvent(t, p.Routes(), sign("app-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if held := conversationTenant(t, pool); held != sdk.DefaultTenantID {
		t.Errorf("conversation tenant = %s, want the default tenant", held)
	}
}

// unnumberedEventBody returns an inbound message payload naming no number at all.
func unnumberedEventBody(wamid string) []byte {
	return fmt.Appendf(nil, `{
		"object": "whatsapp_business_account",
		"entry": [{"id": "0", "changes": [{"field": "messages", "value": {
			"messaging_product": "whatsapp",
			"contacts": [{"wa_id": "184467235", "profile": {"name": "Maria Perez"}}],
			"messages": [{"from": "184467235", "id": %q, "timestamp": "1751791000", "type": "text",
				"text": {"body": "hello"}}]
		}}]}]
	}`, wamid)
}

func TestWebhookEventsDropAnUnnumberedArrivalWhenNoNumberIsConfigured(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, nil)
	body := unnumberedEventBody("wamid.unattributable")

	recorder := postEvent(t, p.Routes(), sign("app-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d so Meta stops retrying", recorder.Code, http.StatusOK)
	}
	if got := countRows(t, pool, "plugin_whatsapp.conversations"); got != 0 {
		t.Errorf("conversations = %d, want 0 when no number can own the arrival", got)
	}
	if got := countRows(t, pool, "core.contacts"); got != 0 {
		t.Errorf("contacts = %d, want 0 when no number can own the arrival", got)
	}
}

func TestWebhookEventsKeepAnUnnumberedArrivalForTheConfiguredNumber(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, map[string]string{"ALPHONE_WHATSAPP_PHONE_NUMBER_ID": "5550009"})
	body := unnumberedEventBody("wamid.configured")

	recorder := postEvent(t, p.Routes(), sign("app-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if held := conversationTenant(t, pool); held != sdk.DefaultTenantID {
		t.Errorf("conversation tenant = %s, want the default tenant", held)
	}
}

func TestWebhookEventsReportRoutingFailure(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		body []byte
	}{
		"a message": {body: numberedEventBody("5550001", "wamid.down")},
		"a status":  {body: numberedStatusBody("5550001", "wamid.down", "delivered")},
	}
	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			p, _ := newRoutingPlugin(t, nil)
			if err := p.Stop(t.Context()); err != nil {
				t.Fatalf("Stop() error = %v, want nil", err)
			}

			recorder := postEvent(t, p.Routes(), sign("app-secret", tc.body), tc.body)

			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d so Meta retries", recorder.Code, http.StatusInternalServerError)
			}
		})
	}
}

func TestWebhookStatusesRouteByTheirNumber(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, nil)
	acme := seedTenant(t, pool)
	if err := p.SetCredentials(sdk.WithTenant(t.Context(), acme), "5550001", "EAAG-acme-token"); err != nil {
		t.Fatalf("SetCredentials() error = %v, want nil", err)
	}
	seedOutboundRow(t, pool, "wamid.out.routed", acme)

	stray := numberedStatusBody("5559999", "wamid.out.routed", "delivered")
	if recorder := postEvent(t, p.Routes(), sign("app-secret", stray), stray); recorder.Code != http.StatusOK {
		t.Fatalf("stray status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if status := messageStatus(t, pool, "wamid.out.routed"); status != nil {
		t.Fatalf("message status after a stray number = %v, want untouched", *status)
	}

	routed := numberedStatusBody("5550001", "wamid.out.routed", "delivered")
	if recorder := postEvent(t, p.Routes(), sign("app-secret", routed), routed); recorder.Code != http.StatusOK {
		t.Fatalf("routed status = %d, want %d", recorder.Code, http.StatusOK)
	}
	status := messageStatus(t, pool, "wamid.out.routed")
	if status == nil || *status != "delivered" {
		t.Fatalf("message status = %v, want delivered inside the tenant", status)
	}
}
