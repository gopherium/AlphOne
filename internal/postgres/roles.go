// SPDX-License-Identifier: Elastic-2.0

package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/role"
)

// ErrLastAdmin reports a change that would leave the deployment with no enabled admin.
var ErrLastAdmin = errors.New("postgres: last admin")

// RoleStore persists the tier every user stands in.
type RoleStore struct {
	pool *pgxpool.Pool
}

// NewRoleStore returns a [RoleStore] backed by pool.
func NewRoleStore(pool *pgxpool.Pool) *RoleStore {
	return &RoleStore{pool: pool}
}

// RoleOf returns the tier a user stands in, member when it holds no row.
func (s *RoleStore) RoleOf(ctx context.Context, userID uuid.UUID) (role.Role, error) {
	var stored string
	err := s.pool.QueryRow(ctx,
		"SELECT role FROM core.user_roles WHERE user_id = $1", userID).Scan(&stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return role.Member, nil
	}
	if err != nil {
		return role.Member, fmt.Errorf("postgres: read user role: %w", err)
	}
	return role.Of(stored), nil
}

// Grant stores the tier a user stands in, refusing to unseat the last enabled admin.
func (s *RoleStore) Grant(ctx context.Context, userID uuid.UUID, tier role.Role) error {
	if tier != role.Admin {
		return s.demote(ctx, userID, tier)
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO core.user_roles (user_id, role) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET role = EXCLUDED.role`, userID, tier.String())
	if err != nil {
		return fmt.Errorf("postgres: grant user role: %w", err)
	}
	return nil
}

// demote stores a tier below admin only while another enabled admin stands.
func (s *RoleStore) demote(ctx context.Context, userID uuid.UUID, tier role.Role) error {
	tag, err := s.pool.Exec(ctx,
		`INSERT INTO core.user_roles (user_id, role) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET role = EXCLUDED.role
		WHERE EXISTS (
			SELECT 1 FROM core.user_roles other
			JOIN auth.users u ON u.id = other.user_id
			WHERE other.role = 'admin' AND other.user_id <> $1 AND NOT u.disabled
		)`, userID, tier.String())
	if err != nil {
		return fmt.Errorf("postgres: demote user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrLastAdmin
	}
	return nil
}
