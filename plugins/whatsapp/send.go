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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gopherium/alphone/sdk"
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

// graphError reports a Graph API send rejection with its error code.
type graphError struct {
	Code    int
	Message string
}

// Error formats the rejection.
func (e graphError) Error() string {
	return fmt.Sprintf("graph error %d: %s", e.Code, e.Message)
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
		return "", nil, sendFailure(response.StatusCode, raw)
	}
	id, err := sentMessageID(raw)
	if err != nil {
		return "", nil, err
	}
	return id, raw, nil
}

// sendFailure returns the error a rejected Graph send response carries.
func sendFailure(status int, raw []byte) error {
	var failure struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &failure); err == nil && failure.Error.Code != 0 {
		return fmt.Errorf("whatsapp: send message: %w",
			graphError{Code: failure.Error.Code, Message: failure.Error.Message})
	}
	return fmt.Errorf("whatsapp: send message: status %d", status)
}

// sentMessageID returns the message id in a Graph send response.
func sentMessageID(raw []byte) (string, error) {
	var decoded struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return "", fmt.Errorf("whatsapp: decode send response: %w", err)
	}
	if len(decoded.Messages) == 0 || decoded.Messages[0].ID == "" {
		return "", errors.New("whatsapp: send response carries no message id")
	}
	return decoded.Messages[0].ID, nil
}

// Send service errors.
var (
	errEmptyMessageContent  = errors.New("whatsapp: message content must not be empty")
	errConversationNotFound = errors.New("whatsapp: conversation not found")
)

// sendMessage sends a text message on the conversation, persists it, and broadcasts the event.
func (p *Plugin) sendMessage(ctx context.Context, conversationID uuid.UUID, content string) (messageRow, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return messageRow{}, sdk.GraphError{
			Code: "VALIDATION", Reason: "message_content_required", Err: errEmptyMessageContent,
		}
	}
	to, err := p.store.conversationExternalID(ctx, conversationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return messageRow{}, sdk.GraphError{Code: "NOT_FOUND", Reason: "conversation_not_found", Err: errConversationNotFound}
	}
	if err != nil {
		return messageRow{}, err
	}
	externalID, raw, err := p.sender.sendText(ctx, to, trimmed)
	if err != nil {
		return messageRow{}, upstreamError(err)
	}
	row, err := p.store.appendOutboundMessage(ctx, conversationID, outboundMessage{
		externalID: externalID,
		content:    trimmed,
		sentAt:     time.Now().UTC(),
		raw:        raw,
	})
	if err != nil {
		return messageRow{}, err
	}
	p.events.broadcast(event{Conversation: conversationID, Tenant: sdk.TenantOrDefault(ctx)})
	return row, nil
}

// upstreamError classifies a Cloud API send failure, carrying any rejection code.
func upstreamError(err error) error {
	coded := sdk.GraphError{Code: "UPSTREAM", Reason: "upstream_failed", Err: err}
	var rejection graphError
	if errors.As(err, &rejection) {
		coded.Extensions = map[string]any{"metaCode": rejection.Code}
	}
	return coded
}
