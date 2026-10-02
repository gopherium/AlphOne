// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"fmt"
	"testing"

	"github.com/99designs/gqlgen/complexity"
	"github.com/99designs/gqlgen/graphql"
	"github.com/google/uuid"
	gqlparser "github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/validator/rules"

	"github.com/gopherium/alphone/graph"
	"github.com/gopherium/alphone/internal/graphres"
)

// operationCost computes the priced complexity of a query document.
func operationCost(t *testing.T, doc string) int {
	t.Helper()
	return costUnder(t, graphres.ExecutableSchema(composedRoot(t, &graphres.Resolver{})), doc)
}

// costUnder computes the priced complexity of a query document over schema.
func costUnder(t *testing.T, schema graphql.ExecutableSchema, doc string) int {
	t.Helper()
	query, err := gqlparser.LoadQueryWithRules(schema.Schema(), doc, rules.NewDefaultRules())
	if err != nil {
		t.Fatalf("parsing document: %v", err)
	}
	total := 0
	for _, op := range query.Operations {
		total += complexity.Calculate(t.Context(), schema, op, map[string]any{})
	}
	return total
}

func TestExecutableSchemaOverServesTheInjectedSchema(t *testing.T) {
	t.Parallel()

	root := composedRoot(t, &graphres.Resolver{})
	compiled := graphres.ExecutableSchema(root).Schema()
	widened := *compiled

	schema := graphres.ExecutableSchemaOver(root, &widened)

	if schema.Schema() != &widened {
		t.Error("Schema() = the compiled schema, want the injected one served")
	}
	if graphres.ExecutableSchemaOver(root, nil).Schema() != compiled {
		t.Error("Schema() with nil = a substitute, want the compiled schema")
	}
}

func TestRealScreenDocumentsFitUnderTheCapWithHeadroom(t *testing.T) {
	t.Parallel()

	screens := map[string]string{
		"contacts list": `query Contacts($q: String, $first: Int, $after: String) {
			contacts(q: $q, first: $first, after: $after) {
				edges { node { id name createdAt } cursor }
				pageInfo { hasNextPage endCursor }
			}
		}`,
		"contact detail": `query ContactDetail($id: UUID!, $first: Int, $after: String) {
			contact(id: $id) {
				id name createdAt
				identities { id channel identifier displayName }
				tasks(status: "open", first: $first, after: $after) {
					edges { node { id title status priority dueOn } cursor }
					pageInfo { hasNextPage endCursor }
				}
			}
		}`,
		"day tasks": `query DayTasks($date: Date!, $status: String!, $first: Int!, $after: String) {
			tasks(date: $date, status: $status, first: $first, after: $after) {
				edges { node { id title status priority dueOn } cursor }
				pageInfo { hasNextPage endCursor }
			}
		}`,
		"task detail": `query TaskDetail($id: UUID!) {
			task(id: $id) {
				id title status priority dueOn contactId
				contact { id name }
			}
		}`,
	}
	for name, doc := range screens {
		cost := operationCost(t, doc)
		t.Logf("%s costs %d of the %d cap", name, cost, graphres.ComplexityLimit)
		if cost*3 > graphres.ComplexityLimit {
			t.Errorf("%s costs %d, want three times headroom under the cap %d", name, cost, graphres.ComplexityLimit)
		}
		if cost == 0 {
			t.Errorf("%s costs 0, the pricing is not engaged", name)
		}
	}
}

// unsizedAndSized pairs each core list read naming no size with the same read naming 200 rows.
func unsizedAndSized() map[string][2]string {
	return map[string][2]string{
		"contacts": {
			`{ contacts { edges { node { id } } } }`,
			`{ contacts(first: 200) { edges { node { id } } } }`,
		},
		"tasks": {
			`{ tasks(date: "2026-08-06") { edges { node { id } } } }`,
			`{ tasks(date: "2026-08-06", first: 200) { edges { node { id } } } }`,
		},
		"contact tasks": {
			`{ contact(id: "00000000-0000-0000-0000-000000000001") { tasks { edges { node { id } } } } }`,
			`{ contact(id: "00000000-0000-0000-0000-000000000001") { tasks(first: 200) { edges { node { id } } } } }`,
		},
	}
}

