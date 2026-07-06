// SPDX-License-Identifier: AGPL-3.0-or-later

package whatsapp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

type conversationResponse struct {
	ID             uuid.UUID `json:"id"`
	ContactID      uuid.UUID `json:"contact_id"`
	ContactName    string    `json:"contact_name"`
	ExternalID     string    `json:"external_id"`
	Status         string    `json:"status"`
	LastActivityAt time.Time `json:"last_activity_at"`
}

type messageResponse struct {
	ID          uuid.UUID `json:"id"`
	ExternalID  string    `json:"external_id"`
	Direction   string    `json:"direction"`
	Content     string    `json:"content"`
	ContentType string    `json:"content_type"`
	SentAt      time.Time `json:"sent_at"`
}

func (p *Plugin) handleConversationsList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, err := parseListLimit(r.URL.Query())
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		rows, err := p.store.listConversations(r.Context(), limit)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		conversations := make([]conversationResponse, 0, len(rows))
		for _, row := range rows {
			conversations = append(conversations, conversationResponse{
				ID:             row.ID,
				ContactID:      row.ContactID,
				ContactName:    row.ContactName,
				ExternalID:     row.ExternalID,
				Status:         row.Status,
				LastActivityAt: row.LastActivityAt.UTC(),
			})
		}
		respondJSON(w, http.StatusOK, conversations)
	}
}

func (p *Plugin) handleMessagesList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conversationID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		limit, err := parseListLimit(r.URL.Query())
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		rows, err := p.store.listMessages(r.Context(), conversationID, limit)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		messages := make([]messageResponse, 0, len(rows))
		for _, row := range rows {
			messages = append(messages, messageResponse{
				ID:          row.ID,
				ExternalID:  row.ExternalID,
				Direction:   row.Direction,
				Content:     row.Content,
				ContentType: row.ContentType,
				SentAt:      row.SentAt.UTC(),
			})
		}
		respondJSON(w, http.StatusOK, messages)
	}
}

func parseListLimit(query url.Values) (int, error) {
	raw := query.Get("limit")
	if raw == "" {
		return defaultListLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > maxListLimit {
		return 0, fmt.Errorf("whatsapp: invalid limit %q", raw)
	}
	return limit, nil
}

func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
