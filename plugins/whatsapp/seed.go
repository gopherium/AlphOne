// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gopherium/alphone/sdk"
)

// seedPNG is the one pixel image stored as the demo media attachment.
var seedPNG = mustBase64(
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ" +
		"AAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==")

// mustBase64 decodes a base64 constant, panicking when it is invalid.
func mustBase64(encoded string) []byte {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		panic(err)
	}
	return data
}

// demoMessage is one scripted message of the demo data set.
type demoMessage struct {
	wamid    string
	outbound bool
	kind     string
	content  string
	media    *mediaDescriptor
	status   string
	offset   time.Duration
}

// demoConversation is one scripted conversation of the demo data set.
type demoConversation struct {
	waID     string
	profile  string
	messages []demoMessage
}

// demoScript returns the demo conversations stored by [Plugin.Seed].
func demoScript() []demoConversation {
	sum := sha256.Sum256(seedPNG)
	imageSHA := base64.StdEncoding.EncodeToString(sum[:])
	return []demoConversation{
		{waID: "184467235", profile: "María Pérez", messages: []demoMessage{
			{wamid: "wamid.seed.1", kind: "text", offset: -2 * time.Hour,
				content: "Hi, do you have the wicker lamp in stock?"},
			{wamid: "wamid.seed.2", outbound: true, status: "read", offset: -115 * time.Minute,
				content: "Hi María! Yes, we have three left."},
			{wamid: "wamid.seed.3", kind: "image", offset: -110 * time.Minute,
				content: "Is it this model?",
				media:   &mediaDescriptor{mediaID: "seed-media-1", mimeType: "image/png", sha256: imageSHA}},
			{wamid: "wamid.seed.4", outbound: true, status: "delivered", offset: -105 * time.Minute,
				content: "That's the one. Shall I reserve it for you?"},
		}},
		{waID: "15551784465", profile: "John Doe", messages: []demoMessage{
			{wamid: "wamid.seed.5", kind: "text", offset: -24 * time.Hour,
				content: "Hi, is the order ready?"},
			{wamid: "wamid.seed.6", kind: "location", offset: -23*time.Hour - 55*time.Minute,
				content: "British Museum, 23 Great Russell St"},
			{wamid: "wamid.seed.7", outbound: true, status: "sent", offset: -23*time.Hour - 50*time.Minute,
				content: "Ready at 5pm, see you there."},
		}},
		{waID: "34600111", messages: []demoMessage{
			{wamid: "wamid.seed.8", kind: "text", offset: -30 * time.Minute,
				content: "Hi, do you ship overseas?"},
		}},
	}
}

// Seed stores the demo data set, skipping rows that already exist.
func (p *Plugin) Seed(ctx context.Context) error {
	now := time.Now().UTC()
	for _, conversation := range demoScript() {
		if err := p.seedConversation(ctx, now, conversation); err != nil {
			return err
		}
	}
	return nil
}

// seedConversation stores one scripted conversation's missing rows.
func (p *Plugin) seedConversation(ctx context.Context, now time.Time, c demoConversation) error {
	owner, err := p.resolver.Resolve(ctx, "whatsapp", c.waID, c.profile)
	if err != nil {
		return fmt.Errorf("whatsapp: seed resolve %s: %w", c.waID, err)
	}
	lastOffset := c.messages[len(c.messages)-1].offset
	conversationID, err := p.seedConversationID(ctx, owner.ID, c.waID, now.Add(lastOffset))
	if err != nil {
		return err
	}
	for _, m := range c.messages {
		if err := p.seedMessage(ctx, conversationID, now, m); err != nil {
			return err
		}
	}
	return nil
}

// seedConversationID returns the scripted conversation's id, creating the conversation when it is missing.
func (p *Plugin) seedConversationID(
	ctx context.Context,
	contactID uuid.UUID,
	externalID string,
	activityAt time.Time,
) (uuid.UUID, error) {
	var id uuid.UUID
	err := p.pool.QueryRow(ctx,
		"SELECT id FROM plugin_whatsapp.conversations WHERE external_id = $1 AND tenant_id = $2",
		externalID, sdk.TenantOrDefault(ctx)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return upsertConversation(ctx, p.pool, contactID, externalID, activityAt)
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("whatsapp: find seeded conversation: %w", err)
	}
	return id, nil
}

// seedMessage stores one scripted message and its media blob unless they already exist.
func (p *Plugin) seedMessage(
	ctx context.Context,
	conversationID uuid.UUID,
	now time.Time,
	m demoMessage,
) error {
	if m.outbound {
		return p.seedOutbound(ctx, conversationID, now, m)
	}
	_, _, err := insertMessage(ctx, p.pool, conversationID, inboundMessage{
		externalID:  m.wamid,
		content:     m.content,
		contentType: m.kind,
		sentAt:      now.Add(m.offset),
		raw:         json.RawMessage(`{}`),
	})
	if err != nil {
		return err
	}
	if m.media == nil {
		return nil
	}
	return p.seedMedia(ctx, m)
}

// seedMedia stores the demo blob on the scripted message's media row.
func (p *Plugin) seedMedia(ctx context.Context, m demoMessage) error {
	var messageID uuid.UUID
	var hasMedia bool
	err := p.pool.QueryRow(ctx, `
		SELECT msg.id, EXISTS (
			SELECT 1 FROM plugin_whatsapp.media med
			WHERE med.message_id = msg.id AND med.tenant_id = msg.tenant_id)
		FROM plugin_whatsapp.messages msg
		WHERE msg.external_id = $1 AND msg.tenant_id = $2`,
		m.wamid, sdk.TenantOrDefault(ctx)).Scan(&messageID, &hasMedia)
	if err != nil {
		return fmt.Errorf("whatsapp: find seeded media: %w", err)
	}
	if !hasMedia {
		if err := insertMediaPending(ctx, p.pool, messageID, *m.media); err != nil {
			return err
		}
	}
	return p.store.markMediaStored(ctx, messageID, seedPNG, m.media.mimeType, int64(len(seedPNG)))
}

// seedOutbound stores one scripted outbound message and applies its delivery status.
func (p *Plugin) seedOutbound(
	ctx context.Context,
	conversationID uuid.UUID,
	now time.Time,
	m demoMessage,
) error {
	if err := insertSeedOutbound(ctx, p.pool, conversationID, m, now.Add(m.offset)); err != nil {
		return err
	}
	return p.applyStatus(ctx, statusUpdate{wamid: m.wamid, status: m.status})
}

// insertSeedOutbound stores one scripted outbound message unless it already exists.
func insertSeedOutbound(
	ctx context.Context,
	exec pgxExecutor,
	conversationID uuid.UUID,
	m demoMessage,
	sentAt time.Time,
) error {
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("whatsapp: generate message id: %w", err)
	}
	_, err = exec.Exec(ctx, `
		INSERT INTO plugin_whatsapp.messages (id, conversation_id, external_id, direction, content,
			content_type, sent_at, raw, created_at, tenant_id)
		VALUES ($1, $2, $3, 'outbound', $4, 'text', $5, '{}', $6, $7)
		ON CONFLICT (tenant_id, external_id) DO NOTHING`,
		id, conversationID, m.wamid, m.content, sentAt, time.Now().UTC(), sdk.TenantOrDefault(ctx),
	)
	if err != nil {
		return fmt.Errorf("whatsapp: insert seed outbound: %w", err)
	}
	return nil
}
