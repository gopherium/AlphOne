// SPDX-License-Identifier: Elastic-2.0

package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
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

// TenantsOf returns the tenant of every named user a row places, the rest absent.
func (s *TenantStore) TenantsOf(
	ctx context.Context, ids []uuid.UUID,
) (map[uuid.UUID]uuid.UUID, error) {
	rows, err := s.queries.TenantsOfUsers(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("read tenants: %w", err)
	}
	placed := make(map[uuid.UUID]uuid.UUID, len(rows))
	for _, row := range rows {
		placed[row.UserID] = row.TenantID
	}
	return placed, nil
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
	return tenantFrom(row.ID, row.Name, row.DeactivatedAt), nil
}

// TenantByID returns the tenant the id names, whether a caller stands in it or not.
func (s *TenantStore) TenantByID(ctx context.Context, id uuid.UUID) (tenant.Tenant, error) {
	row, err := s.queries.TenantByID(ctx, id)
	if err != nil {
		return tenant.Tenant{}, fmt.Errorf("read tenant by id: %w", err)
	}
	return tenantFrom(row.ID, row.Name, row.DeactivatedAt), nil
}

// tenantFrom returns the tenant one stored row describes.
func tenantFrom(id uuid.UUID, name string, deactivatedAt pgtype.Timestamptz) tenant.Tenant {
	return tenant.Tenant{
		ID:            id,
		Name:          name,
		Deactivated:   deactivatedAt.Valid,
		DeactivatedAt: deactivatedAt.Time,
	}
}
