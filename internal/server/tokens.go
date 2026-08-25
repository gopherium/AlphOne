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
	"github.com/gopherium/alphone/internal/credential"
	"github.com/gopherium/alphone/internal/tenant"
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

// TenantStore resolves the tenant a caller stands in.
type TenantStore interface {
	TenantForUser(ctx context.Context, userID uuid.UUID) (tenant.Tenant, error)
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
		s.identifyBearer(w, r, next, secret)
	})
}

// identifyIdentity resolves any presented credential without requiring one,
// leaving the request anonymous when none is usable.
func (s *server) identifyIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if secret, ok := s.bearerSecret(r); ok {
			s.identifyBearer(w, r, next, secret)
			return
		}
		next.ServeHTTP(w, r.WithContext(s.sessionContext(r)))
	})
}

// identifyBearer serves the request as the token's user, rejecting unusable tokens.
func (s *server) identifyBearer(w http.ResponseWriter, r *http.Request, next http.Handler, secret string) {
	ctx, err := s.identityForToken(r.Context(), secret)
	if isUnusableToken(err) {
		authkit.RespondError(w, http.StatusUnauthorized, authkit.ErrorResponse{Message: "invalid token"})
		return
	}
	if err != nil {
		authkit.RespondError(w, http.StatusInternalServerError, authkit.ErrorResponse{Message: "internal error"})
		return
	}
	next.ServeHTTP(w, r.WithContext(ctx))
}

// isUnusableToken reports whether err means the presented token cannot act.
func isUnusableToken(err error) bool {
	return errors.Is(err, apitoken.ErrNotFound) ||
		errors.Is(err, apitoken.ErrExpired) ||
		errors.Is(err, gouncer.ErrUserNotFound)
}

// sessionContext returns the request context with the session identity when one resolves.
func (s *server) sessionContext(r *http.Request) context.Context {
	cookie, err := r.Cookie(s.auth.CookieName())
	if err != nil {
		return r.Context()
	}
	identity, err := s.auth.SessionIdentity(r.Context(), cookie.Value)
	if err != nil {
		return r.Context()
	}
	return authkit.WithIdentity(r.Context(), identity)
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

// identityForToken returns ctx carrying the enabled user a live token secret acts as.
func (s *server) identityForToken(ctx context.Context, secret string) (context.Context, error) {
	token, err := s.tokens.ByHash(ctx, apitoken.HashSecret(secret))
	if err != nil {
		return ctx, err
	}
	if token.Expired(time.Now().UTC()) {
		return ctx, apitoken.ErrExpired
	}
	user, err := s.users.UserByID(ctx, token.UserID)
	if err != nil {
		return ctx, err
	}
	if user.Disabled {
		return ctx, apitoken.ErrNotFound
	}
	_ = s.tokens.TouchLastUsed(ctx, token.ID, time.Now().UTC())
	stamped := authkit.WithIdentity(ctx, authkit.Identity{
		ID:    user.ID,
		Email: user.Email,
		Name:  user.Name,
		Role:  user.Role,
	})
	return credential.WithToken(stamped, credential.Token{
		ID:     token.ID,
		Name:   token.Name,
		Scopes: token.Scopes,
	}), nil
}
