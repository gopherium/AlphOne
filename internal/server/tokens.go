// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/credential"
	"github.com/gopherium/alphone/internal/role"
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

// RoleStore reads the tier a user stands in.
type RoleStore interface {
	RoleOf(ctx context.Context, userID uuid.UUID) (role.Role, error)
}

// withRole returns ctx carrying the tier its identity stands in.
func (s *server) withRole(ctx context.Context, userID uuid.UUID) (context.Context, error) {
	if s.roles == nil {
		return ctx, nil
	}
	tier, err := s.roles.RoleOf(ctx, userID)
	if err != nil {
		return ctx, fmt.Errorf("server: read user role: %w", err)
	}
	return credential.WithRole(ctx, tier), nil
}

// requireIdentity admits requests carrying either a usable API token or a
// login session, passing the authenticated identity down the chain.
func (s *server) requireIdentity(next http.Handler) http.Handler {
	session := s.auth.RequireSession(s.roleStamped(next))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secret, ok := s.bearerSecret(r)
		if !ok {
			session.ServeHTTP(w, r)
			return
		}
		s.identifyBearer(w, r, next, secret)
	})
}

// roleStamped serves the request carrying the tier its session identity stands in.
func (s *server) roleStamped(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity := authkit.IdentityFromContext(r.Context())
		ctx, err := s.withRole(r.Context(), identity.ID)
		if err != nil {
			authkit.RespondError(w, http.StatusInternalServerError, "internal error")
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
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
		authkit.RespondError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	if err != nil {
		authkit.RespondError(w, http.StatusInternalServerError, "internal error")
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
	stamped, err := s.withRole(authkit.WithIdentity(r.Context(), identity), identity.ID)
	if err != nil {
		return r.Context()
	}
	return stamped
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
	stamped := authkit.WithIdentity(ctx, authkit.Identity{ID: user.ID, Email: user.Email, Name: user.Name})
	stamped = credential.WithToken(stamped, credential.Token{
		ID:     token.ID,
		Name:   token.Name,
		Scopes: token.Scopes,
	})
	return s.withRole(stamped, user.ID)
}
