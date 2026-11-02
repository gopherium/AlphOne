// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/sdk"
)

// manyNamedNumbers is how many distinct numbers a body names to prove one query reads them all.
const manyNamedNumbers = 200

// itemsInAForgedBody is how many bare items a body carries to prove an unproven delivery reads none of them.
const itemsInAForgedBody = 10000

// queryCounter counts the statements that reach the credentials table.
type queryCounter struct {
	reads atomic.Int64
}

// TraceQueryStart counts one statement that names the credentials table.
func (c *queryCounter) TraceQueryStart(
	ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData,
) context.Context {
	if strings.Contains(data.SQL, "plugin_whatsapp.credentials") {
		c.reads.Add(1)
	}
	return ctx
}

// TraceQueryEnd does nothing.
func (*queryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// countingCredentialReads points the plugin's store at a pool that counts what reaches the credentials table.
func countingCredentialReads(t *testing.T, p *Plugin) *queryCounter {
	t.Helper()
	counter := &queryCounter{}
	config := p.pool.Config()
	config.ConnConfig.Tracer = counter
	traced, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatalf("connecting the traced pool: %v", err)
	}
	t.Cleanup(traced.Close)
	p.store = &store{pool: traced}
	return counter
}

// bodyNaming returns a delivery with one empty change for each of count distinct numbers.
func bodyNaming(count int) []byte {
	changes := make([]string, count)
	for i := range changes {
		changes[i] = fmt.Sprintf(`{"value":{"metadata":{"phone_number_id":"555%04d"}}}`, i)
	}
	return []byte(`{"entry":[{"changes":[` + strings.Join(changes, ",") + `]}]}`)
}

// bareItems returns a delivery on no number carrying count items that are not messages.
func bareItems(count int) []byte {
	return []byte(`{"entry":[{"changes":[{"value":{"messages":[1` + strings.Repeat(",1", count-1) + `]}}]}]}`)
}

// signed returns the signature header of body under the secret.
func signed(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// deliveryStatus posts the body to the webhook signed with the secret and answers the status.
func deliveryStatus(p *Plugin, secret string, body []byte) int {
	request := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	request.Header.Set("X-Hub-Signature-256", signed(secret, body))
	recorder := httptest.NewRecorder()
	p.Routes().ServeHTTP(recorder, request)
	return recorder.Code
}

// storeNamedNumbers stores the numbers bodyNaming names, the first owning holding the app secret own-secret.
func storeNamedNumbers(t *testing.T, p *Plugin, owning, count int) {
	t.Helper()
	for i := range count {
		stored := sdk.ChannelCredentials{Identifier: fmt.Sprintf("555%04d", i), Secret: "EAAG-token"}
		if i < owning {
			stored.AppSecret = "own-secret"
		}
		if err := p.SetChannelCredentials(t.Context(), stored); err != nil {
			t.Fatalf("SetChannelCredentials() error = %v, want nil", err)
		}
	}
}

func TestTheNumbersCapCountsTheOwnSecretsADeliveryNames(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		owning   int
		bare     int
		signer   string
		want     int
		decrypts int
	}{
		"at the cap":                     {owning: 2, signer: "own-secret", want: http.StatusOK, decrypts: 2},
		"one over the cap":               {owning: 3, signer: "own-secret", want: http.StatusForbidden},
		"over the cap signed by setting": {owning: 3, signer: "app-secret", want: http.StatusForbidden},
		"rows holding no secret of their own": {
			owning: 2, bare: 3, signer: "own-secret", want: http.StatusOK, decrypts: 2,
		},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			encrypter := &boundEncrypter{}
			p := newMigratedPlugin(t)
			p.UseEncrypter(encrypter)
			p.appSecret = "app-secret"
			p.deliveryNumbersMax = 2
			storeNamedNumbers(t, p, test.owning, test.owning+test.bare)
			encrypter.calls = nil

			status := deliveryStatus(p, test.signer, bodyNaming(test.owning+test.bare))

			if status != test.want {
				t.Errorf("status = %d, want %d", status, test.want)
			}
			if got := len(encrypter.calls); got != test.decrypts {
				t.Errorf("the encrypter answered %q, want %d decrypts", encrypter.calls, test.decrypts)
			}
		})
	}
}

func TestAnUnprovenBodyLeavesItsItemsUnread(t *testing.T) {
	p := &Plugin{store: &store{}}
	body := bareItems(itemsInAForgedBody)
	var status int

	allocations := testing.AllocsPerRun(1, func() {
		status = p.receive(t.Context(), make([]byte, sha256.Size), body)
	})

	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", status, http.StatusForbidden)
	}
	if allocations >= itemsInAForgedBody {
		t.Errorf("an unproven body made %.0f allocations, want fewer than its %d items", allocations, itemsInAForgedBody)
	}
}

func TestADeliveryReadsTheNumbersItNamesInOneQueryAtMost(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		appSecret string
		signer    string
		body      []byte
		want      int
		reads     int64
	}{
		"a body naming many numbers reads them in one query": {
			signer: "a forged secret", body: bodyNaming(manyNamedNumbers), want: http.StatusForbidden, reads: 1,
		},
		"a body naming no number never reads the credentials": {
			appSecret: "app-secret", signer: "app-secret", body: []byte(`{"entry":[]}`), want: http.StatusOK,
		},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			p := newEncryptingPlugin(t)
			p.appSecret = test.appSecret
			counter := countingCredentialReads(t, p)

			status := deliveryStatus(p, test.signer, test.body)

			if status != test.want {
				t.Fatalf("status = %d, want %d", status, test.want)
			}
			if reads := counter.reads.Load(); reads != test.reads {
				t.Errorf("the delivery read the credentials %d times, want %d", reads, test.reads)
			}
		})
	}
}
