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

func TestABodyNamingManyNumbersReadsThemInOneQuery(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	counter := countingCredentialReads(t, p)
	body := bodyNaming(manyNamedNumbers)
	request := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	request.Header.Set("X-Hub-Signature-256", signed("a forged secret", body))
	recorder := httptest.NewRecorder()

	p.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if reads := counter.reads.Load(); reads != 1 {
		t.Errorf("the delivery read the credentials %d times, want one query for every number", reads)
	}
}

func TestABodyNamingNoNumberNeverReadsTheCredentials(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	p.appSecret = "app-secret"
	counter := countingCredentialReads(t, p)
	body := []byte(`{"entry":[]}`)
	request := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	request.Header.Set("X-Hub-Signature-256", signed("app-secret", body))
	recorder := httptest.NewRecorder()

	p.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if reads := counter.reads.Load(); reads != 0 {
		t.Errorf("the delivery read the credentials %d times, want none", reads)
	}
}
