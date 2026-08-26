// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"context"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/sdk"
)

// SubscriptionResolvers serves the plugin's Subscription root fields.
type SubscriptionResolvers struct {
	plugin *Plugin
}

// SubscriptionResolvers returns the plugin's Subscription resolver set.
func (p *Plugin) SubscriptionResolvers() SubscriptionResolvers {
	return SubscriptionResolvers{plugin: p}
}

// WhatsAppConversationEvent streams the id of every conversation that changes.
func (s SubscriptionResolvers) WhatsAppConversationEvent(ctx context.Context) (<-chan uuid.UUID, error) {
	conversations := make(chan uuid.UUID, subscriberBuffer)
	go pumpEvents(ctx, s.plugin.events, conversations, func(e event) bool {
		return sendOrDone(ctx, conversations, e.Conversation)
	})
	return conversations, nil
}

// WhatsAppMessageReceived streams the messages arriving on one conversation.
func (s SubscriptionResolvers) WhatsAppMessageReceived(
	ctx context.Context, conversationID uuid.UUID,
) (<-chan *model.WhatsAppMessage, error) {
	messages := make(chan *model.WhatsAppMessage, subscriberBuffer)
	go pumpEvents(ctx, s.plugin.events, messages, func(e event) bool {
		if e.Message == nil || e.Conversation != conversationID {
			return true
		}
		return sendOrDone(ctx, messages, toGraphMessage(conversationID, *e.Message))
	})
	return messages, nil
}

// pumpEvents hands the caller's tenant's events to emit until it stops or the context ends.
func pumpEvents[T any](ctx context.Context, events *broadcaster, out chan T, emit func(event) bool) {
	subscription := events.subscribe(sdk.TenantOrDefault(ctx))
	go func() {
		<-ctx.Done()
		events.unsubscribe(subscription)
	}()
	defer close(out)
	for e := range subscription {
		if !emit(e) {
			return
		}
	}
}

// sendOrDone sends value unless the context ended first.
func sendOrDone[T any](ctx context.Context, out chan<- T, value T) bool {
	select {
	case out <- value:
		return true
	case <-ctx.Done():
		return false
	}
}
