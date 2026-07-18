// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"encoding/json"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

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

func getMedia(
	t *testing.T, p *Plugin, conversationID, messageID string, header http.Header,
) *httptest.ResponseRecorder {
	t.Helper()
	target := "/conversations/" + conversationID + "/messages/" + messageID + "/media"
	request := httptest.NewRequest(http.MethodGet, target, nil)
	for name, values := range header {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	recorder := httptest.NewRecorder()
	p.Routes().ServeHTTP(recorder, request)
	return recorder
}

func TestMediaDownloadServesStoredImages(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	data := []byte("img-bytes")
	sha := shaOf(data)
	conversationID, messageID := seedStoredMedia(t, p, "wamid.img", data, "image/jpeg", "", sha)

	recorder := getMedia(t, p, conversationID.String(), messageID.String(), nil)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Body.String(); got != string(data) {
		t.Errorf("body = %q, want %q", got, data)
	}
	headers := map[string]string{
		"Content-Type":            "image/jpeg",
		"ETag":                    `"` + sha + `"`,
		"Cache-Control":           "private, max-age=31536000, immutable",
		"X-Content-Type-Options":  "nosniff",
		"Content-Security-Policy": "sandbox",
		"Content-Disposition":     "inline",
		"Accept-Ranges":           "bytes",
	}
	for name, want := range headers {
		if got := recorder.Header().Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if recorder.Header().Get("Last-Modified") == "" {
		t.Error("Last-Modified missing, want the stored time")
	}
}

func TestMediaDownloadServesRanges(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	data := []byte("range-bytes-payload")
	conversationID, messageID := seedStoredMedia(t, p, "wamid.range", data, "audio/ogg", "", shaOf(data))

	recorder := getMedia(t, p, conversationID.String(), messageID.String(), http.Header{"Range": {"bytes=0-3"}})

	if recorder.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusPartialContent)
	}
	if got := recorder.Body.String(); got != "rang" {
		t.Errorf("body = %q, want %q", got, "rang")
	}
	if got := recorder.Header().Get("Content-Range"); !strings.HasPrefix(got, "bytes 0-3/") {
		t.Errorf("Content-Range = %q, want a bytes 0-3 range", got)
	}
}

func TestMediaDownloadHonorsETags(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	data := []byte("cached-bytes")
	sha := shaOf(data)
	conversationID, messageID := seedStoredMedia(t, p, "wamid.etag", data, "image/png", "", sha)

	recorder := getMedia(t, p, conversationID.String(), messageID.String(),
		http.Header{"If-None-Match": {`"` + sha + `"`}})

	if recorder.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotModified)
	}
	if recorder.Body.Len() != 0 {
		t.Errorf("body length = %d, want 0", recorder.Body.Len())
	}
}

func TestMediaDownloadSkipsETagWithoutHash(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	data := []byte("hashless")
	conversationID, messageID := seedStoredMedia(t, p, "wamid.nosha", data, "image/png", "", "")

	recorder := getMedia(t, p, conversationID.String(), messageID.String(), nil)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("ETag"); got != "" {
		t.Errorf("ETag = %q, want none without a hash", got)
	}
}

