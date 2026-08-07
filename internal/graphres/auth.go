// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/graph/model"
)

// rateLimitedError reports a login blocked by the attempt limiter.
type rateLimitedError struct {
	retryAfter time.Duration
}

// Error returns the limiter's rejection message.
func (e rateLimitedError) Error() string {
	return "too many login attempts, try again later"
}

// toAuthIdentity maps an authkit identity onto its graph model.
func toAuthIdentity(identity authkit.Identity) *model.Identity {
	return &model.Identity{ID: identity.ID, Email: identity.Email, Name: identity.Name}
}

// toUser maps an authkit account onto its graph model.
func toUser(account authkit.Account) *model.User {
	return &model.User{
		ID:        account.ID,
		Email:     account.Email,
		Name:      account.Name,
		Disabled:  account.Disabled,
		CreatedAt: account.CreatedAt,
	}
}

// Me reports the calling identity.
func (q QueryResolvers) Me(ctx context.Context) (*model.Identity, error) {
	return toAuthIdentity(authkit.IdentityFromContext(ctx)), nil
}

// Users lists every user account.
func (q QueryResolvers) Users(ctx context.Context) ([]*model.User, error) {
	accounts, err := q.root.Admin.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	users := make([]*model.User, len(accounts))
	for i, account := range accounts {
		users[i] = toUser(account)
	}
	return users, nil
}

// checkLoginBudget reserves a login attempt for the caller's IP key.
func (m MutationResolvers) checkLoginBudget(ctx context.Context) (string, error) {
	key := clientIPFrom(ctx)
	allowed, retryAfter, err := m.root.LoginLimiter.Check(key)
	if err != nil {
		return "", err
	}
	if !allowed {
		return "", rateLimitedError{retryAfter: retryAfter}
	}
	return key, nil
}

// authenticateCounted verifies credentials, counting failures against key.
func (m MutationResolvers) authenticateCounted(
	ctx context.Context, key, email, password string,
) (authkit.Identity, error) {
	identity, err := m.root.Auth.Authenticate(ctx, email, password)
	if errors.Is(err, authkit.ErrInvalidCredentials) {
		if recordErr := m.root.LoginLimiter.RecordFailure(key); recordErr != nil {
			return authkit.Identity{}, fmt.Errorf("recording the failed login: %w", recordErr)
		}
		return authkit.Identity{}, err
	}
	return identity, err
}

// Login verifies credentials under the attempt limiter and issues the session cookie.
func (m MutationResolvers) Login(ctx context.Context, email, password string) (*model.LoginPayload, error) {
	key, err := m.checkLoginBudget(ctx)
	if err != nil {
		return nil, err
	}
	identity, err := m.authenticateCounted(ctx, key, email, password)
	if err != nil {
		return nil, err
	}
	cookie, err := m.root.Auth.StartSession(ctx, identity.ID)
	if err != nil {
		return nil, err
	}
	if err := setResponseCookie(ctx, cookie); err != nil {
		return nil, err
	}
	return &model.LoginPayload{Me: toAuthIdentity(identity)}, nil
}

// Logout ends the calling session and clears its cookie.
func (m MutationResolvers) Logout(ctx context.Context) (bool, error) {
	carrier, err := httpFrom(ctx)
	if err != nil {
		return false, err
	}
	token := ""
	if cookie, cookieErr := carrier.request.Cookie(m.root.Auth.CookieName()); cookieErr == nil {
		token = cookie.Value
	}
	clearing, err := m.root.Auth.EndSession(ctx, token)
	if err != nil {
		return false, err
	}
	http.SetCookie(carrier.writer, clearing)
	return true, nil
}

// CreateUser creates a user account.
func (m MutationResolvers) CreateUser(ctx context.Context, email, name, password string) (*model.User, error) {
	account, err := m.root.Admin.CreateAccount(ctx, email, name, password)
	if err != nil {
		return nil, err
	}
	return toUser(account), nil
}

// SetUserDisabled updates whether the account may log in.
func (m MutationResolvers) SetUserDisabled(ctx context.Context, id uuid.UUID, disabled bool) (bool, error) {
	actor := authkit.IdentityFromContext(ctx)
	if err := m.root.Admin.SetAccountDisabled(ctx, actor.ID, id, disabled); err != nil {
		return false, err
	}
	return true, nil
}

// setResponseCookie writes cookie onto the serving response.
func setResponseCookie(ctx context.Context, cookie *http.Cookie) error {
	carrier, err := httpFrom(ctx)
	if err != nil {
		return err
	}
	http.SetCookie(carrier.writer, cookie)
	return nil
}
