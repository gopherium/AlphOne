// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

// eventMessageReceived names the event a stored inbound message announces.
const eventMessageReceived = "whatsapp.message.received"

// defaultDeliveryItemsMax is how many items one delivery stores when the setting is unset.
const defaultDeliveryItemsMax = 2000

// defaultDeliveryNumbersMax is how many numbers with their own secret one delivery names when the setting is unset.
const defaultDeliveryNumbersMax = 100

type inboundMessage struct {
	externalID         string
	sender             string
	senderName         string
	content            string
	contentType        string
	media              *mediaDescriptor
	sentAt             time.Time
	raw                json.RawMessage
	tenantID           uuid.UUID
	conversationNumber *string
}

type webhookPayload struct {
	Entry []struct {
		Changes []webhookChange `json:"changes"`
	} `json:"entry"`
}

// webhookNames is the part of a delivery read before any secret proves it.
type webhookNames struct {
	Entry []struct {
		Changes []struct {
			Value struct {
				Metadata webhookMetadata `json:"metadata"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

type webhookChange struct {
	Value struct {
		Metadata webhookMetadata   `json:"metadata"`
		Contacts []webhookSender   `json:"contacts"`
		Messages []json.RawMessage `json:"messages"`
		Statuses []json.RawMessage `json:"statuses"`
	} `json:"value"`
}

// webhookMetadata names the business number a change arrived on.
type webhookMetadata struct {
	PhoneNumberID string `json:"phone_number_id"`
}

type webhookSender struct {
	WaID    string `json:"wa_id"`
	UserID  string `json:"user_id"`
	Profile struct {
		Name string `json:"name"`
	} `json:"profile"`
}

type statusUpdate struct {
	wamid    string
	status   string
	detail   string
	tenantID uuid.UUID
	number   *string
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
	limit    int
	messages []inboundMessage
	statuses []statusUpdate
}

type webhookMessage struct {
	From       string `json:"from"`
	FromUserID string `json:"from_user_id"`
	ID         string `json:"id"`
	Timestamp  string `json:"timestamp"`
	Type       string `json:"type"`
	Text       struct {
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

// handleEvents returns an HTTP handler that reads one webhook delivery and answers what became of it.
func (p *Plugin) handleEvents() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		digest, shaped := parseSignature(r.Header.Get("X-Hub-Signature-256"))
		if !shaped {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(p.receive(r.Context(), digest, body))
	}
}

// receive stores what a delivery's signature proves, answering the status Meta reads.
func (p *Plugin) receive(ctx context.Context, digest, body []byte) int {
	signed := signedBody{digest: digest, body: body}
	named, err := namedNumbers(body)
	if err != nil {
		if signed.signedWith([]byte(p.appSecret)) {
			return http.StatusBadRequest
		}
		return http.StatusForbidden
	}
	routes, proven, err := p.proveDelivery(ctx, signed, named)
	if err != nil {
		return http.StatusInternalServerError
	}
	if !proven {
		return http.StatusForbidden
	}
	open, err := p.openRoutes(ctx, routes)
	if err != nil {
		return http.StatusInternalServerError
	}
	batch, err := parseWebhook(body, open, p.deliveryItemsMax)
	if err != nil {
		return http.StatusBadRequest
	}
	if err := p.ingestBatch(ctx, batch); err != nil {
		return http.StatusInternalServerError
	}
	return http.StatusOK
}

// ingestBatch stores the messages and applies the status updates of a batch, each in its own tenant.
func (p *Plugin) ingestBatch(ctx context.Context, batch webhookBatch) error {
	for _, m := range batch.messages {
		if err := p.ingest(sdk.WithTenant(ctx, m.tenantID), m); err != nil {
			return err
		}
	}
	for _, u := range batch.statuses {
		if err := p.applyStatus(sdk.WithTenant(ctx, u.tenantID), u); err != nil {
			return err
		}
	}
	return nil
}

// namedNumbers decodes the distinct numbers a delivery names, leaving out any Postgres cannot hold as text.
func namedNumbers(body []byte) ([]string, error) {
	var payload webhookNames
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("whatsapp: decode numbers: %w", err)
	}
	named := map[string]struct{}{}
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			number := change.Value.Metadata.PhoneNumberID
			if number != "" && !strings.ContainsRune(number, 0) {
				named[number] = struct{}{}
			}
		}
	}
	return slices.Collect(maps.Keys(named)), nil
}

// parseWebhook decodes the items of a delivery that sit on an open route, up to limit of them.
func parseWebhook(body []byte, open map[string]numberRoute, limit int) (webhookBatch, error) {
	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return webhookBatch{}, fmt.Errorf("whatsapp: decode payload: %w", err)
	}
	batch := webhookBatch{limit: limit}
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			if route, proven := open[change.Value.Metadata.PhoneNumberID]; proven {
				batch.collect(change, route)
			}
		}
	}
	return batch, nil
}

// full reports whether the batch holds as many items as a delivery may store.
func (b *webhookBatch) full() bool {
	return len(b.messages)+len(b.statuses) >= b.limit
}

// collect appends a change's messages and status updates, stamped with its route, until the batch is full.
func (b *webhookBatch) collect(change webhookChange, route numberRoute) {
	names := senderNames(change.Value.Contacts)
	for _, raw := range change.Value.Messages {
		if b.full() {
			return
		}
		if m, ok := parseMessage(names, raw); ok {
			m.tenantID, m.conversationNumber = route.tenantID, route.number
			b.messages = append(b.messages, m)
		}
	}
	for _, raw := range change.Value.Statuses {
		if b.full() {
			return
		}
		if u, ok := parseStatus(raw); ok {
			u.tenantID, u.number = route.tenantID, route.statusNumber
			b.statuses = append(b.statuses, u)
		}
	}
}

// senderNames returns the display name of each sender in contacts by phone number and by user ID.
func senderNames(contacts []webhookSender) map[string]string {
	names := make(map[string]string, 2*len(contacts))
	for _, sender := range contacts {
		names[sender.WaID] = sender.Profile.Name
		names[sender.UserID] = sender.Profile.Name
	}
	return names
}

// senderOf returns the address a message came from, empty when it has none to store.
func senderOf(m webhookMessage) string {
	if m.From != "" {
		return m.From
	}
	if isUserID(m.FromUserID) {
		return m.FromUserID
	}
	return ""
}

// isUserID reports whether an address has the shape of a WhatsApp user ID, which always holds a period.
func isUserID(address string) bool {
	return strings.Contains(address, ".")
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
		p.events.broadcast(event{
			Conversation: conversationID, Tenant: sdk.TenantOrDefault(ctx),
		})
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
	sender := senderOf(m)
	if sender == "" || m.ID == "" {
		return inboundMessage{}, false
	}
	message := inboundMessage{
		externalID: m.ID,
		sender:     sender,
		senderName: names[sender],
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
//
//nolint:cyclop // flat type dispatch, cognitive complexity is 5
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
	conversationID, arrival, err := p.store.persistInbound(ctx, owner.ID, m)
	if err != nil {
		return err
	}
	if arrival != nil {
		p.events.broadcast(event{
			Conversation: conversationID, Message: arrival, Tenant: sdk.TenantOrDefault(ctx),
		})
		p.publish(ctx, owner, conversationID, m)
		if m.media != nil {
			p.fetcher.poke()
		}
	}
	return nil
}

// publish announces an inbound message unless the host provided no
// publisher.
func (p *Plugin) publish(ctx context.Context, owner sdk.Contact, conversationID uuid.UUID, m inboundMessage) {
	if p.publisher == nil {
		return
	}
	p.publisher.Publish(ctx, eventMessageReceived, map[string]any{
		"conversation_id": conversationID.String(),
		"contact_id":      owner.ID.String(),
		"contact_name":    owner.Name,
		"external_id":     m.externalID,
		"text":            m.content,
	})
}
