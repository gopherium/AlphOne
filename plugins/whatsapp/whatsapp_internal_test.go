// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/gopherium/alphone/sdk"
)

var errEntropy = errors.New("entropy source failed")

type failingEntropy struct{}

func (failingEntropy) Read([]byte) (int, error) {
	return 0, errEntropy
}

type staticResolver struct {
	owner sdk.Contact
}

func (s staticResolver) Resolve(_ context.Context, _ sdk.Channel, _, _ string) (sdk.Contact, error) {
	return s.owner, nil
}

func newUnreachablePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), "postgres://postgres:x@localhost:9/postgres?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatalf("building pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
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

	s := &store{pool: newUnreachablePool(t)}

	err := s.insertMessage(t.Context(), uuid.Must(uuid.NewV7()), inboundMessage{externalID: "wamid.1"})

	if err == nil {
		t.Fatal("insertMessage() error = nil, want a connection failure")
	}
}

func TestStoreReportsIDGenerationFailure(t *testing.T) {
	t.Run("conversation id", func(t *testing.T) {
		uuid.SetRand(failingEntropy{})
		defer uuid.SetRand(nil)

		s := &store{}

		_, err := s.upsertConversation(t.Context(), uuid.Nil, "184467235", time.Now())

		if !errors.Is(err, errEntropy) {
			t.Fatalf("upsertConversation() error = %v, want the entropy failure in its chain", err)
		}
	})

	t.Run("message id", func(t *testing.T) {
		uuid.SetRand(failingEntropy{})
		defer uuid.SetRand(nil)

		s := &store{}

		err := s.insertMessage(t.Context(), uuid.Nil, inboundMessage{})

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
