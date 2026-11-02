// SPDX-License-Identifier: Elastic-2.0

package whatsapp_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gopherium/alphone/sdk"
)

// handshakeStatus answers the status of Meta's verification handshake with the token.
func handshakeStatus(routes http.Handler, token string) int {
	target := "/webhook?hub.mode=subscribe&hub.verify_token=" + token + "&hub.challenge=42"
	recorder := httptest.NewRecorder()
	routes.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder.Code
}

func TestWebhookVerificationReadsEveryStoredVerifyToken(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, map[string]string{"ALPHONE_WHATSAPP_VERIFY_TOKEN": "env-token"})
	connectNumber(t, p, seedTenant(t, pool), sdk.ChannelCredentials{
		Identifier: "5550001", Secret: "EAAG-5550001", VerifyToken: "stored-token",
	})
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
			if status := handshakeStatus(p.Routes(), test.token); status != test.want {
				t.Errorf("status = %d, want %d", status, test.want)
			}
		})
	}
}

func TestWebhookVerificationRefusesAnEmptyTokenBesideANumberWithoutOne(t *testing.T) {
	t.Parallel()

	p, pool := newRoutingPlugin(t, nil)
	storeNumber(t, p, seedTenant(t, pool))

	if status := handshakeStatus(p.Routes(), ""); status != http.StatusForbidden {
		t.Errorf("status = %d, want %d", status, http.StatusForbidden)
	}
}

func TestWebhookVerificationReportsAStoreFailure(t *testing.T) {
	t.Parallel()

	routes := newPlugin(t, unreachableDatabaseURL, nil, nil).Routes()

	if status := handshakeStatus(routes, "guess"); status != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", status, http.StatusInternalServerError)
	}
}
