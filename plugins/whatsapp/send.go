// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const defaultGraphURL = "https://graph.facebook.com/v23.0"

type sender struct {
	client        *http.Client
	baseURL       string
	accessToken   string
	phoneNumberID string
}

type sendTextRequest struct {
	MessagingProduct string       `json:"messaging_product"`
	To               string       `json:"to"`
	Type             string       `json:"type"`
	Text             sendTextBody `json:"text"`
}

type sendTextBody struct {
	Body string `json:"body"`
}

// sendText posts a WhatsApp text message to the Cloud API and returns the resulting message id and raw response.
func (s *sender) sendText(ctx context.Context, to, body string) (string, json.RawMessage, error) {
	payload, _ := json.Marshal(sendTextRequest{
		MessagingProduct: "whatsapp",
		To:               to,
		Type:             "text",
		Text:             sendTextBody{Body: body},
	})
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		s.baseURL+"/"+s.phoneNumberID+"/messages",
		bytes.NewReader(payload),
	)
	if err != nil {
		return "", nil, fmt.Errorf("whatsapp: build send request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+s.accessToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return "", nil, fmt.Errorf("whatsapp: send message: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	raw, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("whatsapp: send message: status %d", response.StatusCode)
	}
	var decoded struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return "", nil, fmt.Errorf("whatsapp: decode send response: %w", err)
	}
	if len(decoded.Messages) == 0 || decoded.Messages[0].ID == "" {
		return "", nil, errors.New("whatsapp: send response carries no message id")
	}
	return decoded.Messages[0].ID, raw, nil
}

type sendMessageRequest struct {
	Content string `json:"content"`
}

// handleMessageSend returns an HTTP handler that sends an outbound message on a conversation and persists it.
func (p *Plugin) handleMessageSend() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conversationID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var body sendMessageRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		content := strings.TrimSpace(body.Content)
		if content == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		to, err := p.store.conversationExternalID(r.Context(), conversationID)
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		externalID, raw, err := p.sender.sendText(r.Context(), to, content)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		row, err := p.store.appendOutboundMessage(r.Context(), conversationID, outboundMessage{
			externalID: externalID,
			content:    content,
			sentAt:     time.Now().UTC(),
			raw:        raw,
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		p.events.broadcast(event{Conversation: conversationID})
		respondJSON(w, http.StatusCreated, messageResponse{
			ID:          row.ID,
			ExternalID:  row.ExternalID,
			Direction:   row.Direction,
			Content:     row.Content,
			ContentType: row.ContentType,
			SentAt:      row.SentAt.UTC(),
		})
	}
}
