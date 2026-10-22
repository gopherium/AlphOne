// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/framework/goncierge"
	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/sdk"
)

// errTenantDeactivated refuses a member of a deactivated tenant.
var errTenantDeactivated = errors.New("graphres: tenant deactivated")

// rateLimitedError reports a login blocked by the attempt limiter.
type rateLimitedError struct {
	retryAfter time.Duration
}

// Error returns the limiter's rejection message.
func (e rateLimitedError) Error() string {
	return "too many login attempts, try again later"
}

// toAuthIdentity maps an authkit identity onto its graph model, with what the registry gives its role.
func toAuthIdentity(identity authkit.Identity, roles *goncierge.Registry) *model.Identity {
	return &model.Identity{
		ID:           identity.ID,
		Email:        identity.Email,
		Name:         identity.Name,
		Role:         identity.Role,
		Capabilities: roles.CapabilitiesOf(identity.Role),
		Grantable:    roles.Grantable(identity.Role),
	}
}

// toUser maps an authkit account onto its graph model, with whether the reader may change it.
func toUser(account authkit.Account, tier role.Role, withinReach bool) *model.User {
	return &model.User{
		ID:          account.ID,
		Email:       account.Email,
		Name:        account.Name,
		Disabled:    account.Disabled,
		Confirmed:   account.Confirmed,
		CreatedAt:   account.CreatedAt,
		Role:        tier.String(),
		WithinReach: withinReach,
	}
}

// Me reports the calling identity.
func (q QueryResolvers) Me(ctx context.Context) (*model.Identity, error) {
	return toAuthIdentity(authkit.IdentityFromContext(ctx), q.root.Roles), nil
}

// Users lists the accounts standing in the caller's tenant, each marked by whether the caller may change it.
func (q QueryResolvers) Users(ctx context.Context) ([]*model.User, error) {
	accounts, err := q.root.Admin.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	caller := authkit.IdentityFromContext(ctx)
	ids := make([]uuid.UUID, 0, len(accounts)+1)
	ids = append(ids, caller.ID)
	for _, account := range accounts {
		ids = append(ids, account.ID)
	}
	placed, err := q.root.tenantsOf(ctx, ids)
	if err != nil {
		return nil, err
	}
	standing := placedIn(placed, caller.ID)
	manages := q.root.Roles.Can(caller.Role, role.ManageUsers)
	users := make([]*model.User, 0, len(accounts))
	for _, account := range accounts {
		if placedIn(placed, account.ID) != standing {
			continue
		}
		users = append(users, toUser(account, role.Role(account.Role), manages && q.root.reaches(caller.Role, account.Role)))
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
	if err := m.refuseDeactivatedTenant(ctx, identity.ID); err != nil {
		return nil, err
	}
	cookie, err := m.root.Auth.StartSession(ctx, identity.ID)
	if err != nil {
		return nil, err
	}
	if err := setResponseCookie(ctx, cookie); err != nil {
		return nil, err
	}
	return &model.LoginPayload{Me: toAuthIdentity(identity, m.root.Roles)}, nil
}

// refuseDeactivatedTenant refuses a caller whose standing tenant is deactivated.
func (m MutationResolvers) refuseDeactivatedTenant(ctx context.Context, userID uuid.UUID) error {
	if m.root.Tenants == nil {
		return nil
	}
	standing, err := m.root.Tenants.TenantForUser(ctx, userID)
	if err != nil {
		return err
	}
	if standing.Deactivated {
		return sdk.GraphError{Code: "UNAUTHENTICATED", Reason: "tenant_deactivated", Err: errTenantDeactivated}
	}
	return nil
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

// SetUserDisabled updates whether the account may log in, never enabling one whose role nobody declares.
func (m MutationResolvers) SetUserDisabled(ctx context.Context, id uuid.UUID, disabled bool) (bool, error) {
	actor := authkit.IdentityFromContext(ctx)
	return m.root.writeWithinReach(ctx, actor, id, func(held string) error {
		return m.root.Admin.SetAccountDisabledFrom(ctx, actor.ID, id, held, disabled)
	})
}

// SetUserRole stands one user in the tier it names, refusing an account whose role moved after the check.
func (m MutationResolvers) SetUserRole(ctx context.Context, id uuid.UUID, tier string) (bool, error) {
	actor := authkit.IdentityFromContext(ctx)
	if err := m.root.refuseGrant(actor.Role, tier); err != nil {
		return false, err
	}
	return m.root.writeWithinReach(ctx, actor, id, func(held string) error {
		return m.root.Admin.SetAccountRoleFrom(ctx, actor.ID, id, held, tier)
	})
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