func TestMediaDownloadDispositions(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		mimeType        string
		filename        string
		wantType        string
		wantDisposition string
		wantFilename    string
	}{
		"pdf with filename": {
			mimeType:        "application/pdf",
			filename:        "receipt.pdf",
			wantType:        "application/pdf",
			wantDisposition: "attachment",
			wantFilename:    "receipt.pdf",
		},
		"pdf with unicode filename": {
			mimeType:        "application/pdf",
			filename:        "recibö ñ.pdf",
			wantType:        "application/pdf",
			wantDisposition: "attachment",
			wantFilename:    "recibö ñ.pdf",
		},
		"pdf without filename": {
			mimeType:        "application/pdf",
			filename:        "",
			wantType:        "application/pdf",
			wantDisposition: "attachment",
		},
		"opus voice note": {
			mimeType:        "audio/ogg; codecs=opus",
			filename:        "",
			wantType:        "audio/ogg; codecs=opus",
			wantDisposition: "inline",
		},
		"webp sticker": {
			mimeType:        "image/webp",
			filename:        "",
			wantType:        "image/webp",
			wantDisposition: "inline",
		},
		"mp4 video": {
			mimeType:        "video/mp4",
			filename:        "",
			wantType:        "video/mp4",
			wantDisposition: "inline",
		},
		"unknown type": {
			mimeType:        "application/x-thing",
			filename:        "",
			wantType:        "application/x-thing",
			wantDisposition: "attachment",
		},
		"empty type": {
			mimeType:        "",
			filename:        "",
			wantType:        "application/octet-stream",
			wantDisposition: "attachment",
		},
		"unparsable type": {
			mimeType:        "image/png; =",
			filename:        "",
			wantType:        "image/png; =",
			wantDisposition: "attachment",
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			p := newMigratedPlugin(t)
			data := []byte("blob")
			conversationID, messageID := seedStoredMedia(t, p, "wamid.disp", data, tc.mimeType, tc.filename, shaOf(data))

			recorder := getMedia(t, p, conversationID.String(), messageID.String(), nil)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
			}
			if got := recorder.Header().Get("Content-Type"); got != tc.wantType {
				t.Errorf("Content-Type = %q, want %q", got, tc.wantType)
			}
			disposition, params, err := mime.ParseMediaType(recorder.Header().Get("Content-Disposition"))
			if err != nil {
				t.Fatalf("parsing Content-Disposition %q: %v", recorder.Header().Get("Content-Disposition"), err)
			}
			if disposition != tc.wantDisposition {
				t.Errorf("disposition = %q, want %q", disposition, tc.wantDisposition)
			}
			if params["filename"] != tc.wantFilename {
				t.Errorf("filename = %q, want %q", params["filename"], tc.wantFilename)
			}
		})
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

			recorder := getMedia(t, p, tc.conversationID.String(), tc.messageID.String(), nil)

			if recorder.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
			}
		})
	}

	if recorder := getMedia(t, p, storedConv.String(), storedMsg.String(), nil); recorder.Code != http.StatusOK {
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

			recorder := getMedia(t, p, tc.conversationID, tc.messageID, nil)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
		})
	}
}

type mediaPayload struct {
	Status   string  `json:"status"`
	MimeType string  `json:"mime_type"`
	Filename *string `json:"filename"`
	FileSize *int64  `json:"file_size"`
	Voice    bool    `json:"voice"`
	Animated bool    `json:"animated"`
}

type messagePayload struct {
	ID    uuid.UUID     `json:"id"`
	Media *mediaPayload `json:"media"`
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
		VALUES ($1, $2, 'wamid.plain-text', 'inbound', 'hola', 'text', $3, '{}', $3)`,
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

	request := httptest.NewRequest(http.MethodGet, "/conversations/"+conversationID.String()+"/messages", nil)
	recorder := httptest.NewRecorder()
	p.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var messages []messagePayload
	if err := json.Unmarshal(recorder.Body.Bytes(), &messages); err != nil {
		t.Fatalf("decoding messages: %v", err)
	}
	byID := make(map[uuid.UUID]*mediaPayload, len(messages))
	for _, message := range messages {
		byID[message.ID] = message.Media
	}
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
	if stored.FileSize == nil || *stored.FileSize != int64(len(data)) {
		t.Errorf("stored file_size = %v, want %d", stored.FileSize, len(data))
	}
	voice := byID[voiceID]
	if voice == nil {
		t.Fatal("voice message media = null, want an object")
	}
	if voice.Status != "pending" || !voice.Voice {
		t.Errorf("voice media = (%q, voice=%t), want (pending, voice=true)", voice.Status, voice.Voice)
	}
	if voice.FileSize != nil {
		t.Errorf("pending file_size = %v, want null", voice.FileSize)
	}
}

func TestMediaDownloadReportsStoreFailure(t *testing.T) {
	t.Parallel()

	p := &Plugin{store: &store{pool: newUnreachablePool(t)}, events: newBroadcaster()}

	recorder := getMedia(t, p, uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String(), nil)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}
