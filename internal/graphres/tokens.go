// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/internal/apitoken"
)

// toAPIToken maps a stored token onto its graph model, without the hash.
func toAPIToken(stored apitoken.Token) *model.APIToken {
	answered := &model.APIToken{
		ID:        stored.ID,
		Name:      stored.Name,
		Scopes:    stored.Scopes,
		CreatedAt: stored.CreatedAt,
	}
	if !stored.LastUsedAt.IsZero() {
		lastUsed := stored.LastUsedAt
		answered.LastUsedAt = &lastUsed
	}
	if !stored.ExpiresAt.IsZero() {
		expires := stored.ExpiresAt
		answered.ExpiresAt = &expires
	}
	return answered
}

// lifetimeOf returns the lifetime the given days ask for, never when unasked.
func lifetimeOf(days *int) (time.Duration, error) {
	if days == nil {
		return apitoken.Never, nil
	}
	return apitoken.LifetimeOfDays(*days)
}

// APITokens lists the caller's own API tokens, secrets excluded.
func (q QueryResolvers) APITokens(ctx context.Context) ([]*model.APIToken, error) {
	identity := authkit.IdentityFromContext(ctx)
	stored, err := q.root.Tokens.ListForUser(ctx, identity.ID)
	if err != nil {
		return nil, err
	}
	listing := make([]*model.APIToken, len(stored))
	for i, token := range stored {
		listing[i] = toAPIToken(token)
	}
	return listing, nil
}

// APITokenCreate mints a token for the caller, answering its secret exactly once.
func (m MutationResolvers) APITokenCreate(
	ctx context.Context, name string, scopes []string, ttlDays *int,
) (*model.APITokenSecret, error) {
	lifetime, err := lifetimeOf(ttlDays)
	if err != nil {
		return nil, err
	}
	identity := authkit.IdentityFromContext(ctx)
	minted, err := apitoken.Mint(identity.ID, name, apitoken.Scopes(scopes), lifetime)
	if err != nil {
		return nil, err
	}
	if err := m.root.Tokens.Create(ctx, minted.Token); err != nil {
		return nil, err
	}
	return &model.APITokenSecret{Token: toAPIToken(minted.Token), Secret: minted.Secret}, nil
}

// APITokenRevoke stops one of the caller's own tokens.
func (m MutationResolvers) APITokenRevoke(ctx context.Context, id uuid.UUID) (bool, error) {
	identity := authkit.IdentityFromContext(ctx)
	if err := m.root.Tokens.Revoke(ctx, identity.ID, id); err != nil {
		return false, err
	}
	return true, nil
}
