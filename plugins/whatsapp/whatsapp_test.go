// SPDX-License-Identifier: AGPL-3.0-or-later

package whatsapp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/peterldowns/pgtestdb"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/testdb"
	"github.com/gopherium/alphone/plugins/whatsapp"
	"github.com/gopherium/alphone/sdk"
)

var (
	_ sdk.Plugin        = (*whatsapp.Plugin)(nil)
	_ sdk.Migrator      = (*whatsapp.Plugin)(nil)
	_ sdk.RouteProvider = (*whatsapp.Plugin)(nil)
)

type resolverBridge struct {
	resolver *contact.Resolver
}

func (b resolverBridge) Resolve(ctx context.Context, channel sdk.Channel, identifier, displayName string) (sdk.Contact, error) {
	owner, err := b.resolver.Resolve(ctx, contact.Channel(channel), identifier, displayName)
	if err != nil {
		return sdk.Contact{}, err
	}
	return sdk.Contact{ID: owner.ID, Name: owner.Name}, nil
}

const uniqueViolation = "23505"

const unreachableDatabaseURL = "postgres://postgres:alphone@localhost:9/postgres?sslmode=disable&connect_timeout=1"

func newTestDatabase(t *testing.T) *pgtestdb.Config {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	return pgtestdb.Custom(t, testdb.Config(), testdb.CoreMigrator())
}

func newAssertionPool(t *testing.T, url string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), url)
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func newPlugin(t *testing.T, databaseURL string, resolver sdk.ContactResolver, env map[string]string) *whatsapp.Plugin {
	t.Helper()
	p, err := whatsapp.Register(sdk.Deps{
		DatabaseURL: databaseURL,
		Resolver:    resolver,
		Getenv:      func(key string) string { return env[key] },
	})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = p.Stop(context.Background()) })
	return p
}

func TestRegisterRejectsMalformedDatabaseURL(t *testing.T) {
	t.Parallel()

	p, err := whatsapp.Register(sdk.Deps{DatabaseURL: "://not-a-url"})

	if err == nil {
		t.Fatal("Register() error = nil, want a parse error")
	}
	if p != nil {
		t.Errorf("Register() plugin = %v, want nil on failure", p)
	}
}

func TestWebhookVerification(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		configuredToken string
		target          string
		wantStatus      int
		wantBody        string
	}{
		"valid handshake": {
			configuredToken: "secret",
			target:          "/webhook?hub.mode=subscribe&hub.verify_token=secret&hub.challenge=42",
			wantStatus:      http.StatusOK,
			wantBody:        "42",
		},
		"wrong token": {
			configuredToken: "secret",
			target:          "/webhook?hub.mode=subscribe&hub.verify_token=guess&hub.challenge=42",
			wantStatus:      http.StatusForbidden,
		},
		"wrong mode": {
			configuredToken: "secret",
			target:          "/webhook?hub.mode=unsubscribe&hub.verify_token=secret&hub.challenge=42",
			wantStatus:      http.StatusForbidden,
		},
		"unconfigured token never verifies": {
			configuredToken: "",
			target:          "/webhook?hub.mode=subscribe&hub.verify_token=&hub.challenge=42",
			wantStatus:      http.StatusForbidden,
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			routes := newPlugin(t, "", nil, map[string]string{
				"ALPHONE_WHATSAPP_VERIFY_TOKEN": tc.configuredToken,
			}).Routes()
			request := httptest.NewRequest(http.MethodGet, tc.target, nil)
			recorder := httptest.NewRecorder()

			routes.ServeHTTP(recorder, request)

			if recorder.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tc.wantStatus)
			}
			if tc.wantBody != "" && recorder.Body.String() != tc.wantBody {
				t.Errorf("body = %q, want %q", recorder.Body.String(), tc.wantBody)
			}
		})
	}
}

func TestPluginIdentityAndLifecycle(t *testing.T) {
	t.Parallel()

	p, err := whatsapp.Register(sdk.Deps{})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}

	if got := p.ID(); got != "whatsapp" {
		t.Errorf("ID() = %q, want %q", got, "whatsapp")
	}
	if err := p.Start(t.Context()); err != nil {
		t.Errorf("Start() error = %v, want nil", err)
	}
	if err := p.Stop(t.Context()); err != nil {
		t.Errorf("Stop() error = %v, want nil", err)
	}
}

func TestMigrateCreatesMessagingTables(t *testing.T) {
	t.Parallel()

	cfg := newTestDatabase(t)
	pool := newAssertionPool(t, cfg.URL())
	p := newPlugin(t, cfg.URL(), nil, nil)

	if err := p.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	if err := p.Migrate(t.Context()); err != nil {
		t.Fatalf("second Migrate() error = %v, want idempotent nil", err)
	}

	maria, err := contact.New("María Pérez")
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if _, err := pool.Exec(t.Context(), "INSERT INTO core.contacts (id, name, created_at) VALUES ($1, $2, $3)", maria.ID, maria.Name, maria.CreatedAt); err != nil {
		t.Fatalf("inserting contact: %v", err)
	}
	conversationID := uuid.Must(uuid.NewV7())
	now := time.Now().UTC()
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO plugin_whatsapp.conversations (id, contact_id, channel, external_id, status, last_activity_at, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7)",
		conversationID, maria.ID, "whatsapp", "184467235@lid", "open", now, now,
	); err != nil {
		t.Fatalf("inserting conversation: %v", err)
	}
	insertMessage := func(messageID uuid.UUID, externalID string) error {
		_, err := pool.Exec(t.Context(),
			"INSERT INTO plugin_whatsapp.messages (id, conversation_id, external_id, direction, content, content_type, sent_at, raw, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)",
			messageID, conversationID, externalID, "inbound", "hola", "text", now, "{}", now,
		)
		return err
	}
	if err := insertMessage(uuid.Must(uuid.NewV7()), "wamid.1"); err != nil {
		t.Fatalf("inserting message: %v", err)
	}

	err = insertMessage(uuid.Must(uuid.NewV7()), "wamid.1")

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != uniqueViolation {
		t.Fatalf("inserting duplicate message: %v, want unique violation %s", err, uniqueViolation)
	}
}

func TestMigrateReportsConnectionFailure(t *testing.T) {
	t.Parallel()

	p := newPlugin(t, unreachableDatabaseURL, nil, nil)

	if err := p.Migrate(t.Context()); err == nil {
		t.Fatal("Migrate() on unreachable database error = nil, want an error")
	}
}
