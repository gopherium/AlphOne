// SPDX-License-Identifier: AGPL-3.0-or-later

package whatsapp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

type inboundMessage struct {
	externalID string
	sender     string
	senderName string
	text       string
	sentAt     time.Time
	raw        json.RawMessage
}

type webhookPayload struct {
	Entry []struct {
		Changes []struct {
			Value struct {
				Contacts []struct {
					WaID    string `json:"wa_id"`
					Profile struct {
						Name string `json:"name"`
					} `json:"profile"`
				} `json:"contacts"`
				Messages []json.RawMessage `json:"messages"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

type webhookMessage struct {
	From      string `json:"from"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Text      struct {
		Body string `json:"body"`
	} `json:"text"`
}

// handleEvents returns an HTTP handler that verifies the webhook signature, parses inbound text messages, and ingests them.
func (p *Plugin) handleEvents() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if !p.signatureValid(r.Header.Get("X-Hub-Signature-256"), body) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		messages, err := parseTextMessages(body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for _, m := range messages {
			if err := p.ingest(r.Context(), m); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	}
}

// signatureValid reports whether the given header holds a valid HMAC-SHA256 signature of body computed with the app secret.
func (p *Plugin) signatureValid(header string, body []byte) bool {
	if p.appSecret == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(p.appSecret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(header), []byte(expected))
}

// parseTextMessages decodes a webhook payload and returns the inbound text messages it contains.
func parseTextMessages(body []byte) ([]inboundMessage, error) {
	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("whatsapp: decode payload: %w", err)
	}
	var messages []inboundMessage
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			names := make(map[string]string, len(change.Value.Contacts))
			for _, sender := range change.Value.Contacts {
				names[sender.WaID] = sender.Profile.Name
			}
			for _, raw := range change.Value.Messages {
				var m webhookMessage
				if err := json.Unmarshal(raw, &m); err != nil {
					return nil, fmt.Errorf("whatsapp: decode message: %w", err)
				}
				if m.Type != "text" {
					continue
				}
				seconds, err := strconv.ParseInt(m.Timestamp, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("whatsapp: parse timestamp %q: %w", m.Timestamp, err)
				}
				messages = append(messages, inboundMessage{
					externalID: m.ID,
					sender:     m.From,
					senderName: names[m.From],
					text:       m.Text.Body,
					sentAt:     time.Unix(seconds, 0).UTC(),
					raw:        raw,
				})
			}
		}
	}
	return messages, nil
}

// ingest stores an inbound message and broadcasts the change.
func (p *Plugin) ingest(ctx context.Context, m inboundMessage) error {
	owner, err := p.resolver.Resolve(ctx, "whatsapp", m.sender, m.senderName)
	if err != nil {
		return fmt.Errorf("whatsapp: resolve sender: %w", err)
	}
	conversationID, err := p.store.upsertConversation(ctx, owner.ID, m.sender, m.sentAt)
	if err != nil {
		return err
	}
	err = p.store.insertMessage(ctx, conversationID, m)
	p.events.broadcast(event{Conversation: conversationID})
	return err
}
