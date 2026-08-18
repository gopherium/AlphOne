// SPDX-License-Identifier: Elastic-2.0

package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/gouncer"

	"github.com/gopherium/alphone/internal/postgres/db"
	"github.com/gopherium/alphone/internal/role"
)

// ErrLastAdmin reports a change that would leave the deployment with no enabled admin.
var ErrLastAdmin = role.ErrLastAdmin

// grantKnownUser stands one user in a tier only when the deployment holds that user.
const grantKnownUser = `INSERT INTO core.user_roles (user_id, role)
SELECT $1, $2 WHERE EXISTS (SELECT 1 FROM auth.users WHERE id = $1)
ON CONFLICT (user_id) DO UPDATE SET role = EXCLUDED.role`

// holdAdmins locks every admin row in a fixed order so two unseatings cannot pass each other.
const holdAdmins = "SELECT user_id FROM core.user_roles WHERE role = 'admin' ORDER BY user_id FOR UPDATE"

// guardedDisable bars one user from logging in only while the deployment keeps an enabled admin,
// answering whether the user is known and whether it was barred.
const guardedDisable = `WITH barred AS (
	UPDATE auth.users SET disabled = true
	WHERE id = $1 AND (
		NOT EXISTS (SELECT 1 FROM core.user_roles held WHERE held.user_id = $1 AND held.role = 'admin')
		OR EXISTS (
			SELECT 1 FROM core.user_roles other
			JOIN auth.users u ON u.id = other.user_id
			WHERE other.role = 'admin' AND other.user_id <> $1 AND NOT u.disabled
		)
	)
	RETURNING id
)
SELECT EXISTS (SELECT 1 FROM auth.users WHERE id = $1), EXISTS (SELECT 1 FROM barred)`

// guardedDemote stands one user below admin only while another enabled admin stands,
// answering whether the tier was stored.
const guardedDemote = `WITH stood AS (
	INSERT INTO core.user_roles (user_id, role) VALUES ($1, $2)
	ON CONFLICT (user_id) DO UPDATE SET role = EXCLUDED.role
	WHERE EXISTS (
		SELECT 1 FROM core.user_roles other
		JOIN auth.users u ON u.id = other.user_id
		WHERE other.role = 'admin' AND other.user_id <> $1 AND NOT u.disabled
	)
	RETURNING user_id
)
SELECT EXISTS (SELECT 1 FROM stood)`

// RoleStore persists the tier every user stands in.
type RoleStore struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

// NewRoleStore returns a [RoleStore] backed by pool.
func NewRoleStore(pool *pgxpool.Pool) *RoleStore {
	return &RoleStore{pool: pool, queries: db.New(pool)}
}

// RoleOf returns the tier a user stands in, member when it holds no row.
func (s *RoleStore) RoleOf(ctx context.Context, userID uuid.UUID) (role.Role, error) {
	stored, err := s.queries.UserRole(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return role.Member, nil
	}
	if err != nil {
		return role.Member, fmt.Errorf("postgres: read user role: %w", err)
	}
	return role.Of(stored), nil
}

// RolesOf returns the tier each named user stands in, member for those holding no row.
func (s *RoleStore) RolesOf(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]role.Role, error) {
	tiers := make(map[uuid.UUID]role.Role, len(userIDs))
	for _, id := range userIDs {
		tiers[id] = role.Member
	}
	rows, err := s.queries.UserRoles(ctx, userIDs)
	if err != nil {
		return nil, fmt.Errorf("postgres: read user roles: %w", err)
	}
	for _, row := range rows {
		tiers[row.UserID] = role.Of(row.Role)
	}
	return tiers, nil
}

// Grant stores the tier a user stands in, refusing to unseat the last enabled admin.
func (s *RoleStore) Grant(ctx context.Context, userID uuid.UUID, tier role.Role) error {
	if tier != role.Admin {
		return s.demote(ctx, userID, tier)
	}
	tag, err := s.pool.Exec(ctx, grantKnownUser, userID, tier.String())
	if err != nil {
		return fmt.Errorf("postgres: grant user role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return gouncer.ErrUserNotFound
	}
	return nil
}

// unseating runs one write that may remove admin cover behind a lock over every admin row.
func (s *RoleStore) unseating(ctx context.Context, write func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, holdAdmins); err != nil {
		return err
	}
	if err := write(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Disable bars one user from logging in and ends its sessions, refusing to unseat the last enabled admin.
func (s *RoleStore) Disable(ctx context.Context, userID uuid.UUID) error {
	var known, barred bool
	err := s.unseating(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, guardedDisable, userID).Scan(&known, &barred); err != nil {
			return err
		}
		if !barred {
			return nil
		}
		_, err := tx.Exec(ctx, "DELETE FROM auth.sessions WHERE user_id = $1", userID)
		return err
	})
	if err != nil {
		return fmt.Errorf("postgres: disable user: %w", err)
	}
	if !known {
		return gouncer.ErrUserNotFound
	}
	if !barred {
		return ErrLastAdmin
	}
	return nil
}

// demote stores a tier below admin only while another enabled admin stands.
func (s *RoleStore) demote(ctx context.Context, userID uuid.UUID, tier role.Role) error {
	var stood bool
	err := s.unseating(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, guardedDemote, userID, tier.String()).Scan(&stood)
	})
	if err != nil {
		return fmt.Errorf("postgres: demote user: %w", err)
	}
	if !stood {
		return ErrLastAdmin
	}
	return nil
}
