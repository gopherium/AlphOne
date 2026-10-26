// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/graph/model"
)

// seedStoredMedia seeds one message with pending media and marks that media stored.
func seedStoredMedia(
	t *testing.T, p *Plugin, externalID string, data []byte, mimeType, filename, sha string,
) (uuid.UUID, uuid.UUID) {
	t.Helper()
	conversationID, messageID := seedMessage(t, p, externalID)
	insertPendingMedia(t, p, messageID, mediaDescriptor{
		mediaID: "MEDIA1", mimeType: "application/octet-stream", sha256: sha, filename: filename,
	})
	if err := p.store.markMediaStored(t.Context(), messageID, data, mimeType, int64(len(data))); err != nil {
		t.Fatalf("markMediaStored() error = %v, want nil", err)
	}
	return conversationID, messageID
}

// getMedia requests one message's media through the plugin routes.
func getMedia(t *testing.T, p *Plugin, conversationID, messageID string) *httptest.ResponseRecorder {
	t.Helper()
	target := "/conversations/" + conversationID + "/messages/" + messageID + "/media"
	request := httptest.NewRequest(http.MethodGet, target, nil)
	recorder := httptest.NewRecorder()
	p.Routes().ServeHTTP(recorder, request)
	return recorder
}

func TestMediaDownloadSendsTheDownloadHeaders(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		externalID      string
		mimeType        string
		filename        string
		wantDisposition string
	}{
		"a photo shown inline": {
			externalID: "wamid.img", mimeType: "image/jpeg", wantDisposition: "inline",
		},
		"a document saved under its name": {
			externalID: "wamid.doc", mimeType: "application/pdf", filename: "receipt.pdf",
			wantDisposition: `attachment; filename=receipt.pdf`,
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			p := newMigratedPlugin(t)
			data := []byte("media-bytes")
			sha := shaOf(data)
			conversationID, messageID := seedStoredMedia(t, p, tc.externalID, data, tc.mimeType, tc.filename, sha)

			recorder := getMedia(t, p, conversationID.String(), messageID.String())

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
			}
			if got := recorder.Body.String(); got != string(data) {
				t.Errorf("body = %q, want %q", got, data)
			}
			expectHeaders(t, recorder.Header(), map[string]string{
				"Content-Type":            tc.mimeType,
				"ETag":                    `"` + sha + `"`,
				"Cache-Control":           "private, max-age=31536000, immutable",
				"X-Content-Type-Options":  "nosniff",
				"Content-Security-Policy": "sandbox",
				"Content-Disposition":     tc.wantDisposition,
				"Accept-Ranges":           "bytes",
			})
			if recorder.Header().Get("Last-Modified") == "" {
				t.Error("Last-Modified missing, want the stored time")
			}
		})
	}
}

// expectHeaders fails the test for every header of want the answer does not carry as want names it.
func expectHeaders(t *testing.T, header http.Header, want map[string]string) {
	t.Helper()
	for name, value := range want {
		if got := header.Get(name); got != value {
			t.Errorf("%s = %q, want %q", name, got, value)
		}
	}
}

func TestMediaDownloadRejectsUnservableRows(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	ctx := t.Context()
	pendingConv, pendingMsg := seedMessage(t, p, "wamid.pending")
	insertPendingMedia(t, p, pendingMsg, mediaDescriptor{mediaID: "M", mimeType: "image/jpeg", sha256: "c2hh"})
	failedConv, failedMsg := seedMessage(t, p, "wamid.failed")
	insertPendingMedia(t, p, failedMsg, mediaDescriptor{mediaID: "M", mimeType: "image/jpeg", sha256: "c2hh"})
	if err := p.store.markMediaFailed(ctx, failedMsg, "expired"); err != nil {
		t.Fatalf("markMediaFailed() error = %v, want nil", err)
	}
	bareConv, bareMsg := seedMessage(t, p, "wamid.bare")
	storedConv, storedMsg := seedStoredMedia(t, p, "wamid.other", []byte("blob"), "image/png", "", "")

	tests := map[string]struct {
		conversationID uuid.UUID
		messageID      uuid.UUID
	}{
		"pending media":          {conversationID: pendingConv, messageID: pendingMsg},
		"failed media":           {conversationID: failedConv, messageID: failedMsg},
		"message without media":  {conversationID: bareConv, messageID: bareMsg},
		"unknown message":        {conversationID: storedConv, messageID: uuid.Must(uuid.NewV7())},
		"cross conversation ids": {conversationID: bareConv, messageID: storedMsg},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			recorder := getMedia(t, p, tc.conversationID.String(), tc.messageID.String())

			if recorder.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
			}
		})
	}

	if recorder := getMedia(t, p, storedConv.String(), storedMsg.String()); recorder.Code != http.StatusOK {
		t.Fatalf("control request status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestMediaDownloadRejectsMalformedIDs(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	valid := uuid.Must(uuid.NewV7()).String()

	tests := map[string]struct {
		conversationID string
		messageID      string
	}{
		"malformed conversation id": {conversationID: "not-a-uuid", messageID: valid},
		"malformed message id":      {conversationID: valid, messageID: "not-a-uuid"},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			recorder := getMedia(t, p, tc.conversationID, tc.messageID)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
		})
	}
}

