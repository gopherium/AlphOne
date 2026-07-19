// SPDX-License-Identifier: Elastic-2.0

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
	"strings"
	"time"
)

type inboundMessage struct {
	externalID  string
	sender      string
	senderName  string
	content     string
	contentType string
	media       *mediaDescriptor
	sentAt      time.Time
	raw         json.RawMessage
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
				Statuses []json.RawMessage `json:"statuses"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

type statusUpdate struct {
	wamid  string
	status string
	detail string
}

type webhookStatus struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Errors []struct {
		Code  int    `json:"code"`
		Title string `json:"title"`
	} `json:"errors"`
}

type webhookBatch struct {
	messages []inboundMessage
	statuses []statusUpdate
}

type webhookMessage struct {
	From      string `json:"from"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Text      struct {
		Body string `json:"body"`
	} `json:"text"`
	Image    *webhookMedia        `json:"image"`
	Audio    *webhookMedia        `json:"audio"`
	Video    *webhookMedia        `json:"video"`
	Document *webhookMedia        `json:"document"`
	Sticker  *webhookMedia        `json:"sticker"`
	Location *webhookLocation     `json:"location"`
	Contacts []webhookContactCard `json:"contacts"`
	Reaction *webhookReaction     `json:"reaction"`
}

type webhookMedia struct {
	ID       string `json:"id"`
	MimeType string `json:"mime_type"`
	SHA256   string `json:"sha256"`
	Caption  string `json:"caption"`
	Filename string `json:"filename"`
	Voice    bool   `json:"voice"`
	Animated bool   `json:"animated"`
}

type webhookLocation struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Name      string  `json:"name"`
	Address   string  `json:"address"`
}

type webhookContactCard struct {
	Name struct {
		FormattedName string `json:"formatted_name"`
	} `json:"name"`
}

type webhookReaction struct {
	Emoji string `json:"emoji"`
}

// handleEvents returns an HTTP handler that verifies the webhook
// signature, parses the inbound messages, and ingests them.
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
		batch, err := parseWebhook(body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for _, m := range batch.messages {
			if err := p.ingest(r.Context(), m); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		for _, u := range batch.statuses {
			if err := p.applyStatus(r.Context(), u); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	}
}

// signatureValid reports whether header holds a valid HMAC-SHA256
// signature of body computed with the app secret.
func (p *Plugin) signatureValid(header string, body []byte) bool {
	if p.appSecret == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(p.appSecret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(header), []byte(expected))
}

// parseWebhook decodes a webhook payload into the attributable inbound
// messages and delivery status updates it contains.
func parseWebhook(body []byte) (webhookBatch, error) {
	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return webhookBatch{}, fmt.Errorf("whatsapp: decode payload: %w", err)
	}
	var batch webhookBatch
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			names := make(map[string]string, len(change.Value.Contacts))
			for _, sender := range change.Value.Contacts {
				names[sender.WaID] = sender.Profile.Name
			}
			for _, raw := range change.Value.Messages {
				if m, ok := parseMessage(names, raw); ok {
					batch.messages = append(batch.messages, m)
				}
			}
			for _, raw := range change.Value.Statuses {
				if u, ok := parseStatus(raw); ok {
					batch.statuses = append(batch.statuses, u)
				}
			}
		}
	}
	return batch, nil
}

// parseStatus converts one webhook status entry into a status update,
// reporting whether it carries enough identity to apply.
func parseStatus(raw json.RawMessage) (statusUpdate, bool) {
	var s webhookStatus
	if err := json.Unmarshal(raw, &s); err != nil {
		return statusUpdate{}, false
	}
	if s.ID == "" || s.Status == "" {
		return statusUpdate{}, false
	}
	update := statusUpdate{wamid: s.ID, status: s.Status}
	if s.Status == "failed" && len(s.Errors) > 0 {
		update.detail = strings.TrimSpace(fmt.Sprintf("%d %s", s.Errors[0].Code, s.Errors[0].Title))
	}
	return update, true
}

