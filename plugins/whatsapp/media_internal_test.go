// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func seedMessage(t *testing.T, p *Plugin, externalID string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := t.Context()
	now := time.Now().UTC()
	contactID := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(ctx,
		`INSERT INTO core.contacts (id, name, created_at) VALUES ($1, $2, $3)`,
		contactID, "María Pérez", now,
	); err != nil {
		t.Fatalf("inserting contact: %v", err)
	}
	conversationID := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(ctx,
		`INSERT INTO plugin_whatsapp.conversations (id, contact_id, channel, external_id, status,
			last_activity_at, created_at)
		VALUES ($1, $2, 'whatsapp', $3, 'open', $4, $4)`,
		conversationID, contactID, externalID, now,
	); err != nil {
		t.Fatalf("inserting conversation: %v", err)
	}
	messageID := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(ctx,
		`INSERT INTO plugin_whatsapp.messages (id, conversation_id, external_id, direction, content,
			content_type, sent_at, raw, created_at)
		VALUES ($1, $2, $3, 'inbound', '', 'image', $4, '{}', $4)`,
		messageID, conversationID, externalID, now,
	); err != nil {
		t.Fatalf("inserting message: %v", err)
	}
	return conversationID, messageID
}

func insertPendingMedia(t *testing.T, p *Plugin, messageID uuid.UUID, d mediaDescriptor) {
	t.Helper()
	if err := insertMediaPending(t.Context(), p.pool, messageID, d); err != nil {
		t.Fatalf("insertMediaPending() error = %v, want nil", err)
	}
}