// threadMedia reads a conversation's messages and keys their media by message id.
func threadMedia(t *testing.T, p *Plugin, conversationID uuid.UUID) map[uuid.UUID]*model.WhatsAppMedia {
	t.Helper()
	messages, err := p.WhatsAppConversationResolvers().Messages(
		t.Context(), &model.WhatsAppConversation{ID: conversationID}, nil)
	if err != nil {
		t.Fatalf("Messages() error = %v, want nil", err)
	}
	byID := make(map[uuid.UUID]*model.WhatsAppMedia, len(messages))
	for _, message := range messages {
		byID[message.ID] = message.Media
	}
	return byID
}

func TestMessagesListCarriesMedia(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	ctx := t.Context()
	data := []byte("blob")
	conversationID, storedID := seedStoredMedia(t, p, "wamid.stored-media", data, "image/jpeg", "photo.jpg", "c2hh")
	textID := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(ctx,
		`INSERT INTO plugin_whatsapp.messages (id, conversation_id, external_id, direction, content,
			content_type, sent_at, raw, created_at)
		VALUES ($1, $2, 'wamid.plain-text', 'inbound', 'hello', 'text', $3, '{}', $3)`,
		textID, conversationID, time.Now().UTC(),
	); err != nil {
		t.Fatalf("inserting text message: %v", err)
	}
	voiceID := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(ctx,
		`INSERT INTO plugin_whatsapp.messages (id, conversation_id, external_id, direction, content,
			content_type, sent_at, raw, created_at)
		VALUES ($1, $2, 'wamid.voice-note', 'inbound', '', 'audio', $3, '{}', $3)`,
		voiceID, conversationID, time.Now().UTC(),
	); err != nil {
		t.Fatalf("inserting voice message: %v", err)
	}
	insertPendingMedia(t, p, voiceID, mediaDescriptor{
		mediaID: "MEDIA2", mimeType: "audio/ogg", sha256: "c2hh", voice: true,
	})

	byID := threadMedia(t, p, conversationID)

	if byID[textID] != nil {
		t.Errorf("text message media = %+v, want null", byID[textID])
	}
	stored := byID[storedID]
	if stored == nil {
		t.Fatal("stored message media = null, want an object")
	}
	if stored.Status != "stored" || stored.MimeType != "image/jpeg" {
		t.Errorf("stored media = (%q, %q), want (stored, image/jpeg)", stored.Status, stored.MimeType)
	}
	if stored.Filename == nil || *stored.Filename != "photo.jpg" {
		t.Errorf("stored filename = %v, want photo.jpg", stored.Filename)
	}
	if stored.FileSize == nil || *stored.FileSize != len(data) {
		t.Errorf("stored fileSize = %v, want %d", stored.FileSize, len(data))
	}
	voice := byID[voiceID]
	if voice == nil {
		t.Fatal("voice message media = null, want an object")
	}
	if voice.Status != "pending" || !voice.Voice {
		t.Errorf("voice media = (%q, voice=%t), want (pending, voice=true)", voice.Status, voice.Voice)
	}
	if voice.FileSize != nil {
		t.Errorf("pending fileSize = %v, want null", voice.FileSize)
	}
}

func TestMediaDownloadReportsStoreFailure(t *testing.T) {
	t.Parallel()

	p := &Plugin{store: &store{pool: newUnreachablePool(t)}, events: newBroadcaster()}

	recorder := getMedia(t, p, uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String())

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}
