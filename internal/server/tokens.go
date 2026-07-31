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

// originTokenPrefix namespaces attribution stamped from an API token.
const originTokenPrefix = "token:"

// originKey is the context key carrying the attribution of a request credential.
type originKey struct{}

// withTokenOrigin returns ctx carrying the attribution of the named token.
func withTokenOrigin(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, originKey{}, originTokenPrefix+name)
}

// credentialOrigin returns the attribution of the request credential, or empty.
func credentialOrigin(ctx context.Context) string {
	origin, _ := ctx.Value(originKey{}).(string)
	return origin
}

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
		identity, token, err := s.identityForToken(r.Context(), secret)
		if errors.Is(err, apitoken.ErrNotFound) || errors.Is(err, gouncer.ErrUserNotFound) {
			authkit.RespondError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		if err != nil {
			authkit.RespondError(w, http.StatusInternalServerError, "internal error")
			return
		}
		ctx := authkit.WithIdentity(r.Context(), identity)
		ctx = withTokenOrigin(ctx, token.Name)
		next.ServeHTTP(w, r.WithContext(ctx))
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

// identityForToken resolves the enabled user a token secret acts as, and the token itself.
func (s *server) identityForToken(ctx context.Context, secret string) (authkit.Identity, apitoken.Token, error) {
	token, err := s.tokens.ByHash(ctx, apitoken.HashSecret(secret))
	if err != nil {
		return authkit.Identity{}, apitoken.Token{}, err
	}
	user, err := s.users.UserByID(ctx, token.UserID)
	if err != nil {
		return authkit.Identity{}, apitoken.Token{}, err
	}
	if user.Disabled {
		return authkit.Identity{}, apitoken.Token{}, apitoken.ErrNotFound
	}
	_ = s.tokens.TouchLastUsed(ctx, token.ID, time.Now().UTC())
	return authkit.Identity{ID: user.ID, Email: user.Email, Name: user.Name}, token, nil
}
