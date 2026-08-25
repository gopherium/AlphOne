// SPDX-License-Identifier: Elastic-2.0

package sdk

import (
	"context"

	"github.com/google/uuid"
)

// DefaultTenantID identifies the tenant every unplaced caller stands in.
var DefaultTenantID = uuid.MustParse("00000000-0000-7000-8000-000000000001")

// tenantKey is the context key carrying the tenant a plugin request serves.
type tenantKey struct{}

// WithTenant returns ctx carrying the tenant a plugin request serves.
func WithTenant(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, tenantKey{}, id)
}

// TenantFromContext returns the tenant a plugin request serves, if the host set one.
func TenantFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(tenantKey{}).(uuid.UUID)
	return id, ok
}

// TenantOrDefault returns the tenant a request serves, the default when the host placed none.
func TenantOrDefault(ctx context.Context) uuid.UUID {
	if id, ok := TenantFromContext(ctx); ok {
		return id
	}
	return DefaultTenantID
}
