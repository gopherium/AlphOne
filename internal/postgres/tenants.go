// SPDX-License-Identifier: Elastic-2.0

package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/postgres/db"
	"github.com/gopherium/alphone/internal/tenant"
)

// TenantStore reads the tenant a user stands in.
type TenantStore struct {
	queries *db.Queries
}

// NewTenantStore returns a TenantStore backed by pool.
func NewTenantStore(pool *pgxpool.Pool) *TenantStore {
	return &TenantStore{queries: db.New(pool)}
}

// TenantForUser returns the user's tenant, the default when no row places them.
func (s *TenantStore) TenantForUser(ctx context.Context, userID uuid.UUID) (tenant.Tenant, error) {
	row, err := s.queries.TenantForUser(ctx, db.TenantForUserParams{
		UserID:    userID,
		DefaultID: tenant.DefaultID,
	})
	if err != nil {
		return tenant.Tenant{}, fmt.Errorf("read tenant: %w", err)
	}
	return tenant.Tenant{ID: row.ID, Name: row.Name}, nil
}