func TestMigrationsCreateMediaPendingIndex(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)

	var count int
	err := p.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM pg_indexes
		WHERE schemaname = 'plugin_whatsapp' AND tablename = 'media' AND indexname = 'media_pending_idx'`,
	).Scan(&count)
	if err != nil {
		t.Fatalf("querying pg_indexes: %v", err)
	}

	if count != 1 {
		t.Fatalf("media_pending_idx count = %d, want 1", count)
	}
}

func TestClaimDueMediaReturnsDuePendingOldestFirst(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	ctx := t.Context()
	conversationA, messageA := seedMessage(t, p, "wamid.a")
	_, messageB := seedMessage(t, p, "wamid.b")
	_, messageC := seedMessage(t, p, "wamid.c")
	for _, id := range []uuid.UUID{messageA, messageB, messageC} {
		insertPendingMedia(t, p, id, mediaDescriptor{mediaID: "MEDIA-" + id.String(), mimeType: "image/jpeg", sha256: "c2hh"})
	}

	pending, err := p.store.claimDueMedia(ctx, time.Now().UTC(), 2)
	if err != nil {
		t.Fatalf("claimDueMedia() error = %v, want nil", err)
	}

	if got, want := len(pending), 2; got != want {
		t.Fatalf("len(pending) = %d, want %d", got, want)
	}
	if pending[0].MessageID != messageA || pending[1].MessageID != messageB {
		t.Fatalf("claim order = %s, %s, want %s, %s", pending[0].MessageID, pending[1].MessageID, messageA, messageB)
	}
	first := pending[0]
	if first.ConversationID != conversationA {
		t.Errorf("ConversationID = %s, want %s", first.ConversationID, conversationA)
	}
	if want := "MEDIA-" + messageA.String(); first.MediaID != want {
		t.Errorf("MediaID = %q, want %q", first.MediaID, want)
	}
	if first.SHA256 != "c2hh" {
		t.Errorf("SHA256 = %q, want %q", first.SHA256, "c2hh")
	}
	if first.Attempts != 0 {
		t.Errorf("Attempts = %d, want 0", first.Attempts)
	}
}

func TestClaimDueMediaSkipsFutureAndSettledRows(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	ctx := t.Context()
	_, due := seedMessage(t, p, "wamid.due")
	_, future := seedMessage(t, p, "wamid.future")
	_, stored := seedMessage(t, p, "wamid.stored")
	_, failed := seedMessage(t, p, "wamid.failed")
	for _, id := range []uuid.UUID{due, future, stored, failed} {
		insertPendingMedia(t, p, id, mediaDescriptor{mediaID: "MEDIA", mimeType: "image/jpeg", sha256: "c2hh"})
	}
	if err := p.store.rescheduleMedia(ctx, future, "graph 500", time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("rescheduleMedia() error = %v, want nil", err)
	}
	if err := p.store.markMediaStored(ctx, stored, []byte("bytes"), "image/jpeg", 5); err != nil {
		t.Fatalf("markMediaStored() error = %v, want nil", err)
	}
	if err := p.store.markMediaFailed(ctx, failed, "too_large"); err != nil {
		t.Fatalf("markMediaFailed() error = %v, want nil", err)
	}

	pending, err := p.store.claimDueMedia(ctx, time.Now().UTC(), 10)
	if err != nil {
		t.Fatalf("claimDueMedia() error = %v, want nil", err)
	}

	if got, want := len(pending), 1; got != want {
		t.Fatalf("len(pending) = %d, want %d", got, want)
	}
	if pending[0].MessageID != due {
		t.Fatalf("pending[0].MessageID = %s, want %s", pending[0].MessageID, due)
	}
}

func TestMarkMediaStoredIgnoresSettledRows(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	ctx := t.Context()
	_, messageID := seedMessage(t, p, "wamid.stored-once")
	insertPendingMedia(t, p, messageID, mediaDescriptor{mediaID: "MEDIA", mimeType: "image/jpeg", sha256: "c2hh"})
	if err := p.store.markMediaStored(ctx, messageID, []byte("first"), "image/png", 5); err != nil {
		t.Fatalf("markMediaStored() error = %v, want nil", err)
	}

	if err := p.store.markMediaStored(ctx, messageID, []byte("second"), "application/pdf", 6); err != nil {
		t.Fatalf("markMediaStored() second call error = %v, want nil", err)
	}

	var status, mimeType string
	var data []byte
	var fileSize int64
	var storedAt *time.Time
	err := p.pool.QueryRow(ctx,
		`SELECT status, data, mime_type, file_size, stored_at FROM plugin_whatsapp.media WHERE message_id = $1`,
		messageID,
	).Scan(&status, &data, &mimeType, &fileSize, &storedAt)
	if err != nil {
		t.Fatalf("querying media row: %v", err)
	}
	if status != "stored" {
		t.Errorf("status = %q, want %q", status, "stored")
	}
	if string(data) != "first" {
		t.Errorf("data = %q, want %q", data, "first")
	}
	if mimeType != "image/png" {
		t.Errorf("mime_type = %q, want %q", mimeType, "image/png")
	}
	if fileSize != 5 {
		t.Errorf("file_size = %d, want 5", fileSize)
	}
	if storedAt == nil {
		t.Error("stored_at = nil, want a timestamp")
	}
}

func TestMarkMediaFailedIgnoresSettledRows(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	ctx := t.Context()
	_, failing := seedMessage(t, p, "wamid.failing")
	_, stored := seedMessage(t, p, "wamid.already-stored")
	insertPendingMedia(t, p, failing, mediaDescriptor{mediaID: "MEDIA", mimeType: "image/jpeg", sha256: "c2hh"})
	insertPendingMedia(t, p, stored, mediaDescriptor{mediaID: "MEDIA", mimeType: "image/jpeg", sha256: "c2hh"})
	if err := p.store.markMediaStored(ctx, stored, []byte("bytes"), "image/jpeg", 5); err != nil {
		t.Fatalf("markMediaStored() error = %v, want nil", err)
	}

	if err := p.store.markMediaFailed(ctx, failing, "too_large"); err != nil {
		t.Fatalf("markMediaFailed() error = %v, want nil", err)
	}
	if err := p.store.markMediaFailed(ctx, stored, "boom"); err != nil {
		t.Fatalf("markMediaFailed() on stored row error = %v, want nil", err)
	}

	var status string
	var lastError *string
	if err := p.pool.QueryRow(ctx,
		`SELECT status, last_error FROM plugin_whatsapp.media WHERE message_id = $1`, failing,
	).Scan(&status, &lastError); err != nil {
		t.Fatalf("querying failed row: %v", err)
	}
	if status != "failed" || lastError == nil || *lastError != "too_large" {
		t.Errorf("failed row = (%q, %v), want (failed, too_large)", status, lastError)
	}
	if err := p.pool.QueryRow(ctx,
		`SELECT status FROM plugin_whatsapp.media WHERE message_id = $1`, stored,
	).Scan(&status); err != nil {
		t.Fatalf("querying stored row: %v", err)
	}
	if status != "stored" {
		t.Errorf("stored row status = %q, want %q", status, "stored")
	}
}

func TestRescheduleMediaDefersTheNextAttempt(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	ctx := t.Context()
	_, messageID := seedMessage(t, p, "wamid.retry")
	insertPendingMedia(t, p, messageID, mediaDescriptor{mediaID: "MEDIA", mimeType: "image/jpeg", sha256: "c2hh"})
	nextAttempt := time.Now().UTC().Add(time.Hour)

	if err := p.store.rescheduleMedia(ctx, messageID, "graph 500", nextAttempt); err != nil {
		t.Fatalf("rescheduleMedia() error = %v, want nil", err)
	}

	pending, err := p.store.claimDueMedia(ctx, time.Now().UTC(), 10)
	if err != nil {
		t.Fatalf("claimDueMedia() error = %v, want nil", err)
	}
	if len(pending) != 0 {
		t.Fatalf("len(pending) before the deferral = %d, want 0", len(pending))
	}
	pending, err = p.store.claimDueMedia(ctx, nextAttempt.Add(time.Minute), 10)
	if err != nil {
		t.Fatalf("claimDueMedia() after the deferral error = %v, want nil", err)
	}
	if len(pending) != 1 {
		t.Fatalf("len(pending) after the deferral = %d, want 1", len(pending))
	}
	if pending[0].Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", pending[0].Attempts)
	}
}

func TestMediaRowsCascadeWithTheirMessage(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	ctx := t.Context()
	_, messageID := seedMessage(t, p, "wamid.cascade")
	insertPendingMedia(t, p, messageID, mediaDescriptor{mediaID: "MEDIA", mimeType: "image/jpeg", sha256: "c2hh"})

	if _, err := p.pool.Exec(ctx, `DELETE FROM plugin_whatsapp.messages WHERE id = $1`, messageID); err != nil {
		t.Fatalf("deleting message: %v", err)
	}

	var count int
	if err := p.pool.QueryRow(ctx, `SELECT count(*) FROM plugin_whatsapp.media`).Scan(&count); err != nil {
		t.Fatalf("counting media rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("media rows after message delete = %d, want 0", count)
	}
}

func TestMediaStoreReportsFailures(t *testing.T) {
	t.Parallel()

	s := &store{pool: newUnreachablePool(t)}
	messageID := uuid.Must(uuid.NewV7())

	if err := insertMediaPending(t.Context(), s.pool, messageID, mediaDescriptor{}); err == nil {
		t.Error("insertMediaPending() error = nil, want a connection failure")
	}
	if _, err := s.claimDueMedia(t.Context(), time.Now().UTC(), 1); err == nil {
		t.Error("claimDueMedia() error = nil, want a connection failure")
	}
	if err := s.markMediaStored(t.Context(), messageID, nil, "image/jpeg", 0); err == nil {
		t.Error("markMediaStored() error = nil, want a connection failure")
	}
	if err := s.markMediaFailed(t.Context(), messageID, "boom"); err == nil {
		t.Error("markMediaFailed() error = nil, want a connection failure")
	}
	if err := s.rescheduleMedia(t.Context(), messageID, "boom", time.Now().UTC()); err == nil {
		t.Error("rescheduleMedia() error = nil, want a connection failure")
	}
}
