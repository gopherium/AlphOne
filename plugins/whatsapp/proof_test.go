// SPDX-License-Identifier: Elastic-2.0

package whatsapp_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/plugins/whatsapp"
	"github.com/gopherium/alphone/sdk"
)

// connectNumber stores a number under the tenant with its own app secret.
func connectNumber(t *testing.T, p *whatsapp.Plugin, tenantID uuid.UUID, number, appSecret string) {
	t.Helper()
	stored := sdk.ChannelCredentials{Identifier: number, Secret: "EAAG-" + number, AppSecret: appSecret}
	if err := p.SetChannelCredentials(sdk.WithTenant(t.Context(), tenantID), stored); err != nil {
		t.Fatalf("SetChannelCredentials() error = %v, want nil", err)
	}
}

// seedGlobex stores a second tenant beside the one seedTenant stores and returns its id.
func seedGlobex(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(), `INSERT INTO core.tenants (id, name) VALUES ($1, 'Globex')`, id); err != nil {
		t.Fatalf("seeding the tenant: %v", err)
	}
	return id
}

// messageChange returns one change carrying a text message that arrived on number.
func messageChange(number, wamid string) string {
	return fmt.Sprintf(`{"field":"messages","value":{"metadata":{"phone_number_id":%q},`+
		`"contacts":[{"wa_id":"184467235","profile":{"name":"Maria Perez"}}],`+
		`"messages":[{"from":"184467235","id":%q,"timestamp":"1751791000","type":"text","text":{"body":"hello"}}]}}`,
		number, wamid)
}

// statusChange returns one change carrying a delivery status that arrived on number.
func statusChange(number, wamid string) string {
	return fmt.Sprintf(`{"field":"messages","value":{"metadata":{"phone_number_id":%q},`+
		`"statuses":[{"id":%q,"status":"delivered","timestamp":"1751791000","recipient_id":"184467235"}]}}`,
		number, wamid)
}

// deliveryOf returns one delivery body carrying the changes.
func deliveryOf(changes ...string) []byte {
	return []byte(`{"object":"whatsapp_business_account","entry":[{"id":"0","changes":[` +
		strings.Join(changes, ",") + `]}]}`)
}

// heldConversations counts the conversations of a tenant.
func heldConversations(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID) int {
	t.Helper()
	var held int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM plugin_whatsapp.conversations WHERE tenant_id = $1`, tenantID).Scan(&held); err != nil {
		t.Fatalf("counting the conversations: %v", err)
	}
	return held
}

// heldContacts counts the contacts of a tenant.
func heldContacts(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID) int {
	t.Helper()
	var held int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM core.contacts WHERE tenant_id = $1`, tenantID).Scan(&held); err != nil {
		t.Fatalf("counting the contacts: %v", err)
	}
	return held
}

// onlyConversationNumber returns the number the single stored conversation came in on, nil for none.
func onlyConversationNumber(t *testing.T, pool *pgxpool.Pool) *string {
	t.Helper()
	var number *string
	if err := pool.QueryRow(t.Context(),
		`SELECT phone_number_id FROM plugin_whatsapp.conversations`).Scan(&number); err != nil {
		t.Fatalf("reading the number of the conversation: %v", err)
	}
	return number
}

func TestWebhookEventsTurnDownAShapelessSignatureBeforeParsing(t *testing.T) {
	t.Parallel()

	body := []byte(`{"entry":`)
	valid := sign("app-secret", body)
	tests := map[string]string{
		"no prefix":          strings.TrimPrefix(valid, "sha256="),
		"another hash":       "sha1=" + strings.TrimPrefix(valid, "sha256="),
		"a short digest":     valid[:len(valid)-2],
		"a long digest":      valid + "00",
		"not hexadecimal":    "sha256=" + strings.Repeat("z", 64),
		"nothing but prefix": "sha256=",
	}
	for testName, signature := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			routes := newPlugin(t, unreachableDatabaseURL, nil, map[string]string{
				"ALPHONE_WHATSAPP_APP_SECRET": "app-secret",
			}).Routes()

			recorder := postEvent(t, routes, signature, body)

			if recorder.Code != http.StatusForbidden {
				t.Errorf("status = %d, want %d before the body is parsed", recorder.Code, http.StatusForbidden)
			}
		})
	}
}

