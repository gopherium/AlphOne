// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/internal/apitoken"
)

// bearerScheme prefixes the credential in an Authorization header.
const bearerScheme = "Bearer "

// TokenStore reads the API tokens a bearer credential resolves to.
type TokenStore interface {
	ByHash(ctx context.Context, hash string) (apitoken.Token, error)
	TouchLastUsed(ctx context.Context, id uuid.UUID, at time.Time) error
}

// UserStore persists users for login, administration, and token lookup.
type UserStore interface {
	authkit.AdminStore
	UserByID(ctx context.Context, id uuid.UUID) (gouncer.User, error)
}

// requireIdentity admits requests carrying either a usable API token or a
// login session, passing the authenticated identity down the chain.
func (s *server) requireIdentity(next http.Handler) http.Handler {
	session := s.auth.RequireSession(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secret, ok := s.bearerSecret(r)
		if !ok {
			session.ServeHTTP(w, r)
			return
		}
		identity, err := s.identityForToken(r.Context(), secret)
		if errors.Is(err, apitoken.ErrNotFound) || errors.Is(err, gouncer.ErrUserNotFound) {
			authkit.RespondError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		if err != nil {
			authkit.RespondError(w, http.StatusInternalServerError, "internal error")
			return
		}
		next.ServeHTTP(w, r.WithContext(authkit.WithIdentity(r.Context(), identity)))
	})
}

// bearerSecret returns the credential of an Authorization Bearer header.
func (s *server) bearerSecret(r *http.Request) (string, bool) {
	if s.tokens == nil {
		return "", false
	}
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, bearerScheme) {
		return "", false
	}
	secret := strings.TrimSpace(strings.TrimPrefix(header, bearerScheme))
	return secret, secret != ""
}

// identityForToken resolves the enabled user a token secret acts as.
func (s *server) identityForToken(ctx context.Context, secret string) (authkit.Identity, error) {
	token, err := s.tokens.ByHash(ctx, apitoken.HashSecret(secret))
	if err != nil {
		return authkit.Identity{}, err
	}
	user, err := s.users.UserByID(ctx, token.UserID)
	if err != nil {
		return authkit.Identity{}, err
	}
	if user.Disabled {
		return authkit.Identity{}, apitoken.ErrNotFound
	}
	_ = s.tokens.TouchLastUsed(ctx, token.ID, time.Now().UTC())
	return authkit.Identity{ID: user.ID, Email: user.Email, Name: user.Name}, nil
}
