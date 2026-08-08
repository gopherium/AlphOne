// SPDX-License-Identifier: Elastic-2.0

// Package graphres resolves the GraphQL schema over the core services.
package graphres

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/task"
	"github.com/gopherium/alphone/internal/webhook"
)

// ContactStore provides the contact reads and writes the graph resolves from.
type ContactStore interface {
	Get(ctx context.Context, id uuid.UUID) (contact.Contact, error)
	ListContacts(
		ctx context.Context, query, digits, afterName string, afterID uuid.UUID, limit int,
	) ([]contact.Contact, error)
	ListContactIdentities(ctx context.Context, contactID uuid.UUID) ([]contact.Identity, error)
	ListByIDs(ctx context.Context, ids []uuid.UUID) ([]contact.Contact, error)
	Create(ctx context.Context, c contact.Contact) error
	CreateContactWithIdentities(ctx context.Context, c contact.Contact, identities []contact.Identity) error
	RenameContact(ctx context.Context, id uuid.UUID, name string) (contact.Contact, error)
	AddIdentity(ctx context.Context, identity contact.Identity) error
	DeleteIdentity(ctx context.Context, contactID, identityID uuid.UUID) error
}

// TaskStore provides the task reads the graph resolves from.
type TaskStore interface {
	Get(ctx context.Context, id uuid.UUID) (task.Task, error)
	ListForDay(
		ctx context.Context, assigneeID uuid.UUID, dueOn time.Time, status string, page task.Page,
	) ([]task.Task, error)
	ListDueBefore(
		ctx context.Context, assigneeID uuid.UUID, dueBefore time.Time, status string, page task.Page,
	) ([]task.Task, error)
	ListForContact(
		ctx context.Context, contactID uuid.UUID, status string, page task.Page,
	) ([]task.Task, error)
	Create(ctx context.Context, t task.Task) (task.Task, bool, error)
	Update(ctx context.Context, t task.Task) (task.Task, error)
}

// WebhookStore provides the webhook subscriptions the graph manages.
type WebhookStore interface {
	CreateSubscription(ctx context.Context, sub webhook.Subscription) error
	ListSubscriptionsForUser(ctx context.Context, userID uuid.UUID) ([]webhook.Subscription, error)
	DeleteSubscription(ctx context.Context, userID, id uuid.UUID) error
}

// Publisher announces domain events.
type Publisher interface {
	Publish(ctx context.Context, frame event.Frame, data map[string]any)
}

// Broadcaster hands out the event names a user may see.
type Broadcaster interface {
	Subscribe(user uuid.UUID) chan event.Name
	Unsubscribe(names chan event.Name)
}

// AttemptLimiter budgets failed logins per client key.
type AttemptLimiter interface {
	Check(key string) (bool, time.Duration, error)
	RecordFailure(key string) error
}

// Resolver is the root resolver serving the core schema.
type Resolver struct {
	// Version is the reported application version.
	Version string
	// Contacts serves contact reads and writes.
	Contacts ContactStore
	// Tasks serves task reads and writes.
	Tasks TaskStore
	// Webhooks serves the webhook subscriptions.
	Webhooks WebhookStore
	// Events announces domain events. Nil publishes nothing.
	Events Publisher
	// Live hands subscriptions the frames they may see. Nil serves no subscription.
	Live Broadcaster
	// Auth serves login sessions through the authkit seams.
	Auth *authkit.Handlers
	// Admin serves user administration through the authkit seams.
	Admin *authkit.AdminHandlers
	// LoginLimiter counts failed logins per client IP.
	LoginLimiter AttemptLimiter
	// BatchWait bounds the loader batching window. Zero means one millisecond.
	BatchWait time.Duration
}

// publish announces an event unless the resolver was built without a publisher.
func (r *Resolver) publish(ctx context.Context, frame event.Frame, data map[string]any) {
	if r.Events == nil {
		return
	}
	r.Events.Publish(ctx, frame, data)
}

// QueryResolvers returns the core Query resolver set.
func (r *Resolver) QueryResolvers() QueryResolvers {
	return QueryResolvers{root: r}
}

// ContactResolvers returns the core Contact resolver set.
func (r *Resolver) ContactResolvers() ContactResolvers {
	return ContactResolvers{root: r}
}

// TaskResolvers returns the core Task resolver set.
func (r *Resolver) TaskResolvers() TaskResolvers {
	return TaskResolvers{root: r}
}

// MutationResolvers returns the core Mutation resolver set.
func (r *Resolver) MutationResolvers() MutationResolvers {
	return MutationResolvers{root: r}
}

// MutationResolvers serves the Mutation root fields.
type MutationResolvers struct {
	root *Resolver
}

// QueryResolvers serves the Query root fields.
type QueryResolvers struct {
	root *Resolver
}

// Version reports the application version.
func (q QueryResolvers) Version(context.Context) (string, error) {
	return q.root.Version, nil
}
