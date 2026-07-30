// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/webhook"
)

// WebhookStore persists the webhook subscriptions the API manages.
type WebhookStore interface {
	CreateSubscription(ctx context.Context, sub webhook.Subscription) error
	ListSubscriptionsForUser(ctx context.Context, userID uuid.UUID) ([]webhook.Subscription, error)
	DeleteSubscription(ctx context.Context, userID, id uuid.UUID) error
}

// webhookResponse is one subscription as the API reports it, without its
// signing secret.
type webhookResponse struct {
	ID        uuid.UUID    `json:"id"`
	URL       string       `json:"url"`
	Events    []event.Name `json:"events"`
	CreatedAt time.Time    `json:"created_at"`
}

// webhookListResponse is a page of subscriptions.
type webhookListResponse struct {
	Webhooks []webhookResponse `json:"webhooks"`
}

// createdWebhookResponse carries the signing secret shown only at creation.
type createdWebhookResponse struct {
	webhookResponse
	Secret string `json:"secret"`
}

// newWebhookResponse maps a subscription onto its API representation.
func newWebhookResponse(sub webhook.Subscription) webhookResponse {
	return webhookResponse{ID: sub.ID, URL: sub.URL, Events: sub.Events, CreatedAt: sub.CreatedAt}
}

// handleWebhookList returns an http.HandlerFunc listing the caller's
// subscriptions.
func (s *server) handleWebhookList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity := authkit.IdentityFromContext(r.Context())
		subs, err := s.webhooks.ListSubscriptionsForUser(r.Context(), identity.ID)
		if err != nil {
			respondDomainError(w, err)
			return
		}
		listing := make([]webhookResponse, 0, len(subs))
		for _, sub := range subs {
			listing = append(listing, newWebhookResponse(sub))
		}
		authkit.Respond(w, http.StatusOK, webhookListResponse{Webhooks: listing})
	}
}

// handleWebhookCreate returns an http.HandlerFunc subscribing an endpoint to
// named events and responding with its signing secret.
func (s *server) handleWebhookCreate() http.HandlerFunc {
	type request struct {
		URL    string       `json:"url"`
		Events []event.Name `json:"events"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := authkit.Decode[request](w, r)
		if err != nil {
			authkit.RespondError(w, http.StatusBadRequest, "malformed json")
			return
		}
		identity := authkit.IdentityFromContext(r.Context())
		sub, err := webhook.NewSubscription(identity.ID, req.URL, req.Events)
		if err != nil {
			respondDomainError(w, err)
			return
		}
		if err := s.webhooks.CreateSubscription(r.Context(), sub); err != nil {
			respondDomainError(w, err)
			return
		}
		authkit.Respond(w, http.StatusCreated, createdWebhookResponse{
			webhookResponse: newWebhookResponse(sub),
			Secret:          sub.Secret,
		})
	}
}

// handleWebhookDelete returns an http.HandlerFunc revoking one of the
// caller's subscriptions.
func (s *server) handleWebhookDelete() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			authkit.RespondError(w, http.StatusBadRequest, "malformed webhook id")
			return
		}
		identity := authkit.IdentityFromContext(r.Context())
		if err := s.webhooks.DeleteSubscription(r.Context(), identity.ID, id); err != nil {
			respondDomainError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
