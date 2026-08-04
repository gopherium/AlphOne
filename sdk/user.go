// SPDX-License-Identifier: Elastic-2.0

package sdk

import (
	"context"

	"github.com/google/uuid"
)

// userKey is the context key carrying the user a plugin request acts as.
type userKey struct{}

// WithUser returns ctx carrying the user a plugin request acts as.
func WithUser(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, userKey{}, id)
}

// UserFromContext returns the user a plugin request acts as, if the host set one.
func UserFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(userKey{}).(uuid.UUID)
	return id, ok
}
