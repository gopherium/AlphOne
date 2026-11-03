// SPDX-License-Identifier: Elastic-2.0

package whatsapp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/plugins/whatsapp"
	"github.com/gopherium/alphone/sdk"
)

// failingGate fails every question about a tenant.
type failingGate struct{}

// AcceptsMachineTraffic answers a failure.
func (failingGate) AcceptsMachineTraffic(context.Context, uuid.UUID) (bool, error) {
	return false, errors.New("the gate is down")
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
	connectNumber(t, p, sdk.DefaultTenantID, numberWithSecret("5550009", "stored-secret"))
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
	connectNumber(t, p, acme, numberWithSecret("5550001", "acme-secret"))
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
			connectNumber(t, p, acme, numberWithSecret("5550001", test.ownSecret))
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
	connectNumber(t, p, sdk.DefaultTenantID, numberWithSecret("555000222", "own-secret"))
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
