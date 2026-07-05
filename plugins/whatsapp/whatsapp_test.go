// SPDX-License-Identifier: AGPL-3.0-or-later

package whatsapp_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/peterldowns/pgtestdb"
	"github.com/peterldowns/pgtestdb/migrators/goosemigrator"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/plugin"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/plugins/whatsapp"
)

var (
	_ plugin.Plugin   = (*whatsapp.Plugin)(nil)
	_ plugin.Migrator = (*whatsapp.Plugin)(nil)
)

const uniqueViolation = "23505"

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	cfg := pgtestdb.Custom(t, pgtestdb.Config{
		DriverName: "pgx",
		User:       "postgres",
		Password:   "alphone",
		Host:       "localhost",
		Port:       "5433",
		Database:   "postgres",
		Options:    "sslmode=disable",
	}, goosemigrator.New("migrations", goosemigrator.WithFS(postgres.Migrations)))
	pool, err := pgxpool.New(t.Context(), cfg.URL())
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestPluginIdentityAndLifecycle(t *testing.T) {
	t.Parallel()

	p := whatsapp.New(nil)

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

	pool := newTestPool(t)
	p := whatsapp.New(pool)

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

	pool := newTestPool(t)
	pool.Close()
	p := whatsapp.New(pool)

	if err := p.Migrate(t.Context()); err == nil {
		t.Fatal("Migrate() on closed pool error = nil, want an error")
	}
}