// applyStatus records a delivery status update and broadcasts the change
// when it advanced a message.
func (p *Plugin) applyStatus(ctx context.Context, u statusUpdate) error {
	conversationID, applied, err := p.store.applyMessageStatus(ctx, u)
	if err != nil {
		return err
	}
	if applied {
		p.events.broadcast(event{Conversation: conversationID})
	}
	return nil
}

// parseMessage converts one webhook message into an inbound message,
// reporting whether the message carries enough identity to store.
func parseMessage(names map[string]string, raw json.RawMessage) (inboundMessage, bool) {
	var m webhookMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return inboundMessage{}, false
	}
	if m.From == "" || m.ID == "" {
		return inboundMessage{}, false
	}
	message := inboundMessage{
		externalID: m.ID,
		sender:     m.From,
		senderName: names[m.From],
		sentAt:     parseTimestamp(m.Timestamp),
		raw:        raw,
	}
	message.contentType, message.content, message.media = classifyMessage(m)
	return message, true
}

// parseTimestamp converts a webhook Unix timestamp, falling back to the
// current time when it does not parse.
func parseTimestamp(value string) time.Time {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Now().UTC()
	}
	return time.Unix(seconds, 0).UTC()
}

// classifyMessage maps a webhook message to its content type, content text,
// and media descriptor.
func classifyMessage(m webhookMessage) (string, string, *mediaDescriptor) {
	switch m.Type {
	case "text":
		return "text", m.Text.Body, nil
	case "image":
		return classifyMedia("image", m.Image)
	case "audio":
		return classifyMedia("audio", m.Audio)
	case "video":
		return classifyMedia("video", m.Video)
	case "document":
		return classifyMedia("document", m.Document)
	case "sticker":
		return classifyMedia("sticker", m.Sticker)
	case "location":
		if m.Location == nil {
			return "unsupported", "", nil
		}
		return "location", locationContent(*m.Location), nil
	case "contacts":
		return "contacts", contactsContent(m.Contacts), nil
	case "reaction":
		if m.Reaction == nil {
			return "unsupported", "", nil
		}
		return "reaction", m.Reaction.Emoji, nil
	default:
		return "unsupported", "", nil
	}
}

// classifyMedia builds the classification for a media message of the given
// kind, degrading to unsupported when the asset reference is missing.
func classifyMedia(kind string, media *webhookMedia) (string, string, *mediaDescriptor) {
	if media == nil || media.ID == "" {
		return "unsupported", "", nil
	}
	return kind, media.Caption, &mediaDescriptor{
		mediaID:  media.ID,
		mimeType: media.MimeType,
		sha256:   media.SHA256,
		filename: media.Filename,
		voice:    media.Voice,
		animated: media.Animated,
	}
}

// locationContent renders a shared location as its place text or its
// coordinates.
func locationContent(l webhookLocation) string {
	place := strings.TrimSpace(strings.TrimSpace(l.Name) + " " + strings.TrimSpace(l.Address))
	if place != "" {
		return place
	}
	return strconv.FormatFloat(l.Latitude, 'f', -1, 64) + ", " + strconv.FormatFloat(l.Longitude, 'f', -1, 64)
}

// contactsContent joins shared contact cards into a name list.
func contactsContent(cards []webhookContactCard) string {
	names := make([]string, 0, len(cards))
	for _, card := range cards {
		if name := strings.TrimSpace(card.Name.FormattedName); name != "" {
			names = append(names, name)
		}
	}
	return strings.Join(names, ", ")
}

// ingest stores an inbound message and broadcasts newly stored arrivals.
func (p *Plugin) ingest(ctx context.Context, m inboundMessage) error {
	owner, err := p.resolver.Resolve(ctx, "whatsapp", m.sender, m.senderName)
	if err != nil {
		return fmt.Errorf("whatsapp: resolve sender: %w", err)
	}
	conversationID, stored, err := p.store.persistInbound(ctx, owner.ID, m)
	if err != nil {
		return err
	}
	if stored {
		p.events.broadcast(event{Conversation: conversationID})
		if m.media != nil {
			p.fetcher.poke()
		}
	}
	return nil
}
