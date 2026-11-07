// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"

	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/internal/tenant"
)

// pendingUnder invites maria@example.com under the role named, with no mailer to deliver it.
func pendingUnder(t *testing.T, held role.Role) (*graphres.Resolver, *testkit.Store) {
	t.Helper()
	store := testkit.NewStore()
	resolver := newInviteResolver(store, nil)
	if _, err := resolver.Onboarding.InviteInto(
		t.Context(), tenant.DefaultID, "maria@example.com", "Maria Perez", held.String(),
	); err != nil {
		t.Fatalf("inviting the pending account: %v", err)
	}
	return resolver, store
}

func TestAnInvitationAnswersNeutrallyBeyondTheCallersReach(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"resending": `mutation { answer: resendInvite(email: "maria@example.com") ` +
			`{ delivered activationLink } }`,
		"inviting the same address": `mutation { answer: invite(email: "maria@example.com", name: "Maria Perez") ` +
			`{ delivered activationLink } }`,
	}
	for name, mutation := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			resolver, store := pendingUnder(t, stewardRole)
			standing := storedTokens(store)
			client := newActingClient(t, resolver, authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()})

			var response struct {
				Answer invitePayload `json:"answer"`
			}
			client.MustPost(mutation, &response)

			if !response.Answer.Delivered || response.Answer.ActivationLink != nil {
				t.Errorf("answer = %+v, want the neutral answer with no link for an account beyond reach", response.Answer)
			}
			if held := storedTokens(store); !slices.Equal(held, standing) {
				t.Errorf("the stored tokens became %v, want %v left standing", held, standing)
			}
		})
	}
}

func TestResendShowsTheLinkOfAnInvitationWithinReach(t *testing.T) {
	t.Parallel()

	resolver, _ := pendingUnder(t, role.Member)
	client := newActingClient(t, resolver, authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()})

	var response struct {
		ResendInvite invitePayload `json:"resendInvite"`
	}
	client.MustPost(`mutation { resendInvite(email: "maria@example.com") { delivered activationLink } }`, &response)

	if response.ResendInvite.Delivered || response.ResendInvite.ActivationLink == nil {
		t.Errorf("resendInvite = %+v, want the fresh link shown without a mailer", response.ResendInvite)
	}
}
