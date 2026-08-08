// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/vikstrous/dataloadgen"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/sdk"
)

// Graph resolver errors.
var (
	errInvalidListLimit    = errors.New("whatsapp: limit must be between 1 and 200")
	errMissingContactStash = errors.New("whatsapp: conversation carries no contact")
)

// graphListLimit resolves a limit argument against the REST list bounds.
func graphListLimit(limit *int) (int, error) {
	if limit == nil {
		return defaultListLimit, nil
	}
	if *limit < 1 || *limit > maxListLimit {
		return 0, sdk.GraphError{Code: "VALIDATION", Err: errInvalidListLimit}
	}
	return *limit, nil
}

// toGraphConversation maps a conversation row onto its graph model.
func toGraphConversation(row conversationRow) *model.WhatsAppConversation {
	return &model.WhatsAppConversation{
		ID:                 row.ID,
		ExternalID:         row.ExternalID,
		Status:             row.Status,
		LastActivityAt:     row.LastActivityAt.UTC(),
		LastMessagePreview: row.LastMessagePreview,
		Contact:            &model.Contact{ID: row.ContactID, Name: row.ContactName},
	}
}

// toGraphMessage maps a message row onto its graph model.
func toGraphMessage(conversationID uuid.UUID, row messageRow) *model.WhatsAppMessage {
	message := &model.WhatsAppMessage{
		ID:           row.ID,
		ExternalID:   row.ExternalID,
		Direction:    row.Direction,
		Content:      row.Content,
		ContentType:  row.ContentType,
		SentAt:       row.SentAt.UTC(),
		Status:       row.Status,
		StatusDetail: row.StatusDetail,
	}
	if row.MediaStatus != nil {
		message.Media = toGraphMedia(conversationID, row)
	}
	return message
}

// toGraphMedia maps a message row's media columns onto the media model.
func toGraphMedia(conversationID uuid.UUID, row messageRow) *model.WhatsAppMedia {
	media := &model.WhatsAppMedia{
		Status:   *row.MediaStatus,
		Filename: row.MediaFilename,
		DownloadPath: fmt.Sprintf(
			"/api/plugins/whatsapp/conversations/%s/messages/%s/media", conversationID, row.ID,
		),
	}
	if row.MediaMimeType != nil {
		media.MimeType = *row.MediaMimeType
	}
	if row.MediaFileSize != nil {
		size := int(*row.MediaFileSize)
		media.FileSize = &size
	}
	if row.MediaVoice != nil {
		media.Voice = *row.MediaVoice
	}
	if row.MediaAnimated != nil {
		media.Animated = *row.MediaAnimated
	}
	return media
}

// QueryResolvers serves the plugin's Query root fields.
type QueryResolvers struct {
	plugin *Plugin
}

// QueryResolvers returns the plugin's Query resolver set.
func (p *Plugin) QueryResolvers() QueryResolvers {
	return QueryResolvers{plugin: p}
}

// WhatsAppConversations lists the conversations, most recently active first.
func (q QueryResolvers) WhatsAppConversations(
	ctx context.Context, limit *int,
) ([]*model.WhatsAppConversation, error) {
	bounded, err := graphListLimit(limit)
	if err != nil {
		return nil, err
	}
	rows, err := q.plugin.store.listConversations(ctx, bounded)
	if err != nil {
		return nil, err
	}
	conversations := make([]*model.WhatsAppConversation, len(rows))
	for i, row := range rows {
		conversations[i] = toGraphConversation(row)
	}
	return conversations, nil
}

// WhatsAppConversation reads one conversation, or nothing when it is unknown.
func (q QueryResolvers) WhatsAppConversation(
	ctx context.Context, id uuid.UUID,
) (*model.WhatsAppConversation, error) {
	row, err := q.plugin.store.getConversation(ctx, id)
	if err != nil {
		return nil, err
	}
	if row.ID == uuid.Nil {
		return nil, nil
	}
	return toGraphConversation(row), nil
}

// conversationsLoaderKey keys the conversations by contact loader in the request scope.
type conversationsLoaderKey struct{}

