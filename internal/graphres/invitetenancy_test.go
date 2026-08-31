// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/gopherium/gouncer/authkit/testkit"

	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/tenant"
)

// pendingElsewhere invites an address and stands it in a tenant of its own.
func pendingElsewhere(t *testing.T, resolver *graphres.Resolver, store *testkit.Store) uuid.UUID {
	t.Helper()
	inviteThrough(t, resolver)
	invited, err := store.UserByEmail(t.Context(), "maria@example.com")
	if err != nil {
		t.Fatalf("reading the invited account: %v", err)
	}
	actor := uuid.Must(uuid.NewV7())
	resolver.Tenants = standingTenants{standing: map[uuid.UUID]uuid.UUID{
		actor:      tenant.DefaultID,
		invited.ID: uuid.Must(uuid.NewV7()),
	}}
	return actor
}

func TestResendKeepsAnotherTenantsInvitationOutOfReach(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	resolver := newInviteResolver(store, nil)
	actor := pendingElsewhere(t, resolver, store)
	client := newGraphClient(t, resolver, actor)

	var response struct {
		ResendInvite invitePayload `json:"resendInvite"`
	}
	client.MustPost(
		`mutation { resendInvite(email: "maria@example.com") { delivered activationLink } }`,
		&response,
	)

	if !response.ResendInvite.Delivered || response.ResendInvite.ActivationLink != nil {
		t.Errorf("resendInvite = %+v, want the neutral answer with no token", response.ResendInvite)
	}
}

func TestResendLeavesAnotherTenantsTokenStanding(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	mailer := &recordingMailer{}
	resolver := newInviteResolver(store, mailer)
	actor := pendingElsewhere(t, resolver, store)
	standing := storedTokens(store)
	client := newGraphClient(t, resolver, actor)

	var response struct {
		ResendInvite invitePayload `json:"resendInvite"`
	}
	client.MustPost(`mutation { resendInvite(email: "maria@example.com") { delivered } }`, &response)

	if held := storedTokens(store); !slices.Equal(held, standing) {
		t.Errorf("the stored tokens became %v, want another tenant's %v left standing", held, standing)
	}
	if len(mailer.invites) != 1 {
		t.Errorf("sent %d mails, want no second mail to another tenant's address", len(mailer.invites))
	}
}

// storedTokens answers the hashes the store holds, in a stable order.
func storedTokens(store *testkit.Store) []string {
	held := make([]string, 0, len(store.Tokens))
	for hash := range store.Tokens {
		held = append(held, hash)
	}
	slices.Sort(held)
	return held
}

func TestInviteKeepsAnotherTenantsAddressOutOfReach(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	resolver := newInviteResolver(store, nil)
	actor := pendingElsewhere(t, resolver, store)
	client := newGraphClient(t, resolver, actor)

	var response struct {
		Invite invitePayload `json:"invite"`
	}
	client.MustPost(
		`mutation { invite(email: "maria@example.com", name: "Maria Perez") { delivered activationLink } }`,
		&response,
	)

	if !response.Invite.Delivered || response.Invite.ActivationLink != nil {
		t.Errorf("invite = %+v, want the neutral answer with no token", response.Invite)
	}
}