func TestTheEnvironmentSecretJudgesABodyThatDoesNotParse(t *testing.T) {
	t.Parallel()

	body := []byte(`{"entry":`)
	tests := map[string]struct {
		configured string
		signature  string
		want       int
	}{
		"signed with the setting": {
			configured: "app-secret", signature: sign("app-secret", body), want: http.StatusBadRequest,
		},
		"signed with another secret": {
			configured: "app-secret", signature: sign("other-secret", body), want: http.StatusForbidden,
		},
		"no setting, an empty secret": {configured: "", signature: sign("", body), want: http.StatusForbidden},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			routes := newPlugin(t, unreachableDatabaseURL, nil, map[string]string{
				"ALPHONE_WHATSAPP_APP_SECRET": test.configured,
			}).Routes()

			recorder := postEvent(t, routes, test.signature, body)

			if recorder.Code != test.want {
				t.Errorf("status = %d, want %d", recorder.Code, test.want)
			}
		})
	}
}

func TestANumberIsProvenByItsOwnSecret(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, nil)
	acme := seedTenant(t, pool)
	connectNumber(t, p, acme, "5550001", "acme-secret")
	body := numberedEventBody("5550001", "wamid.own")

	recorder := postEvent(t, p.Routes(), sign("acme-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if held := conversationTenant(t, pool); held != acme {
		t.Errorf("conversation tenant = %s, want the number's tenant", held)
	}
	if number := onlyConversationNumber(t, pool); number == nil || *number != "5550001" {
		t.Errorf("the conversation came in on %v, want 5550001", number)
	}
}

func TestANumberWithoutASecretFallsBackToTheEnvironmentSecret(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, nil)
	acme := seedTenant(t, pool)
	storeNumber(t, p, acme)
	body := numberedEventBody("5550001", "wamid.fallback")

	recorder := postEvent(t, p.Routes(), sign("app-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if number := onlyConversationNumber(t, pool); number == nil || *number != "5550001" {
		t.Errorf("the conversation came in on %v, want 5550001", number)
	}
}

func TestTheEnvironmentSecretDoesNotProveANumberWithItsOwnSecret(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, nil)
	acme := seedTenant(t, pool)
	connectNumber(t, p, acme, "5550001", "acme-secret")
	body := numberedEventBody("5550001", "wamid.env")

	recorder := postEvent(t, p.Routes(), sign("app-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if held := heldConversations(t, pool, acme); held != 0 {
		t.Errorf("conversations = %d, want the item dropped", held)
	}
}

func TestAnotherTenantsSecretWritesNothing(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, nil)
	acme, globex := seedTenant(t, pool), seedGlobex(t, pool)
	connectNumber(t, p, acme, "5550001", "acme-secret")
	connectNumber(t, p, globex, "5550002", "globex-secret")
	body := numberedEventBody("5550002", "wamid.forged")

	recorder := postEvent(t, p.Routes(), sign("acme-secret", body), body)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d when no secret proves the body", recorder.Code, http.StatusForbidden)
	}
	if held := heldConversations(t, pool, globex); held != 0 {
		t.Errorf("conversations in the other tenant = %d, want none", held)
	}
	if held := heldContacts(t, pool, globex); held != 0 {
		t.Errorf("contacts in the other tenant = %d, want none", held)
	}
}

func TestAMixedDeliveryStoresOnlyTheProvenItems(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, nil)
	acme, globex := seedTenant(t, pool), seedGlobex(t, pool)
	connectNumber(t, p, acme, "5550001", "acme-secret")
	connectNumber(t, p, globex, "5550002", "globex-secret")
	body := deliveryOf(messageChange("5550002", "wamid.forged"), messageChange("5550001", "wamid.own"))

	recorder := postEvent(t, p.Routes(), sign("acme-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d once a secret proved the body", recorder.Code, http.StatusOK)
	}
	if held := heldConversations(t, pool, acme); held != 1 {
		t.Errorf("conversations of the proven number = %d, want 1", held)
	}
	if held := heldConversations(t, pool, globex); held != 0 {
		t.Errorf("conversations of the unproven number = %d, want none", held)
	}
	if held := heldContacts(t, pool, globex); held != 0 {
		t.Errorf("contacts of the unproven number = %d, want none", held)
	}
}