// conversationsLoader returns the request's conversations by contact loader.
func (p *Plugin) conversationsLoader(
	ctx context.Context,
) (*dataloadgen.Loader[uuid.UUID, []conversationRow], error) {
	return sdk.ScopedValue(ctx, conversationsLoaderKey{},
		func() *dataloadgen.Loader[uuid.UUID, []conversationRow] {
			return dataloadgen.NewLoader(p.fetchConversations, dataloadgen.WithWait(time.Millisecond))
		})
}

// fetchConversations loads a batch of conversation lists by contact id.
func (p *Plugin) fetchConversations(
	ctx context.Context, contactIDs []uuid.UUID,
) ([][]conversationRow, []error) {
	results := make([][]conversationRow, len(contactIDs))
	errs := make([]error, len(contactIDs))
	rows, err := p.store.listConversationsByContactIDs(ctx, contactIDs)
	if err != nil {
		for i := range errs {
			errs[i] = err
		}
		return results, errs
	}
	byContact := map[uuid.UUID][]conversationRow{}
	for _, row := range rows {
		byContact[row.ContactID] = append(byContact[row.ContactID], row)
	}
	for i, contactID := range contactIDs {
		results[i] = byContact[contactID]
	}
	return results, errs
}

// ContactResolvers serves the plugin's fields on the core Contact type.
type ContactResolvers struct {
	plugin *Plugin
}

// ContactResolvers returns the plugin's Contact resolver set.
func (p *Plugin) ContactResolvers() ContactResolvers {
	return ContactResolvers{plugin: p}
}

// WhatsAppConversations lists the contact's conversations through the request loader.
func (c ContactResolvers) WhatsAppConversations(
	ctx context.Context, obj *model.Contact,
) ([]*model.WhatsAppConversation, error) {
	loader, err := c.plugin.conversationsLoader(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := loader.Load(ctx, obj.ID)
	if err != nil {
		return nil, err
	}
	conversations := make([]*model.WhatsAppConversation, len(rows))
	for i, row := range rows {
		conversations[i] = toGraphConversation(row)
	}
	return conversations, nil
}

// WhatsAppConversationResolvers serves the conversation's resolver backed fields.
//
//nolint:revive // the wiring names resolver sets after the GraphQL type, whose prefix is the plugin's own name
type WhatsAppConversationResolvers struct {
	plugin *Plugin
}

// WhatsAppConversationResolvers returns the plugin's conversation resolver set.
func (p *Plugin) WhatsAppConversationResolvers() WhatsAppConversationResolvers {
	return WhatsAppConversationResolvers{plugin: p}
}

// Contact reports the conversation's contact from the listing join.
func (w WhatsAppConversationResolvers) Contact(
	_ context.Context, obj *model.WhatsAppConversation,
) (*model.Contact, error) {
	if obj.Contact == nil {
		return nil, errMissingContactStash
	}
	return obj.Contact, nil
}

// Messages lists the conversation's messages, oldest first.
func (w WhatsAppConversationResolvers) Messages(
	ctx context.Context, obj *model.WhatsAppConversation, limit *int,
) ([]*model.WhatsAppMessage, error) {
	bounded, err := graphListLimit(limit)
	if err != nil {
		return nil, err
	}
	rows, err := w.plugin.store.listMessages(ctx, obj.ID, bounded)
	if err != nil {
		return nil, err
	}
	messages := make([]*model.WhatsAppMessage, len(rows))
	for i, row := range rows {
		messages[i] = toGraphMessage(obj.ID, row)
	}
	return messages, nil
}

// MutationResolvers serves the plugin's Mutation root fields.
type MutationResolvers struct {
	plugin *Plugin
}

// MutationResolvers returns the plugin's Mutation resolver set.
func (p *Plugin) MutationResolvers() MutationResolvers {
	return MutationResolvers{plugin: p}
}

// WhatsAppSendMessage sends a text message on the conversation.
func (m MutationResolvers) WhatsAppSendMessage(
	ctx context.Context, conversationID uuid.UUID, content string,
) (*model.WhatsAppMessage, error) {
	row, err := m.plugin.sendMessage(ctx, conversationID, content)
	if err != nil {
		return nil, err
	}
	return toGraphMessage(conversationID, row), nil
}
