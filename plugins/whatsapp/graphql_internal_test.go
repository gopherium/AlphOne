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

func TestTheListsAnswerWithinTheHostPageBounds(t *testing.T) {
	t.Parallel()

	p, err := Register(sdk.Deps{
		DatabaseURL: "postgres://postgres:alphone@localhost:9/postgres?sslmode=disable&connect_timeout=1",
		Pages:       sdk.PageBounds{Size: 2, Cap: 3},
	})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = p.Stop(context.Background()) })
	past := 4

	_, conversations := p.QueryResolvers().WhatsAppConversations(t.Context(), &past)
	_, messages := p.WhatsAppConversationResolvers().Messages(t.Context(),
		&model.WhatsAppConversation{ID: uuid.Must(uuid.NewV7())}, &past)

	for name, err := range map[string]error{"conversations": conversations, "messages": messages} {
		var coded sdk.GraphError
		if !errors.As(err, &coded) {
			t.Errorf("%s past the host cap error = %v, want a graph error", name, err)
			continue
		}
		if coded.Reason != "first_out_of_range" || coded.Meta["max"] != 3 {
			t.Errorf("%s past the host cap = %s %v, want first_out_of_range naming the cap 3", name, coded.Reason, coded.Meta)
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
