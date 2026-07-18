// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/peterldowns/pgtestdb"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/testdb"
	"github.com/gopherium/alphone/sdk"
)

func newMigratedPlugin(t *testing.T) *Plugin {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	cfg := pgtestdb.Custom(t, testdb.Config(), testdb.CoreMigrator())
	pool, err := pgxpool.New(t.Context(), cfg.URL())
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)
	p := &Plugin{pool: pool, store: &store{pool: pool}, events: newBroadcaster()}
	if err := p.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	return p
}

func TestIngestSkipsTheBroadcastWhenTheInsertFails(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	owner, err := contact.New("María Pérez")
	if err != nil {
		t.Fatalf("contact.New() error = %v, want nil", err)
	}
	if err := postgres.NewContactStore(p.pool).Create(t.Context(), owner); err != nil {
		t.Fatalf("creating contact: %v", err)
	}
	p.resolver = staticResolver{owner: sdk.Contact{ID: owner.ID, Name: owner.Name}}
	subscription := p.events.subscribe()
	defer p.events.unsubscribe(subscription)

	err = p.ingest(t.Context(), inboundMessage{
		externalID: "wamid.broken-raw",
		sender:     "184467235",
		senderName: "María Pérez",
		sentAt:     time.Now().UTC(),
		raw:        json.RawMessage("{"),
	})

	if err == nil {
		t.Fatal("ingest() error = nil, want an insert failure")
	}
	select {
	case <-subscription:
		t.Fatal("ingest() broadcast an event despite the failed insert")
	default:
	}
}
