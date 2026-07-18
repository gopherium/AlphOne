// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
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

func newIngestReadyPlugin(t *testing.T) *Plugin {
	t.Helper()
	p := newMigratedPlugin(t)
	owner, err := contact.New("María Pérez")
	if err != nil {
		t.Fatalf("contact.New() error = %v, want nil", err)
	}
	if err := postgres.NewContactStore(p.pool).Create(t.Context(), owner); err != nil {
		t.Fatalf("creating contact: %v", err)
	}
	p.resolver = staticResolver{owner: sdk.Contact{ID: owner.ID, Name: owner.Name}}
	return p
}

func imageMessage(wamid string) inboundMessage {
	return inboundMessage{
		externalID:  wamid,
		sender:      "184467235",
		senderName:  "María Pérez",
		content:     "la factura",
		contentType: "image",
		media:       &mediaDescriptor{mediaID: "MEDIA1", mimeType: "image/jpeg", sha256: "c2hh"},
		sentAt:      time.Now().UTC(),
		raw:         json.RawMessage(`{}`),
	}
}

func tableCount(t *testing.T, p *Plugin, table string) int {
	t.Helper()
	var count int
	if err := p.pool.QueryRow(t.Context(), "SELECT count(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("counting %s: %v", table, err)
	}
	return count
}

func TestIngestSkipsTheBroadcastWhenTheInsertFails(t *testing.T) {
	t.Parallel()

	p := newIngestReadyPlugin(t)
	subscription := p.events.subscribe()
	defer p.events.unsubscribe(subscription)

	err := p.ingest(t.Context(), inboundMessage{
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

func TestIngestReportsConversationIDFailure(t *testing.T) {
	p := newIngestReadyPlugin(t)
	uuid.SetRand(failingEntropy{})
	defer uuid.SetRand(nil)

	err := p.ingest(t.Context(), imageMessage("wamid.entropy"))

	if !errors.Is(err, errEntropy) {
		t.Fatalf("ingest() error = %v, want the entropy failure in its chain", err)
	}
}

func TestIngestStoresMediaMessagesWithPendingDownload(t *testing.T) {
	t.Parallel()

	p := newIngestReadyPlugin(t)
	ctx := t.Context()
	subscription := p.events.subscribe()
	defer p.events.unsubscribe(subscription)

	if err := p.ingest(ctx, imageMessage("wamid.m1")); err != nil {
		t.Fatalf("ingest() error = %v, want nil", err)
	}

	select {
	case <-subscription:
	default:
		t.Error("no event broadcast for a new media message")
	}
	pending, err := p.store.claimDueMedia(ctx, time.Now().UTC(), 10)
	if err != nil {
		t.Fatalf("claimDueMedia() error = %v, want nil", err)
	}
	if got, want := len(pending), 1; got != want {
		t.Fatalf("len(pending) = %d, want %d", got, want)
	}
	var messageID uuid.UUID
	err = p.pool.QueryRow(ctx,
		`SELECT id FROM plugin_whatsapp.messages WHERE external_id = 'wamid.m1'`,
	).Scan(&messageID)
	if err != nil {
		t.Fatalf("loading message id: %v", err)
	}
	if pending[0].MessageID != messageID {
		t.Errorf("pending MessageID = %s, want %s", pending[0].MessageID, messageID)
	}
	if pending[0].MediaID != "MEDIA1" {
		t.Errorf("pending MediaID = %q, want %q", pending[0].MediaID, "MEDIA1")
	}
}

func TestIngestSkipsMediaOnRedelivery(t *testing.T) {
	t.Parallel()

	p := newIngestReadyPlugin(t)
	ctx := t.Context()
	if err := p.ingest(ctx, imageMessage("wamid.dup")); err != nil {
		t.Fatalf("first ingest() error = %v, want nil", err)
	}
	subscription := p.events.subscribe()
	defer p.events.unsubscribe(subscription)

	if err := p.ingest(ctx, imageMessage("wamid.dup")); err != nil {
		t.Fatalf("redelivered ingest() error = %v, want nil", err)
	}

	select {
	case <-subscription:
		t.Error("redelivery broadcast an event, want none")
	default:
	}
	if got := tableCount(t, p, "plugin_whatsapp.messages"); got != 1 {
		t.Errorf("messages after redelivery = %d, want 1", got)
	}
	if got := tableCount(t, p, "plugin_whatsapp.media"); got != 1 {
		t.Errorf("media rows after redelivery = %d, want 1", got)
	}
}

func TestIngestStoresTextWithoutMediaRow(t *testing.T) {
	t.Parallel()

	p := newIngestReadyPlugin(t)
	message := imageMessage("wamid.text")
	message.contentType = "text"
	message.content = "hola"
	message.media = nil

	if err := p.ingest(t.Context(), message); err != nil {
		t.Fatalf("ingest() error = %v, want nil", err)
	}

	if got := tableCount(t, p, "plugin_whatsapp.media"); got != 0 {
		t.Errorf("media rows for a text message = %d, want 0", got)
	}
}

func TestIngestRollsBackTheMessageWhenMediaCannotBeStored(t *testing.T) {
	t.Parallel()

	p := newIngestReadyPlugin(t)
	ctx := t.Context()
	if _, err := p.pool.Exec(ctx,
		`ALTER TABLE plugin_whatsapp.media ADD CONSTRAINT media_test_reject CHECK (false)`,
	); err != nil {
		t.Fatalf("adding rejection constraint: %v", err)
	}
	subscription := p.events.subscribe()
	defer p.events.unsubscribe(subscription)

	err := p.ingest(ctx, imageMessage("wamid.atomic"))

	if err == nil {
		t.Fatal("ingest() error = nil, want a media insert failure")
	}
	if got := tableCount(t, p, "plugin_whatsapp.messages"); got != 0 {
		t.Errorf("messages after the failed media insert = %d, want 0 rolled back", got)
	}
	select {
	case <-subscription:
		t.Error("ingest() broadcast an event despite the rollback")
	default:
	}
}
