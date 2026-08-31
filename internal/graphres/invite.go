// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/internal/mail"
	"github.com/gopherium/alphone/internal/role"
)

// errTokenInvalid refuses a spent, expired or unknown single use link.
var errTokenInvalid = errors.New("this link is no longer valid")

// deliveredPayload is the neutral answer every invite outcome shares.
func deliveredPayload() *model.InvitePayload {
	return &model.InvitePayload{Delivered: true}
}

// Invite creates an invited account under the role it names and delivers the activation link.
func (m MutationResolvers) Invite(
	ctx context.Context, email, name string, named *string,
) (*model.InvitePayload, error) {
	stood := role.Member
	if named != nil {
		parsed, err := role.Parse(*named)
		if err != nil {
			return nil, err
		}
		stood = parsed
	}
	actor := authkit.IdentityFromContext(ctx)
	if !role.Outranks(role.Role(actor.Role), stood) {
		return nil, role.ErrBeyondReach
	}
	token, err := m.root.Invites.Invite(ctx, email, name, stood.String())
	if errors.Is(err, gouncer.ErrEmailTaken) {
		return m.resend(ctx, email)
	}
	if err != nil {
		return nil, err
	}
	return m.deliverInvite(ctx, email, name, token)
}

// ResendInvite replaces a pending account's activation link, answering every address the same way.
func (m MutationResolvers) ResendInvite(ctx context.Context, email string) (*model.InvitePayload, error) {
	return m.resend(ctx, email)
}

// resend replaces the pending token behind email, staying neutral for every other address.
func (m MutationResolvers) resend(ctx context.Context, email string) (*model.InvitePayload, error) {
	held, err := m.root.Accounts.UserByEmail(ctx, normalizedEmail(email))
	if errors.Is(err, gouncer.ErrUserNotFound) {
		return deliveredPayload(), nil
	}
	if err != nil {
		return nil, err
	}
	sharing, err := m.root.sharesTenant(ctx, authkit.IdentityFromContext(ctx).ID, held.ID)
	if err != nil {
		return nil, err
	}
	if !sharing {
		return deliveredPayload(), nil
	}
	token, err := m.root.Invites.ResendInvite(ctx, email)
	if errors.Is(err, gouncer.ErrUserNotFound) || errors.Is(err, authkit.ErrAlreadyActivated) {
		return deliveredPayload(), nil
	}
	if err != nil {
		return nil, err
	}
	return m.deliverInvite(ctx, held.Email, held.Name, token)
}

// normalizedEmail returns the address the store keys an account under.
func normalizedEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// deliverInvite mails the activation link, or answers it for hand delivery without a mailer.
func (m MutationResolvers) deliverInvite(
	ctx context.Context, to, name string, token gouncer.Token,
) (*model.InvitePayload, error) {
	link := mail.ActivationLink(m.root.PublicURL, token.Token)
	if m.root.Mailer == nil {
		return &model.InvitePayload{Delivered: false, ActivationLink: &link}, nil
	}
	if err := m.root.Mailer.SendInvite(ctx, to, name, link); err != nil {
		return nil, fmt.Errorf("sending the invitation: %w", err)
	}
	return deliveredPayload(), nil
}

