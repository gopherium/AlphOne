// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"bytes"
	"errors"
	"mime"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// handleMediaDownload returns an HTTP handler that serves a stored media
// blob for a message within its conversation.
func (p *Plugin) handleMediaDownload() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conversationID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		messageID, err := uuid.Parse(chi.URLParam(r, "mid"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		media, err := p.store.storedMedia(r.Context(), conversationID, messageID)
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		mimeType := media.MimeType
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		w.Header().Set("Content-Type", mimeType)
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "sandbox")
		w.Header().Set("Content-Disposition", mediaDisposition(mimeType, media.Filename))
		if media.SHA256 != "" {
			w.Header().Set("ETag", `"`+media.SHA256+`"`)
		}
		http.ServeContent(w, r, "", media.StoredAt, bytes.NewReader(media.Data))
	}
}

// mediaDisposition builds the Content-Disposition for the mime type, keeping
// inline rendering to a safe allowlist.
func mediaDisposition(mimeType string, filename *string) string {
	if renderableInline(mimeType) {
		return "inline"
	}
	if filename == nil || *filename == "" {
		return "attachment"
	}
	return mime.FormatMediaType("attachment", map[string]string{"filename": *filename})
}

// renderableInline reports whether the mime type is safe to render inline in
// the browser.
func renderableInline(mimeType string) bool {
	mediaType, _, err := mime.ParseMediaType(mimeType)
	if err != nil {
		return false
	}
	if strings.HasPrefix(mediaType, "audio/") {
		return true
	}
	switch mediaType {
	case "image/jpeg", "image/png", "image/webp", "video/mp4", "video/3gpp":
		return true
	}
	return false
}
