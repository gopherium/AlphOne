// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/sdk"
)

type seedResolver struct {
	resolver *contact.Resolver
}

func (r seedResolver) Resolve(
	ctx context.Context,
	channel sdk.Channel,
	identifier, displayName string,
) (sdk.Contact, error) {
	owner, err := r.resolver.Resolve(ctx, contact.Channel(channel), identifier, displayName)
	if err != nil {
		return sdk.Contact{}, err
	}
	return sdk.Contact{ID: owner.ID, Name: owner.Name}, nil
}

type erroringResolver struct {
	err error
}

func (r erroringResolver) Resolve(context.Context, sdk.Channel, string, string) (sdk.Contact, error) {
	return sdk.Contact{}, r.err
}

func newSeedReadyPlugin(t *testing.T) *Plugin {
	t.Helper()
	p := newMigratedPlugin(t)
	p.resolver = seedResolver{resolver: contact.NewResolver(postgres.NewContactStore(p.pool))}
	return p
}

func seedCounts(t *testing.T, p *Plugin) [4]int {
	t.Helper()
	return [4]int{
		tableCount(t, p, "plugin_whatsapp.conversations"),
		tableCount(t, p, "plugin_whatsapp.messages"),
		tableCount(t, p, "plugin_whatsapp.media"),
		tableCount(t, p, "core.contacts"),
	}
}

func collectStrings(t *testing.T, p *Plugin, query string) []string {
	t.Helper()
	rows, err := p.pool.Query(t.Context(), query)
	if err != nil {
		t.Fatalf("querying: %v", err)
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatalf("scanning: %v", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading rows: %v", err)
	}
	return values
}

func conversationActivity(t *testing.T, p *Plugin) map[string]time.Time {
	t.Helper()
	rows, err := p.pool.Query(t.Context(),
		"SELECT external_id, last_activity_at FROM plugin_whatsapp.conversations")
	if err != nil {
		t.Fatalf("querying conversations: %v", err)
	}
	defer rows.Close()
	activity := make(map[string]time.Time)
	for rows.Next() {
		var externalID string
		var lastActivityAt time.Time
		if err := rows.Scan(&externalID, &lastActivityAt); err != nil {
			t.Fatalf("scanning: %v", err)
		}
		activity[externalID] = lastActivityAt
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading rows: %v", err)
	}
	return activity
}

func assertSeededDataSet(t *testing.T, p *Plugin) {
	t.Helper()
	if got, want := seedCounts(t, p), [4]int{3, 8, 1, 3}; got != want {
		t.Fatalf("seeded counts = %v, want %v", got, want)
	}
	for wamid, want := range map[string]string{
		"wamid.seed.2": "read",
		"wamid.seed.4": "delivered",
		"wamid.seed.7": "sent",
	} {
		var got string
		err := p.pool.QueryRow(t.Context(),
			"SELECT status FROM plugin_whatsapp.messages WHERE external_id = $1", wamid).Scan(&got)
		if err != nil || got != want {
			t.Fatalf("%s status = %q (err %v), want %q", wamid, got, err, want)
		}
	}
	var stored int
	err := p.pool.QueryRow(t.Context(),
		"SELECT count(*) FROM plugin_whatsapp.media WHERE status = 'stored' AND octet_length(data) > 0").
		Scan(&stored)
	if err != nil || stored != 1 {
		t.Fatalf("stored media rows = %d (err %v), want 1", stored, err)
	}
	recency := collectStrings(t, p,
		"SELECT external_id FROM plugin_whatsapp.conversations ORDER BY last_activity_at DESC")
	if want := []string{"34600111", "184467235", "15551784465"}; !slices.Equal(recency, want) {
		t.Fatalf("conversations by recency = %v, want %v", recency, want)
	}
	thread := collectStrings(t, p, `
		SELECT msg.external_id FROM plugin_whatsapp.messages msg
		JOIN plugin_whatsapp.conversations c ON c.id = msg.conversation_id
		WHERE c.external_id = '184467235' ORDER BY msg.sent_at`)
	if want := []string{"wamid.seed.1", "wamid.seed.2", "wamid.seed.3", "wamid.seed.4"}; !slices.Equal(thread, want) {
		t.Fatalf("first thread order = %v, want %v", thread, want)
	}
}

func TestSeedStoresTheDemoDataSet(t *testing.T) {
	t.Parallel()

	p := newSeedReadyPlugin(t)

	if err := p.Seed(t.Context()); err != nil {
		t.Fatalf("Seed() error = %v, want nil", err)
	}

	assertSeededDataSet(t, p)
	var numberNamed int
	err := p.pool.QueryRow(t.Context(),
		"SELECT count(*) FROM core.contacts WHERE name = '34600111'").Scan(&numberNamed)
	if err != nil || numberNamed != 1 {
		t.Fatalf("number-named contacts = %d (err %v), want 1", numberNamed, err)
	}
}

func TestSeedIsIdempotent(t *testing.T) {
	t.Parallel()

	p := newSeedReadyPlugin(t)

	if err := p.Seed(t.Context()); err != nil {
		t.Fatalf("first Seed() error = %v, want nil", err)
	}
	firstActivity := conversationActivity(t, p)
	if err := p.Seed(t.Context()); err != nil {
		t.Fatalf("second Seed() error = %v, want nil", err)
	}

	assertSeededDataSet(t, p)
	for externalID, first := range firstActivity {
		if second := conversationActivity(t, p)[externalID]; !second.Equal(first) {
			t.Fatalf("conversation %s activity moved from %v to %v on a re-run", externalID, first, second)
		}
	}
}

func TestSeedRepairsAPartiallySeededConversation(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	tests := map[string]func(t *testing.T, p *Plugin, conversationID uuid.UUID){
		"after the first message": func(*testing.T, *Plugin, uuid.UUID) {},
		"after an outbound append": func(t *testing.T, p *Plugin, conversationID uuid.UUID) {
			_, err := p.store.appendOutboundMessage(t.Context(), conversationID, outboundMessage{
				externalID: "wamid.seed.2",
				content:    "Hi María! Yes, we have three left.",
				sentAt:     now.Add(-115 * time.Minute),
				raw:        json.RawMessage(`{}`),
			})
			if err != nil {
				t.Fatalf("pre-inserting outbound message: %v", err)
			}
		},
		"after pending media": func(t *testing.T, p *Plugin, conversationID uuid.UUID) {
			messageID, _, err := insertMessage(t.Context(), p.pool, conversationID, inboundMessage{
				externalID:  "wamid.seed.3",
				content:     "Is it this model?",
				contentType: "image",
				sentAt:      now.Add(-110 * time.Minute),
				raw:         json.RawMessage(`{}`),
			})
			if err != nil {
				t.Fatalf("pre-inserting media message: %v", err)
			}
			descriptor := mediaDescriptor{mediaID: "seed-media-1", mimeType: "image/png", sha256: "c2hh"}
			if err := insertMediaPending(t.Context(), p.pool, messageID, descriptor); err != nil {
				t.Fatalf("pre-inserting pending media: %v", err)
			}
		},
	}
	for name, prepare := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p := newSeedReadyPlugin(t)
			owner, err := p.resolver.Resolve(t.Context(), "whatsapp", "184467235", "María Pérez")
			if err != nil {
				t.Fatalf("resolving owner: %v", err)
			}
			conversationID, err := upsertConversation(
				t.Context(), p.pool, owner.ID, "184467235", now.Add(-2*time.Hour))
			if err != nil {
				t.Fatalf("pre-inserting conversation: %v", err)
			}
			_, _, err = insertMessage(t.Context(), p.pool, conversationID, inboundMessage{
				externalID:  "wamid.seed.1",
				content:     "Hi, do you have the wicker lamp in stock?",
				contentType: "text",
				sentAt:      now.Add(-2 * time.Hour),
				raw:         json.RawMessage(`{}`),
			})
			if err != nil {
				t.Fatalf("pre-inserting first message: %v", err)
			}
			prepare(t, p, conversationID)

			if err := p.Seed(t.Context()); err != nil {
				t.Fatalf("Seed() error = %v, want nil", err)
			}

			assertSeededDataSet(t, p)
		})
	}
}

