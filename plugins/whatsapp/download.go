// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"bytes"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gopherium/alphone/sdk"
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
		sdk.ServeDownload(w, r, sdk.Download{
			Content:     bytes.NewReader(media.Data),
			ContentType: media.MimeType,
			Filename:    media.Filename,
			SHA256:      media.SHA256,
			ModTime:     media.StoredAt,
		})
	}
}