// AcceptInvite spends the activation link, setting the password and starting the session.
func (m MutationResolvers) AcceptInvite(
	ctx context.Context, token, password string,
) (*model.LoginPayload, error) {
	key, err := m.checkTokenBudget(ctx)
	if err != nil {
		return nil, err
	}
	id, err := m.root.Invites.RedeemInvite(ctx, token, password)
	if refused, mappedErr := m.refuseDeadToken(key, err); refused {
		return nil, mappedErr
	}
	if err := m.refuseDeactivatedTenant(ctx, id); err != nil {
		return nil, err
	}
	cookie, err := m.root.Auth.StartSession(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := setResponseCookie(ctx, cookie); err != nil {
		return nil, err
	}
	identity, err := m.root.Auth.SessionIdentity(ctx, cookie.Value)
	if err != nil {
		return nil, err
	}
	return &model.LoginPayload{Me: toAuthIdentity(identity, role.Role(identity.Role))}, nil
}

// RequestPasswordReset asks for a reset link, answering nothing about the address.
func (m MutationResolvers) RequestPasswordReset(ctx context.Context, email string) (bool, error) {
	key := clientIPFrom(ctx)
	allowed, retryAfter, err := m.root.ResetLimiter.Check(key)
	if err != nil {
		return false, err
	}
	if !allowed {
		return false, rateLimitedError{retryAfter: retryAfter}
	}
	if err := m.root.ResetLimiter.RecordFailure(key); err != nil {
		return false, fmt.Errorf("recording the reset request: %w", err)
	}
	if m.root.Mailer == nil {
		return true, nil
	}
	held, err := m.root.Accounts.UserByEmail(ctx, normalizedEmail(email))
	if err != nil {
		m.logReset(ctx, "reading the account behind a reset request", err)
		return true, nil
	}
	cooled, err := m.spendResetCooldown(held.Email)
	if err != nil {
		m.logReset(ctx, "budgeting the reset mail for an address", err)
		return true, nil
	}
	if !cooled {
		return true, nil
	}
	token, err := m.root.Invites.RequestReset(ctx, email)
	if err != nil {
		m.logReset(ctx, "issuing a reset token", err)
		return true, nil
	}
	link := mail.ResetLink(m.root.PublicURL, token.Token)
	if err := m.root.Mailer.SendReset(ctx, held.Email, held.Name, link); err != nil {
		m.logReset(ctx, "sending the reset link", err)
	}
	return true, nil
}

// spendResetCooldown reports whether the address may be mailed now, spending its budget.
func (m MutationResolvers) spendResetCooldown(address string) (bool, error) {
	if m.root.ResetCooldown == nil {
		return true, nil
	}
	allowed, _, err := m.root.ResetCooldown.Check(address)
	if err != nil || !allowed {
		return false, err
	}
	return true, m.root.ResetCooldown.RecordFailure(address)
}

// logReset records what the neutral reset answer hides.
func (m MutationResolvers) logReset(ctx context.Context, what string, err error) {
	if m.root.Logger == nil {
		return
	}
	m.root.Logger.ErrorContext(ctx, what, "error", err)
}

// ResetPassword spends the reset link, replacing the password and ending every session.
func (m MutationResolvers) ResetPassword(ctx context.Context, token, password string) (bool, error) {
	key, err := m.checkTokenBudget(ctx)
	if err != nil {
		return false, err
	}
	_, err = m.root.Invites.RedeemReset(ctx, token, password)
	if refused, mappedErr := m.refuseDeadToken(key, err); refused {
		return false, mappedErr
	}
	return true, nil
}

// checkTokenBudget reserves a token operation for the caller's IP key.
func (m MutationResolvers) checkTokenBudget(ctx context.Context) (string, error) {
	key := clientIPFrom(ctx)
	allowed, retryAfter, err := m.root.TokenLimiter.Check(key)
	if err != nil {
		return "", err
	}
	if !allowed {
		return "", rateLimitedError{retryAfter: retryAfter}
	}
	return key, nil
}

// refuseDeadToken maps a dead link to its refusal, counting it against key.
func (m MutationResolvers) refuseDeadToken(key string, err error) (bool, error) {
	if err == nil {
		return false, nil
	}
	if errors.Is(err, gouncer.ErrTokenNotFound) || errors.Is(err, gouncer.ErrUserNotFound) {
		if recordErr := m.root.TokenLimiter.RecordFailure(key); recordErr != nil {
			return true, fmt.Errorf("recording the refused token: %w", recordErr)
		}
		return true, errTokenInvalid
	}
	return true, err
}