func TestAStatusOnAnotherTenantsNumberMovesNothing(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, nil)
	acme, globex := seedTenant(t, pool), seedGlobex(t, pool)
	connectNumber(t, p, acme, "5550001", "acme-secret")
	connectNumber(t, p, globex, "5550002", "globex-secret")
	seedOutboundRow(t, pool, "wamid.out.theirs", globex)
	seedOutboundRow(t, pool, "wamid.out.mine", acme)
	body := deliveryOf(statusChange("5550002", "wamid.out.theirs"), statusChange("5550001", "wamid.out.mine"))

	recorder := postEvent(t, p.Routes(), sign("acme-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if status := messageStatus(t, pool, "wamid.out.theirs"); status != nil {
		t.Errorf("the other tenant's message status = %q, want it untouched", *status)
	}
	if status := messageStatus(t, pool, "wamid.out.mine"); status == nil || *status != "delivered" {
		t.Errorf("the proven message status = %v, want delivered", status)
	}
}

func TestASecretCopiedIntoAnotherTenantProvesNothing(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, nil)
	acme, globex := seedTenant(t, pool), seedGlobex(t, pool)
	connectNumber(t, p, acme, "5550001", "acme-secret")
	connectNumber(t, p, globex, "5550002", "globex-secret")
	if _, err := pool.Exec(t.Context(),
		`UPDATE plugin_whatsapp.credentials SET app_secret =
			(SELECT app_secret FROM plugin_whatsapp.credentials WHERE phone_number_id = '5550001')
		WHERE phone_number_id = '5550002'`); err != nil {
		t.Fatalf("copying the secret: %v", err)
	}
	body := numberedEventBody("5550002", "wamid.copied")

	recorder := postEvent(t, p.Routes(), sign("acme-secret", body), body)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if held := heldConversations(t, pool, globex); held != 0 {
		t.Errorf("conversations = %d, want a copied secret to open nothing", held)
	}
}

func TestAnEnvironmentNumberConversationNamesNoNumber(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, map[string]string{"ALPHONE_WHATSAPP_PHONE_NUMBER_ID": "5550009"})
	body := numberedEventBody("5550009", "wamid.env")

	recorder := postEvent(t, p.Routes(), sign("app-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if number := onlyConversationNumber(t, pool); number != nil {
		t.Errorf("the conversation came in on %q, want no number", *number)
	}
}

func TestAnEnvironmentNumberTheDefaultTenantStoredAnswersItsStoredSecret(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, map[string]string{"ALPHONE_WHATSAPP_PHONE_NUMBER_ID": "5550009"})
	connectNumber(t, p, sdk.DefaultTenantID, "5550009", "stored-secret")
	unproven := numberedEventBody("5550009", "wamid.env")

	recorder := postEvent(t, p.Routes(), sign("app-secret", unproven), unproven)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := countRows(t, pool, "plugin_whatsapp.conversations"); got != 0 {
		t.Fatalf("conversations = %d, want the environment secret to prove nothing here", got)
	}

	proven := numberedEventBody("5550009", "wamid.stored")
	if recorder := postEvent(t, p.Routes(), sign("stored-secret", proven), proven); recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if held := conversationTenant(t, pool); held != sdk.DefaultTenantID {
		t.Errorf("conversation tenant = %s, want the default tenant", held)
	}
	if number := onlyConversationNumber(t, pool); number == nil || *number != "5550009" {
		t.Errorf("the conversation came in on %v, want 5550009", number)
	}
}

func TestTheEnvironmentNumberStaysWithTheDefaultTenant(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, map[string]string{"ALPHONE_WHATSAPP_PHONE_NUMBER_ID": "5550009"})
	acme := seedTenant(t, pool)

	err := p.SetChannelCredentials(sdk.WithTenant(t.Context(), acme), sdk.ChannelCredentials{
		Identifier: "5550009", Secret: "EAAG-5550009", AppSecret: "acme-secret",
	})

	var coded sdk.GraphError
	if !errors.As(err, &coded) || coded.Reason != "channel_number_taken" {
		t.Fatalf("SetChannelCredentials() error = %v, want the number turned down as taken", err)
	}
	body := numberedEventBody("5550009", "wamid.env")
	if recorder := postEvent(t, p.Routes(), sign("app-secret", body), body); recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if held := conversationTenant(t, pool); held != sdk.DefaultTenantID {
		t.Errorf("conversation tenant = %s, want the default tenant", held)
	}
}

