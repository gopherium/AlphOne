// SPDX-License-Identifier: Elastic-2.0

package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/postgres/db"
	"github.com/gopherium/alphone/internal/tenant"
)

// Onboarding invites an account and places it in a tenant as one transaction.
type Onboarding struct {
	pool   *pgxpool.Pool
	users  *authkitpg.UserStore
	config authkit.InvitesConfig
}

// NewOnboarding returns an Onboarding inviting under config over pool.
func NewOnboarding(pool *pgxpool.Pool, config authkit.InvitesConfig) *Onboarding {
	return &Onboarding{pool: pool, users: authkitpg.NewUserStore(pool), config: config}
}

// InviteInto creates the invited account, its token and its membership in tenantID as one transaction.
func (o *Onboarding) InviteInto(
	ctx context.Context, tenantID uuid.UUID, email, name, role string,
) (gouncer.Token, error) {
	tx, err := o.pool.Begin(ctx)
	if err != nil {
		return gouncer.Token{}, fmt.Errorf("invite: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	scoped := o.config
	scoped.Store = o.users.Within(tx)
	token, err := authkit.NewInvites(scoped).Invite(ctx, email, name, role)
	if err != nil {
		return gouncer.Token{}, err
	}
	if tenantID != tenant.DefaultID {
		placement := db.PlaceMemberParams{UserID: token.UserID, TenantID: tenantID}
		if err := db.New(tx).PlaceMember(ctx, placement); err != nil {
			return gouncer.Token{}, fmt.Errorf("place member: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return gouncer.Token{}, fmt.Errorf("invite: %w", err)
	}
	return token, nil
}
