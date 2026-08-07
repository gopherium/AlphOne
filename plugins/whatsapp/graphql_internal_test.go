// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/sdk"
)

// newBrokenPlugin returns a plugin whose pool points at an unreachable database.
func newBrokenPlugin(t *testing.T) *Plugin {
	t.Helper()
	p, err := Register(sdk.Deps{
		DatabaseURL: "postgres://postgres:alphone@localhost:9/postgres?sslmode=disable&connect_timeout=1",
	})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = p.Stop(context.Background()) })
	return p
}

func TestGraphListLimitBounds(t *testing.T) {
	t.Parallel()

	if got, err := graphListLimit(nil); err != nil || got != defaultListLimit {
		t.Errorf("graphListLimit(nil) = %d, %v, want the default %d", got, err, defaultListLimit)
	}
	top := maxListLimit
	if got, err := graphListLimit(&top); err != nil || got != maxListLimit {
		t.Errorf("graphListLimit(%d) = %d, %v, want it accepted", maxListLimit, got, err)
	}
	for _, invalid := range []int{0, -1, maxListLimit + 1} {
		_, err := graphListLimit(&invalid)
		var coded sdk.GraphError
		if !errors.As(err, &coded) || coded.Code != "VALIDATION" {
			t.Errorf("graphListLimit(%d) error = %v, want a VALIDATION graph error", invalid, err)
		}
	}
}

func TestConversationContactRequiresTheStash(t *testing.T) {
	t.Parallel()

	resolvers := (&Plugin{}).WhatsAppConversationResolvers()

	_, err := resolvers.Contact(t.Context(), &model.WhatsAppConversation{})

	if !errors.Is(err, errMissingContactStash) {
		t.Errorf("Contact() error = %v, want %v", err, errMissingContactStash)
	}
}

func TestFetchConversationsFansTheStoreErrorOut(t *testing.T) {
	t.Parallel()

	p := newBrokenPlugin(t)
	ids := []uuid.UUID{uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())}

	results, errs := p.fetchConversations(t.Context(), ids)

	if len(results) != len(ids) || len(errs) != len(ids) {
		t.Fatalf("results = %d, errs = %d, want one slot per requested id", len(results), len(errs))
	}
	for i, err := range errs {
		if err == nil {
			t.Errorf("errs[%d] = nil, want the store failure fanned out", i)
		}
	}
}

func TestConversationsLoaderNeedsARequestScope(t *testing.T) {
	t.Parallel()

	p := newBrokenPlugin(t)

	_, err := p.conversationsLoader(t.Context())

	if !errors.Is(err, sdk.ErrNoRequestScope) {
		t.Errorf("conversationsLoader() error = %v, want %v", err, sdk.ErrNoRequestScope)
	}
}