func TestAnUnsizedListIsPricedAtTheConfiguredPage(t *testing.T) {
	t.Parallel()

	schema := graphres.ExecutableSchema(composedRoot(t, &graphres.Resolver{
		Paging: graphres.Paging{Size: 200, Cap: 200},
	}))

	for name, reads := range unsizedAndSized() {
		unsized, sized := costUnder(t, schema, reads[0]), costUnder(t, schema, reads[1])
		if unsized != sized {
			t.Errorf("%s unsized costs %d, want the 200 row page's %d", name, unsized, sized)
		}
	}
}

// unboundedRoot is a resolver root whose query set names no page bounds.
type unboundedRoot struct {
	graph.ResolverRoot
}

// Query returns no query set.
func (unboundedRoot) Query() graph.QueryResolver {
	return nil
}

func TestAnUnsizedListOverARootNamingNoBoundsIsPricedAtTheDefaultPage(t *testing.T) {
	t.Parallel()

	for name, root := range map[string]graph.ResolverRoot{"no root": nil, "a root naming no bounds": unboundedRoot{}} {
		schema := graphres.ExecutableSchema(root)

		unsized := costUnder(t, schema, `{ contacts { edges { node { id } } } }`)
		sized := costUnder(t, schema, fmt.Sprintf(`{ contacts(first: %d) { edges { node { id } } } }`,
			graphres.DefaultPageSize))
		if unsized != sized {
			t.Errorf("%s prices unsized at %d, want the default page's %d", name, unsized, sized)
		}
	}
}

func TestMultipliedListQueriesExceedTheCap(t *testing.T) {
	t.Parallel()

	monster := `{ contacts(first: 200) {
		edges { node {
			tasks(first: 200, status: "all") { edges { node { title } } }
		} }
	} }`

	if cost := operationCost(t, monster); cost <= graphres.ComplexityLimit {
		t.Errorf("nested 200x200 query costs %d, want above the cap %d", cost, graphres.ComplexityLimit)
	}
}

func TestComplexityCapRejectsAtTheEndpoint(t *testing.T) {
	t.Parallel()

	resolver := &graphres.Resolver{Version: "9.9.9", Contacts: &stubContactStore{}, Tasks: &stubTaskStore{}}
	client := newGraphClient(t, resolver, uuid.Must(uuid.NewV7()))

	response, err := client.RawPost(`{ contacts(first: 200) {
		edges { node { tasks(first: 200, status: "all") { edges { node { title } } } } }
	} }`)
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}

	if got := firstErrorCode(t, response.Errors); got != "COMPLEXITY_LIMIT_EXCEEDED" {
		t.Errorf("code = %q, want COMPLEXITY_LIMIT_EXCEEDED", got)
	}
}

func TestWhatsAppListFieldsMultiplyLikeTheCoreLists(t *testing.T) {
	t.Parallel()

	monster := `{ whatsAppConversations(limit: 200) {
		messages(limit: 200) { content }
	} }`

	if cost := operationCost(t, monster); cost <= graphres.ComplexityLimit {
		t.Errorf("nested 200x200 whatsapp query costs %d, want above the cap %d", cost, graphres.ComplexityLimit)
	}
}

func TestWhatsAppScreenDocumentsFitUnderTheCapWithHeadroom(t *testing.T) {
	t.Parallel()

	screens := map[string]string{
		"conversation list": `query WhatsAppConversations {
			whatsAppConversations {
				id status lastActivityAt lastMessagePreview
				contact { id name }
			}
		}`,
		"thread": `query WhatsAppThread($conversationId: UUID!) {
			whatsAppConversation(id: $conversationId) {
				id
				contact { id name }
				messages {
					id externalId direction content contentType sentAt status statusDetail
					media { status mimeType filename fileSize voice animated downloadPath }
				}
			}
		}`,
	}
	for name, doc := range screens {
		cost := operationCost(t, doc)
		t.Logf("%s costs %d of the %d cap", name, cost, graphres.ComplexityLimit)
		if cost*3 > graphres.ComplexityLimit {
			t.Errorf("%s costs %d, want three times headroom under the cap %d", name, cost, graphres.ComplexityLimit)
		}
		if cost == 0 {
			t.Errorf("%s costs 0, the pricing is not engaged", name)
		}
	}
}
