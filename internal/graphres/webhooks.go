// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"context"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/webhook"
)

// toWebhook maps a subscription onto its graph model, without the secret.
func toWebhook(sub webhook.Subscription) *model.Webhook {
	events := make([]string, len(sub.Events))
	for i, name := range sub.Events {
		events[i] = string(name)
	}
	return &model.Webhook{ID: sub.ID, URL: sub.URL, Events: events, CreatedAt: sub.CreatedAt}
}

// Webhooks lists the caller's webhook subscriptions.
func (q QueryResolvers) Webhooks(ctx context.Context) ([]*model.Webhook, error) {
	identity := authkit.IdentityFromContext(ctx)
	subs, err := q.root.Webhooks.ListSubscriptionsForUser(ctx, identity.ID)
	if err != nil {
		return nil, err
	}
	listing := make([]*model.Webhook, len(subs))
	for i, sub := range subs {
		listing[i] = toWebhook(sub)
	}
	return listing, nil
}

// CreateWebhook subscribes an endpoint, answering the signing secret exactly once.
func (m MutationResolvers) CreateWebhook(
	ctx context.Context, url string, events []string,
) (*model.CreateWebhookPayload, error) {
	names := make([]event.Name, len(events))
	for i, name := range events {
		names[i] = event.Name(name)
	}
	identity := authkit.IdentityFromContext(ctx)
	sub, err := webhook.NewSubscription(identity.ID, url, names)
	if err != nil {
		return nil, err
	}
	if err := m.root.Webhooks.CreateSubscription(ctx, sub); err != nil {
		return nil, err
	}
	return &model.CreateWebhookPayload{Webhook: toWebhook(sub), Secret: sub.Secret}, nil
}

// DeleteWebhook revokes one of the caller's subscriptions.
func (m MutationResolvers) DeleteWebhook(ctx context.Context, id uuid.UUID) (bool, error) {
	identity := authkit.IdentityFromContext(ctx)
	if err := m.root.Webhooks.DeleteSubscription(ctx, identity.ID, id); err != nil {
		return false, err
	}
	return true, nil
}
