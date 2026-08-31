// SPDX-License-Identifier: Elastic-2.0

// Package graphres resolves the GraphQL schema over the core services.
package graphres

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/internal/task"
	"github.com/gopherium/alphone/internal/tenant"
	"github.com/gopherium/alphone/internal/webhook"
	"github.com/gopherium/alphone/sdk"
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

// TenantStore reads the tenant a caller stands in.
type TenantStore interface {
	TenantForUser(ctx context.Context, userID uuid.UUID) (tenant.Tenant, error)
	TenantsOf(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]uuid.UUID, error)
}

// WebhookStore provides the webhook subscriptions the graph manages.
type WebhookStore interface {
	CreateSubscription(ctx context.Context, sub webhook.Subscription) error
	ListSubscriptionsForUser(ctx context.Context, userID uuid.UUID) ([]webhook.Subscription, error)
	DeleteSubscription(ctx context.Context, userID, id uuid.UUID) error
}

// outranking refuses an actor writing an account of another tenant or holding a capability it lacks.
func (r *Resolver) outranking(ctx context.Context, actor authkit.Identity, target uuid.UUID) error {
	held, err := r.Admin.ListAccounts(ctx)
	if err != nil {
		return err
	}
	for _, account := range held {
		if account.ID != target {
			continue
		}
		placed, err := r.tenantsOf(ctx, []uuid.UUID{actor.ID, account.ID})
		if err != nil {
			return err
		}
		if placedIn(placed, actor.ID) != placedIn(placed, account.ID) {
			return gouncer.ErrUserNotFound
		}
		if !role.Outranks(role.Role(actor.Role), role.Role(account.Role)) {
			return role.ErrBeyondReach
		}
		return nil
	}
	return gouncer.ErrUserNotFound
}

// sharesTenant reports whether the actor and the target stand in one tenant.
func (r *Resolver) sharesTenant(ctx context.Context, actor, target uuid.UUID) (bool, error) {
	placed, err := r.tenantsOf(ctx, []uuid.UUID{actor, target})
	if err != nil {
		return false, err
	}
	return placedIn(placed, actor) == placedIn(placed, target), nil
}

// tenantsOf returns the tenant each named account a row places stands in, the rest absent.
func (r *Resolver) tenantsOf(
	ctx context.Context, ids []uuid.UUID,
) (map[uuid.UUID]uuid.UUID, error) {
	if r.Tenants == nil {
		return nil, nil
	}
	return r.Tenants.TenantsOf(ctx, ids)
}

// placedIn returns the tenant a placement holds for one account, the default when none does.
func placedIn(placed map[uuid.UUID]uuid.UUID, id uuid.UUID) uuid.UUID {
	if standing, ok := placed[id]; ok {
		return standing
	}
	return tenant.DefaultID
}

// TokenStore serves the caller's own API tokens.
type TokenStore interface {
	Create(ctx context.Context, token apitoken.Token) error
	ListForUser(ctx context.Context, userID uuid.UUID) ([]apitoken.Token, error)
	Revoke(ctx context.Context, userID, id uuid.UUID) error
}

// Publisher announces domain events.
type Publisher interface {
	Publish(ctx context.Context, frame event.Frame, data map[string]any)
}

// Broadcaster hands out the event names a user may see.
type Broadcaster interface {
	Subscribe(user, tenant uuid.UUID) chan event.Name
	Unsubscribe(names chan event.Name)
}

// AttemptLimiter budgets failed logins per client key.
type AttemptLimiter interface {
	Check(key string) (bool, time.Duration, error)
	RecordFailure(key string) error
}

// Mailer delivers the invitation and reset mail.
type Mailer interface {
	SendInvite(ctx context.Context, to, name, link string) error
	SendReset(ctx context.Context, to, name, link string) error
}

// AccountReader reads one account by its normalized address.
type AccountReader interface {
	UserByEmail(ctx context.Context, email string) (gouncer.User, error)
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
	// Tenants serves the caller's own tenant.
	Tenants TenantStore
	// Tokens serves the caller's own API tokens.
	Tokens TokenStore
	// Events announces domain events. Nil publishes nothing.
	Events Publisher
	// Live hands subscriptions the frames they may see. Nil serves no subscription.
	Live Broadcaster
	// Auth serves login sessions through the authkit seams.
	Auth *authkit.Handlers
	// Admin serves user administration through the authkit seams.
	Admin *authkit.AdminHandlers
	// Settings reads and writes the caller's stored preferences.
	Settings SettingStore
	// LoginLimiter counts failed logins per client IP.
	LoginLimiter AttemptLimiter
	// Invites serves the invitation and reset flows.
	Invites *authkit.Invites
	// Accounts reads accounts for the mail the flows deliver.
	Accounts AccountReader
	// Mailer delivers the account mail. Nil runs without a mailer.
	Mailer Mailer
	// PublicURL is the address email links lead back to, empty without a mailer.
	PublicURL string
	// TokenLimiter counts token operations per client IP.
	TokenLimiter AttemptLimiter

	// ResetLimiter budgets reset requests per client IP.
	ResetLimiter AttemptLimiter

	// Logger records what a neutral answer hides. Nil discards it.
	Logger *slog.Logger
	// BatchWait bounds the loader batching window. Zero means one millisecond.
	BatchWait time.Duration
}

// publish announces an event in the caller's tenant unless no publisher is wired.
func (r *Resolver) publish(ctx context.Context, frame event.Frame, data map[string]any) {
	if r.Events == nil {
		return
	}
	frame.Tenant = sdk.TenantOrDefault(ctx)
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

// Tenant answers the caller's own tenant.
func (q QueryResolvers) Tenant(ctx context.Context) (*model.Tenant, error) {
	held, err := q.root.Tenants.TenantForUser(ctx, authkit.IdentityFromContext(ctx).ID)
	if err != nil {
		return nil, err
	}
	return &model.Tenant{ID: held.ID, Name: held.Name}, nil
}
