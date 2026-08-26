// SPDX-License-Identifier: Elastic-2.0

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/postgres/db"
	"github.com/gopherium/alphone/internal/webhook"
	"github.com/gopherium/alphone/sdk"
)

// WebhookStore persists webhook subscriptions and their delivery queue.
type WebhookStore struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

// NewWebhookStore returns a [WebhookStore] backed by pool.
func NewWebhookStore(pool *pgxpool.Pool) *WebhookStore {
	return &WebhookStore{pool: pool, queries: db.New(pool)}
}

// CreateSubscription stores a new subscription.
func (s *WebhookStore) CreateSubscription(ctx context.Context, sub webhook.Subscription) error {
	err := s.queries.CreateWebhookSubscription(ctx, db.CreateWebhookSubscriptionParams{
		ID:        sub.ID,
		UserID:    sub.UserID,
		Url:       sub.URL,
		Events:    eventNamesToText(sub.Events),
		Secret:    sub.Secret,
		CreatedAt: sub.CreatedAt,
		TenantID:  sdk.TenantOrDefault(ctx),
	})
	if err != nil {
		return fmt.Errorf("postgres: create webhook subscription: %w", err)
	}
	return nil
}

// ListSubscriptionsForUser returns the subscriptions of one user, newest
// first.
func (s *WebhookStore) ListSubscriptionsForUser(
	ctx context.Context,
	userID uuid.UUID,
) ([]webhook.Subscription, error) {
	rows, err := s.queries.ListWebhookSubscriptionsForUser(ctx,
		db.ListWebhookSubscriptionsForUserParams{
			UserID: userID, TenantID: sdk.TenantOrDefault(ctx),
		})
	if err != nil {
		return nil, fmt.Errorf("postgres: list webhook subscriptions: %w", err)
	}
	return subscriptionsFromRows(rows), nil
}

// ListSubscriptionsForEvent returns every subscription receiving the named
// event.
func (s *WebhookStore) ListSubscriptionsForEvent(
	ctx context.Context,
	name event.Name,
) ([]webhook.Subscription, error) {
	rows, err := s.queries.ListWebhookSubscriptionsForEvent(ctx,
		db.ListWebhookSubscriptionsForEventParams{
			Column1: string(name), TenantID: sdk.TenantOrDefault(ctx),
		})
	if err != nil {
		return nil, fmt.Errorf("postgres: list webhook subscriptions for event: %w", err)
	}
	return subscriptionsFromRows(rows), nil
}

// DeleteSubscription removes one subscription of userID, or reports
// [webhook.ErrNotFound] when the user owns no such subscription.
func (s *WebhookStore) DeleteSubscription(ctx context.Context, userID, id uuid.UUID) error {
	deleted, err := s.queries.DeleteWebhookSubscription(ctx, db.DeleteWebhookSubscriptionParams{
		ID:       id,
		UserID:   userID,
		TenantID: sdk.TenantOrDefault(ctx),
	})
	if err != nil {
		return fmt.Errorf("postgres: delete webhook subscription: %w", err)
	}
	if deleted == 0 {
		return webhook.ErrNotFound
	}
	return nil
}

// EnqueueDelivery stores a delivery for the worker to pick up.
func (s *WebhookStore) EnqueueDelivery(ctx context.Context, d webhook.Delivery) error {
	err := s.queries.CreateWebhookDelivery(ctx, db.CreateWebhookDeliveryParams{
		ID:             d.ID,
		SubscriptionID: d.SubscriptionID,
		EventID:        d.EventID,
		EventName:      string(d.EventName),
		Payload:        string(d.Payload),
		Attempts:       int32(d.Attempts),
		DeliverAfter:   d.DeliverAfter,
		Status:         d.Status,
		LastError:      optionalText(d.LastError),
		CreatedAt:      d.CreatedAt,
		TenantID:       sdk.TenantOrDefault(ctx),
	})
	if err != nil {
		return fmt.Errorf("postgres: create webhook delivery: %w", err)
	}
	return nil
}

// ClaimDueDeliveries takes up to limit deliveries due at or before due,
// counting an attempt against each and holding them until lease.
func (s *WebhookStore) ClaimDueDeliveries(
	ctx context.Context,
	due, lease time.Time,
	limit int,
) ([]webhook.ClaimedDelivery, error) {
	rows, err := s.queries.ClaimWebhookDeliveries(ctx, db.ClaimWebhookDeliveriesParams{
		Lease:    lease,
		Due:      due,
		RowLimit: int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("postgres: claim webhook deliveries: %w", err)
	}
	deliveries := make([]webhook.ClaimedDelivery, 0, len(rows))
	for _, row := range rows {
		deliveries = append(deliveries, webhook.ClaimedDelivery{
			Delivery: webhook.Delivery{
				ID:             row.ID,
				SubscriptionID: row.SubscriptionID,
				EventID:        row.EventID,
				EventName:      event.Name(row.EventName),
				Payload:        []byte(row.Payload),
				Attempts:       int(row.Attempts),
				DeliverAfter:   row.DeliverAfter,
				Status:         row.Status,
				LastError:      row.LastError.String,
				CreatedAt:      row.CreatedAt,
			},
			URL:    row.Url,
			Secret: row.Secret,
		})
	}
	return deliveries, nil
}

// SettleDelivery records the outcome of one delivery attempt.
func (s *WebhookStore) SettleDelivery(
	ctx context.Context,
	id uuid.UUID,
	status string,
	deliverAfter time.Time,
	lastError string,
) error {
	err := s.queries.SettleWebhookDelivery(ctx, db.SettleWebhookDeliveryParams{
		ID:           id,
		Status:       status,
		DeliverAfter: deliverAfter,
		LastError:    optionalText(lastError),
	})
	if err != nil {
		return fmt.Errorf("postgres: settle webhook delivery: %w", err)
	}
	return nil
}

// subscriptionsFromRows maps stored rows onto domain subscriptions.
func subscriptionsFromRows(rows []db.CoreWebhookSubscription) []webhook.Subscription {
	subs := make([]webhook.Subscription, 0, len(rows))
	for _, row := range rows {
		subs = append(subs, webhook.Subscription{
			ID:        row.ID,
			UserID:    row.UserID,
			URL:       row.Url,
			Events:    eventNamesFromText(row.Events),
			Secret:    row.Secret,
			CreatedAt: row.CreatedAt,
		})
	}
	return subs
}

// eventNamesToText maps event names onto the stored text array.
func eventNamesToText(names []event.Name) []string {
	stored := make([]string, 0, len(names))
	for _, name := range names {
		stored = append(stored, string(name))
	}
	return stored
}

// eventNamesFromText maps a stored text array onto event names.
func eventNamesFromText(stored []string) []event.Name {
	names := make([]event.Name, 0, len(stored))
	for _, value := range stored {
		names = append(names, event.Name(value))
	}
	return names
}
