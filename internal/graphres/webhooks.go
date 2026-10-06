// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"context"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/role"
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

// toListedWebhook maps a listed subscription onto its graph model with its owner, without the secret.
func toListedWebhook(listed webhook.Listed) *model.Webhook {
	mapped := toWebhook(listed.Subscription)
	if listed.Owner != nil {
		mapped.Owner = &model.WebhookOwner{ID: listed.Owner.ID, Name: listed.Owner.Name, Email: listed.Owner.Email}
	}
	return mapped
}

// managesWebhooks reports whether identity may manage every webhook of its workspace.
func managesWebhooks(identity authkit.Identity) bool {
	return role.Can(role.Role(identity.Role), role.ManageWebhooks)
}

// Webhooks lists every webhook of the workspace to a holder of manage_webhooks, and the caller's own to anyone else.
func (q QueryResolvers) Webhooks(ctx context.Context) ([]*model.Webhook, error) {
	identity := authkit.IdentityFromContext(ctx)
	var (
		subs []webhook.Listed
		err  error
	)
	if managesWebhooks(identity) {
		subs, err = q.root.Webhooks.ListWorkspaceSubscriptions(ctx)
	} else {
		subs, err = q.root.Webhooks.ListSubscriptionsForUser(ctx, identity.ID)
	}
	if err != nil {
		return nil, err
	}
	listing := make([]*model.Webhook, len(subs))
	for i, sub := range subs {
		listing[i] = toListedWebhook(sub)
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
	if err := webhook.Admit(m.root.WebhookGuard, url); err != nil {
		return nil, err
	}
	if err := m.root.Webhooks.CreateSubscription(ctx, sub); err != nil {
		return nil, err
	}
	owner := &webhook.Owner{ID: identity.ID, Name: identity.Name, Email: identity.Email}
	return &model.CreateWebhookPayload{
		Webhook: toListedWebhook(webhook.Listed{Subscription: sub, Owner: owner}),
		Secret:  sub.Secret,
	}, nil
}

// DeleteWebhook revokes any webhook of the workspace for a holder of manage_webhooks, and only its own for anyone else.
func (m MutationResolvers) DeleteWebhook(ctx context.Context, id uuid.UUID) (bool, error) {
	identity := authkit.IdentityFromContext(ctx)
	var err error
	if managesWebhooks(identity) {
		err = m.root.Webhooks.DeleteWorkspaceSubscription(ctx, id)
	} else {
		err = m.root.Webhooks.DeleteSubscription(ctx, identity.ID, id)
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