func TestSeedReportsResolveFailure(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	p.resolver = erroringResolver{err: errors.New("resolver down")}

	if err := p.Seed(t.Context()); err == nil {
		t.Fatal("Seed() error = nil, want a resolve failure")
	}
}

func TestSeedReportsConversationFailure(t *testing.T) {
	t.Parallel()

	pool := newUnreachablePool(t)
	p := &Plugin{pool: pool, store: &store{pool: pool}, events: newBroadcaster(), resolver: staticResolver{}}

	if err := p.Seed(t.Context()); err == nil {
		t.Fatal("Seed() error = nil, want a lookup failure")
	}
}

func TestSeedReportsStorageFailures(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"conversation insert": "ALTER TABLE plugin_whatsapp.conversations ADD CONSTRAINT seed_sabotage CHECK (false)",
		"message insert":      "ALTER TABLE plugin_whatsapp.messages ADD CONSTRAINT seed_sabotage CHECK (false)",
		"outbound append": "ALTER TABLE plugin_whatsapp.messages " +
			"ADD CONSTRAINT seed_sabotage CHECK (direction <> 'outbound')",
		"status apply": "ALTER TABLE plugin_whatsapp.messages ADD CONSTRAINT seed_sabotage CHECK (status IS NULL)",
		"media lookup": "DROP TABLE plugin_whatsapp.media",
		"media insert": "ALTER TABLE plugin_whatsapp.media ADD CONSTRAINT seed_sabotage CHECK (false)",
		"media store":  "ALTER TABLE plugin_whatsapp.media ADD CONSTRAINT seed_sabotage CHECK (status <> 'stored')",
	}
	for name, sabotage := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p := newSeedReadyPlugin(t)
			if _, err := p.pool.Exec(t.Context(), sabotage); err != nil {
				t.Fatalf("adding sabotage: %v", err)
			}

			if err := p.Seed(t.Context()); err == nil {
				t.Fatal("Seed() error = nil, want a storage failure")
			}
		})
	}
}

func TestInsertSeedOutboundReportsEntropyFailure(t *testing.T) {
	uuid.SetRand(failingEntropy{})
	defer uuid.SetRand(nil)

	err := insertSeedOutbound(t.Context(), nil, uuid.Nil, demoMessage{}, time.Time{})

	if !errors.Is(err, errEntropy) {
		t.Fatalf("insertSeedOutbound() error = %v, want the entropy failure in its chain", err)
	}
}

func TestMustBase64RejectsInvalidData(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Fatal("mustBase64(..) did not panic, want a panic")
		}
	}()

	mustBase64("%%%")
}
