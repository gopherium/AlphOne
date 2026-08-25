// SPDX-License-Identifier: Elastic-2.0

package sdk

import (
	"context"

	"github.com/google/uuid"
)

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
