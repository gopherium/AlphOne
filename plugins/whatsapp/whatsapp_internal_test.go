// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/gopherium/alphone/sdk"
)

var errEntropy = errors.New("entropy source failed")

type failingEntropy struct{}

// Read always fails with the entropy error.
func (failingEntropy) Read([]byte) (int, error) {
	return 0, errEntropy
}

type staticResolver struct {
	owner sdk.Contact
}

// Resolve returns the same owner contact for every lookup.
func (s staticResolver) Resolve(_ context.Context, _ sdk.Channel, _, _ string) (sdk.Contact, error) {
	return s.owner, nil
}

// newUnreachablePool returns a connection pool pointed at a database that refuses every connection.
func newUnreachablePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), "postgres://postgres:x@localhost:9/postgres?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatalf("building pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestOutboundClientsCarryTheirOwnConnectionPool(t *testing.T) {
	t.Parallel()

	p, err := Register(sdk.Deps{
		DatabaseURL: "postgres://whatsapp:whatsapp@localhost:1/whatsapp",
		Env:         settings(nil),
	})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = p.Stop(t.Context()) })

	clients := map[string]*http.Client{
		"the sender":        p.sender.client,
		"the media fetcher": p.fetcher.client,
	}
	for name, client := range clients {
		if client.Transport == nil {
			t.Errorf("%s transport = nil, so it posts over the pool every other caller shares", name)
			continue
		}
		if client.Transport == http.DefaultTransport {
			t.Errorf("%s shares http.DefaultTransport, so an idle connection closed elsewhere can break a call", name)
		}
	}
}

func TestIngestReportsConversationFailure(t *testing.T) {
	t.Parallel()

	p := &Plugin{
		resolver: staticResolver{},
		store:    &store{pool: newUnreachablePool(t)},
	}

	err := p.ingest(t.Context(), inboundMessage{externalID: "wamid.1", sender: "184467235"})

	if err == nil {
		t.Fatal("ingest() error = nil, want an upsert failure")
	}
}

func TestStoreInsertMessageReportsFailure(t *testing.T) {
	t.Parallel()

	pool := newUnreachablePool(t)

	_, _, err := insertMessage(t.Context(), pool, uuid.Must(uuid.NewV7()), inboundMessage{externalID: "wamid.1"})

	if err == nil {
		t.Fatal("insertMessage() error = nil, want a connection failure")
	}
}

func TestStoreUpsertConversationReportsFailure(t *testing.T) {
	t.Parallel()

	pool := newUnreachablePool(t)

	_, err := upsertConversation(t.Context(), pool, uuid.Must(uuid.NewV7()), "184467235", time.Now().UTC())

	if err == nil {
		t.Fatal("upsertConversation() error = nil, want a connection failure")
	}
}

func TestStoreReportsIDGenerationFailure(t *testing.T) {
	t.Run("conversation id", func(t *testing.T) {
		uuid.SetRand(failingEntropy{})
		defer uuid.SetRand(nil)

		_, err := upsertConversation(t.Context(), nil, uuid.Nil, "184467235", time.Now())

		if !errors.Is(err, errEntropy) {
			t.Fatalf("upsertConversation() error = %v, want the entropy failure in its chain", err)
		}
	})

	t.Run("message id", func(t *testing.T) {
		uuid.SetRand(failingEntropy{})
		defer uuid.SetRand(nil)

		_, _, err := insertMessage(t.Context(), nil, uuid.Nil, inboundMessage{})

		if !errors.Is(err, errEntropy) {
			t.Fatalf("insertMessage() error = %v, want the entropy failure in its chain", err)
		}
	})
}

func TestStoreAppendOutboundMessageReportsFailure(t *testing.T) {
	t.Parallel()

	s := &store{pool: newUnreachablePool(t)}

	_, err := s.appendOutboundMessage(t.Context(), uuid.Must(uuid.NewV7()), outboundMessage{externalID: "wamid.out.1"})

	if err == nil {
		t.Fatal("appendOutboundMessage() error = nil, want a connection failure")
	}
}

func TestRegisterConfiguresTheMediaFetcher(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"ALPHONE_WHATSAPP_GRAPH_URL":       "http://localhost:1",
		"ALPHONE_WHATSAPP_ACCESS_TOKEN":    "tok",
		"ALPHONE_WHATSAPP_PHONE_NUMBER_ID": "PN9",
		"ALPHONE_WHATSAPP_MEDIA_MAX_BYTES": "1024",
	}
	p, err := Register(sdk.Deps{Env: settings(env)})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = p.Stop(context.Background()) })

	f := p.fetcher
	if f == nil {
		t.Fatal("fetcher = nil, want it wired by Register")
	}
	if f.baseURL != "http://localhost:1" {
		t.Errorf("baseURL = %q, want the configured value", f.baseURL)
	}
	seed := credentials{phoneNumberID: "PN9", accessToken: "tok"}
	if p.envCredentials != seed {
		t.Errorf("env credentials = %+v, want the configured seed", p.envCredentials)
	}
	if f.credentials == nil {
		t.Fatal("credentials lookup = nil, want it wired by Register")
	}
	if f.maxBytes != 1024 {
		t.Errorf("maxBytes = %d, want 1024", f.maxBytes)
	}
}