func TestANumbersOwnSecretOpensNoEnvironmentRoute(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, map[string]string{"ALPHONE_WHATSAPP_PHONE_NUMBER_ID": "5550009"})
	acme := seedTenant(t, pool)
	connectNumber(t, p, acme, "5550001", "acme-secret")
	body := deliveryOf(
		messageChange("5550001", "wamid.own"), messageChange("5550009", "wamid.env"), messageChange("", "wamid.none"),
	)

	recorder := postEvent(t, p.Routes(), sign("acme-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d once a secret proved the body", recorder.Code, http.StatusOK)
	}
	if held := heldConversations(t, pool, acme); held != 1 {
		t.Errorf("conversations of the proven number = %d, want 1", held)
	}
	if held := heldConversations(t, pool, sdk.DefaultTenantID); held != 0 {
		t.Errorf("conversations in the default tenant = %d, want none", held)
	}
	if held := heldContacts(t, pool, sdk.DefaultTenantID); held != 0 {
		t.Errorf("contacts in the default tenant = %d, want none", held)
	}
}

func TestANumberHoldingANulAnswersLikeAnUnknownNumber(t *testing.T) {
	t.Parallel()

	p, _ := newRoutingPlugin(t, nil)
	body := []byte(`{"entry":[{"changes":[{"value":{"metadata":{"phone_number_id":"\u0000"}}}]}]}`)

	recorder := postEvent(t, p.Routes(), sign("some-other-secret", body), body)

	if recorder.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestADeliveryAfterARemovalAnswersByTheSecretThatSignedIt(t *testing.T) {
	t.Parallel()

	withoutEnvironmentNumber := map[string]string{"ALPHONE_WHATSAPP_PHONE_NUMBER_ID": ""}
	tests := map[string]struct {
		env       map[string]string
		ownSecret string
		signer    string
		want      int
	}{
		"its own secret": {
			ownSecret: "own-secret", signer: "own-secret", want: http.StatusForbidden,
		},
		"the setting beside an environment number": {
			signer: "app-secret", want: http.StatusOK,
		},
		"the setting with no environment number": {
			env: withoutEnvironmentNumber, signer: "app-secret", want: http.StatusOK,
		},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			p, _, pool := newSendingHarness(t, test.env)
			p.UseEncrypter(whatsapp.BoundEncrypter())
			acme := seedTenant(t, pool)
			connectNumber(t, p, acme, "5550001", test.ownSecret)
			if err := p.RemoveChannelCredentials(sdk.WithTenant(t.Context(), acme), "5550001"); err != nil {
				t.Fatalf("RemoveChannelCredentials() error = %v, want nil", err)
			}
			body := numberedEventBody("5550001", "wamid.removed")

			recorder := postEvent(t, p.Routes(), sign(test.signer, body), body)

			if recorder.Code != test.want {
				t.Errorf("status = %d, want %d", recorder.Code, test.want)
			}
			if held := heldConversations(t, pool, acme) + heldConversations(t, pool, sdk.DefaultTenantID); held != 0 {
				t.Errorf("conversations = %d, want the delivery dropped", held)
			}
		})
	}
}