func TestRegisterAppliesTheDefaultMediaCap(t *testing.T) {
	t.Parallel()

	p, err := Register(sdk.Deps{})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = p.Stop(context.Background()) })

	if p.fetcher.maxBytes != defaultMediaMaxBytes {
		t.Errorf("maxBytes = %d, want the default %d", p.fetcher.maxBytes, int64(defaultMediaMaxBytes))
	}
}

func TestRegisterRejectsAMalformedMediaCap(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"not a number": "abc",
		"negative":     "-5",
		"zero":         "0",
	}

	for testName, value := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			env := map[string]string{"ALPHONE_WHATSAPP_MEDIA_MAX_BYTES": value}

			_, err := Register(sdk.Deps{Env: settings(env)})

			if err == nil || !strings.HasPrefix(err.Error(), "ALPHONE_WHATSAPP_MEDIA_MAX_BYTES: must ") {
				t.Errorf("Register() over %q error = %v, want the media cap refused by name", value, err)
			}
		})
	}
}

// settings returns a reader under the program prefix answering only the given variables.
func settings(held map[string]string) sdk.Env {
	return sdk.Env{Prefix: "ALPHONE_", Getenv: func(name string) string { return held[name] }}
}

// registeredSettings is what the plugin holds of the settings it read.
type registeredSettings struct {
	verifyToken    string
	appSecret      string
	key            []byte
	envCredentials credentials
	sendURL        string
	fetchURL       string
	maxBytes       int64
}

// settingsOf registers the plugin over the given variables and returns what it holds of its settings.
func settingsOf(t *testing.T, held map[string]string) registeredSettings {
	t.Helper()
	p, err := Register(sdk.Deps{Env: settings(held)})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = p.Stop(context.Background()) })
	return registeredSettings{
		verifyToken:    p.verifyToken,
		appSecret:      p.appSecret,
		key:            p.key,
		envCredentials: p.envCredentials,
		sendURL:        p.sender.baseURL,
		fetchURL:       p.fetcher.baseURL,
		maxBytes:       p.fetcher.maxBytes,
	}
}

func TestPaddedSettingsRegisterLikePlainOnes(t *testing.T) {
	t.Parallel()

	plain := map[string]string{
		"ALPHONE_WHATSAPP_VERIFY_TOKEN":    "verify-secret",
		"ALPHONE_WHATSAPP_APP_SECRET":      "app-secret",
		"ALPHONE_WHATSAPP_ACCESS_TOKEN":    "EAAG-token",
		"ALPHONE_WHATSAPP_PHONE_NUMBER_ID": "555000111",
		"ALPHONE_WHATSAPP_CREDENTIALS_KEY": strings.Repeat("ab", 32),
		"ALPHONE_WHATSAPP_GRAPH_URL":       "http://localhost:1",
		"ALPHONE_WHATSAPP_MEDIA_MAX_BYTES": "1024",
	}
	padded := map[string]string{}
	for key, value := range plain {
		padded[key] = "  " + value + "  "
	}
	want := registeredSettings{
		verifyToken:    "verify-secret",
		appSecret:      "app-secret",
		key:            bytes.Repeat([]byte{0xab}, 32),
		envCredentials: credentials{phoneNumberID: "555000111", accessToken: "EAAG-token"},
		sendURL:        "http://localhost:1",
		fetchURL:       "http://localhost:1",
		maxBytes:       1024,
	}

	for name, held := range map[string]map[string]string{"plain": plain, "padded": padded} {
		if got := settingsOf(t, held); !reflect.DeepEqual(got, want) {
			t.Errorf("Register() over %s settings holds %+v, want %+v read through the settings reader", name, got, want)
		}
	}
}

func TestRegisterNamesAMalformedCredentialsKeyOnce(t *testing.T) {
	t.Parallel()

	tests := map[string]struct{ raw, want string }{
		"a short key": {"abcd", "ALPHONE_WHATSAPP_CREDENTIALS_KEY: must hold 32 bytes, got 2"},
		"not hex": {
			strings.Repeat("zz", 32),
			"ALPHONE_WHATSAPP_CREDENTIALS_KEY: must be hex encoded: encoding/hex: invalid byte: U+007A 'z'",
		},
	}
	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			_, err := Register(sdk.Deps{Env: settings(map[string]string{"ALPHONE_WHATSAPP_CREDENTIALS_KEY": tc.raw})})

			if err == nil || err.Error() != tc.want {
				t.Errorf("Register() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestMigrateRequiresVersionTable(t *testing.T) {
	t.Parallel()

	if err := migrate(t.Context(), nil, ""); err == nil {
		t.Fatal("migrate() error = nil, want a store error")
	}
}

func TestMigrateRequiresDatabase(t *testing.T) {
	t.Parallel()

	if err := migrate(t.Context(), nil, "plugin_whatsapp.goose_db_version"); err == nil {
		t.Fatal("migrate(nil) error = nil, want a provider error")
	}
}

func TestMigrateReportsUnreachableDatabase(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("pgx", "postgres://postgres:alphone@localhost:9/postgres?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := migrate(t.Context(), db, "goose_db_version"); err == nil {
		t.Fatal("migrate() error = nil, want a connection error")
	}
}

func TestMustSubRejectsInvalidDir(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Fatal("mustSub(..) did not panic, want a panic")
		}
	}()

	mustSub(migrations, "..")
}