func TestTheEnvironmentNumberKeepsReceivingOnceItsRowIsRemoved(t *testing.T) {
	t.Parallel()

	p, _, pool := newSendingHarness(t, nil)
	p.UseEncrypter(whatsapp.BoundEncrypter())
	connectNumber(t, p, sdk.DefaultTenantID, "555000222", "own-secret")
	if err := p.RemoveChannelCredentials(t.Context(), "555000222"); err != nil {
		t.Fatalf("RemoveChannelCredentials() error = %v, want nil", err)
	}
	body := numberedEventBody("555000222", "wamid.kept")

	recorder := postEvent(t, p.Routes(), sign("app-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if held := heldConversations(t, pool, sdk.DefaultTenantID); held != 1 {
		t.Errorf("conversations = %d, want the delivery stored", held)
	}
}

func TestAProvenBodyWhoseItemsDoNotDecodeAnswersBadRequest(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, nil)
	connectNumber(t, p, seedTenant(t, pool), "5550001", "acme-secret")
	body := deliveryOf(`{"value":{"metadata":{"phone_number_id":"5550001"},"messages":"not a list"}}`)

	recorder := postEvent(t, p.Routes(), sign("acme-secret", body), body)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestADeliveryStoresNoMoreItemsThanTheSettingAllows(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, map[string]string{"ALPHONE_WHATSAPP_DELIVERY_ITEMS_MAX": "2"})
	connectNumber(t, p, seedTenant(t, pool), "5550001", "acme-secret")
	body := deliveryOf(
		messageChange("5550001", "wamid.first"), messageChange("5550001", "wamid.second"),
		messageChange("5550001", "wamid.third"),
	)

	recorder := postEvent(t, p.Routes(), sign("acme-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d so Meta stops retrying", recorder.Code, http.StatusOK)
	}
	if stored := countRows(t, pool, "plugin_whatsapp.messages"); stored != 2 {
		t.Errorf("messages = %d, want the setting's 2", stored)
	}
}

func TestStatusUpdatesCountTowardsTheItemsOfADelivery(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, map[string]string{"ALPHONE_WHATSAPP_DELIVERY_ITEMS_MAX": "2"})
	acme := seedTenant(t, pool)
	connectNumber(t, p, acme, "5550001", "acme-secret")
	seedOutboundRow(t, pool, "wamid.out.kept", acme)
	seedOutboundRow(t, pool, "wamid.out.over", acme)
	body := deliveryOf(
		messageChange("5550001", "wamid.first"), statusChange("5550001", "wamid.out.kept"),
		statusChange("5550001", "wamid.out.over"),
	)

	recorder := postEvent(t, p.Routes(), sign("acme-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if status := messageStatus(t, pool, "wamid.out.kept"); status == nil || *status != "delivered" {
		t.Errorf("the status within the setting = %v, want delivered", status)
	}
	if status := messageStatus(t, pool, "wamid.out.over"); status != nil {
		t.Errorf("the status over the setting = %q, want it left out", *status)
	}
}

func TestUnprovenItemsTakeNoRoomInADelivery(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, map[string]string{"ALPHONE_WHATSAPP_DELIVERY_ITEMS_MAX": "1"})
	acme, globex := seedTenant(t, pool), seedGlobex(t, pool)
	connectNumber(t, p, acme, "5550001", "acme-secret")
	connectNumber(t, p, globex, "5550002", "globex-secret")
	body := deliveryOf(messageChange("5550002", "wamid.forged"), messageChange("5550001", "wamid.own"))

	recorder := postEvent(t, p.Routes(), sign("acme-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if held := heldConversations(t, pool, acme); held != 1 {
		t.Errorf("conversations of the proven number = %d, want 1", held)
	}
}

func TestNoSecretAtAllAnswersForbidden(t *testing.T) {
	t.Parallel()

	address := newTestDatabase(t)
	p := newPlugin(t, address, nil, nil)
	if err := p.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	body := numberedEventBody("5559999", "wamid.nosecret")

	recorder := postEvent(t, p.Routes(), sign("", body), body)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestAnUnknownNumberProvesNothing(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, nil)
	body := numberedEventBody("5559999", "wamid.unknown")

	recorder := postEvent(t, p.Routes(), sign("some-other-secret", body), body)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if got := countRows(t, pool, "plugin_whatsapp.conversations"); got != 0 {
		t.Errorf("conversations = %d, want none", got)
	}
}

func TestAChangeWithNothingToStoreStillProvesItsNumber(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"no items":     `{"field":"messages","value":{"metadata":{"phone_number_id":"5550001"}}}`,
		"a broken one": `{"field":"messages","value":{"metadata":{"phone_number_id":"5550001"},"messages":[{"id":""}]}}`,
	}
	for testName, change := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			p, pool := newRoutingPlugin(t, nil)
			connectNumber(t, p, seedTenant(t, pool), "5550001", "acme-secret")
			body := deliveryOf(change)

			recorder := postEvent(t, p.Routes(), sign("acme-secret", body), body)

			if recorder.Code != http.StatusOK {
				t.Errorf("status = %d, want %d so Meta stops retrying", recorder.Code, http.StatusOK)
			}
		})
	}
}

func TestADeliveryLandsInTheNumbersTenantRatherThanTheAmbientOne(t *testing.T) {
	t.Parallel()

	tests := map[string][]byte{
		"an arrival naming no number":      unnumberedEventBody("wamid.none"),
		"an arrival naming the env number": numberedEventBody("5550009", "wamid.env"),
	}
	for testName, body := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			p, pool := newRoutingPlugin(t, map[string]string{"ALPHONE_WHATSAPP_PHONE_NUMBER_ID": "5550009"})
			ambient := sdk.WithTenant(t.Context(), seedTenant(t, pool))
			request := httptest.NewRequestWithContext(ambient, http.MethodPost, "/webhook", strings.NewReader(string(body)))
			request.Header.Set("X-Hub-Signature-256", sign("app-secret", body))
			recorder := httptest.NewRecorder()

			p.Routes().ServeHTTP(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
			}
			if held := conversationTenant(t, pool); held != sdk.DefaultTenantID {
				t.Errorf("conversation tenant = %s, want the default tenant rather than the ambient one", held)
			}
		})
	}
}

// failingGate fails every question about a tenant.
type failingGate struct{}

// AcceptsMachineTraffic answers a failure.
func (failingGate) AcceptsMachineTraffic(context.Context, uuid.UUID) (bool, error) {
	return false, errors.New("the gate is down")
}

func TestWebhookEventsReportAGateFailure(t *testing.T) {
	t.Parallel()

	p, _ := newRoutingPlugin(t, map[string]string{"ALPHONE_WHATSAPP_PHONE_NUMBER_ID": "5550009"})
	p.UseTenantGate(failingGate{})
	body := numberedEventBody("5550009", "wamid.gate")

	recorder := postEvent(t, p.Routes(), sign("app-secret", body), body)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d so Meta retries", recorder.Code, http.StatusInternalServerError)
	}
}

func TestWebhookVerificationReadsEveryStoredVerifyToken(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, map[string]string{"ALPHONE_WHATSAPP_VERIFY_TOKEN": "env-token"})
	acme := seedTenant(t, pool)
	stored := sdk.ChannelCredentials{Identifier: "5550001", Secret: "EAAG-5550001", VerifyToken: "stored-token"}
	if err := p.SetChannelCredentials(sdk.WithTenant(t.Context(), acme), stored); err != nil {
		t.Fatalf("SetChannelCredentials() error = %v, want nil", err)
	}
	tests := map[string]struct {
		token string
		want  int
	}{
		"the environment token": {token: "env-token", want: http.StatusOK},
		"a stored token":        {token: "stored-token", want: http.StatusOK},
		"any other token":       {token: "guess", want: http.StatusForbidden},
		"no token":              {token: "", want: http.StatusForbidden},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			target := "/webhook?hub.mode=subscribe&hub.verify_token=" + test.token + "&hub.challenge=42"
			recorder := httptest.NewRecorder()

			p.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))

			if recorder.Code != test.want {
				t.Errorf("status = %d, want %d", recorder.Code, test.want)
			}
		})
	}
}

func TestWebhookVerificationRefusesAnEmptyTokenBesideANumberWithoutOne(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, nil)
	storeNumber(t, p, seedTenant(t, pool))
	recorder := httptest.NewRecorder()

	p.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
		"/webhook?hub.mode=subscribe&hub.verify_token=&hub.challenge=42", nil))

	if recorder.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestWebhookVerificationReportsAStoreFailure(t *testing.T) {
	t.Parallel()

	routes := newPlugin(t, unreachableDatabaseURL, nil, nil).Routes()
	recorder := httptest.NewRecorder()

	routes.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
		"/webhook?hub.mode=subscribe&hub.verify_token=guess&hub.challenge=42", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}
