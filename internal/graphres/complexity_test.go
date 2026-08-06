// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"testing"

	"github.com/99designs/gqlgen/complexity"
	"github.com/google/uuid"
	gqlparser "github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/validator/rules"

	"github.com/gopherium/alphone/internal/graphres"
)

// operationCost computes the priced complexity of a query document.
func operationCost(t *testing.T, doc string) int {
	t.Helper()
	schema := graphres.ExecutableSchema(&graphres.Resolver{})
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

func TestRealScreenDocumentsFitUnderTheCapWithHeadroom(t *testing.T) {
	t.Parallel()

	screens := map[string]string{
		"tasks screen": `{ tasks(date: "2026-08-06", first: 50) {
			edges { node { id title status priority dueOn contact { id name } } cursor }
			pageInfo { hasNextPage endCursor }
		} }`,
		"contact detail": `query($id: UUID!) { contact(id: $id) {
			id name createdAt
			identities { id channel identifier displayName }
			tasks(first: 50) { edges { node { id title status dueOn } cursor } pageInfo { hasNextPage endCursor } }
		} }`,
		"contacts list": `{ contacts(first: 50) {
			edges { node { id name createdAt } cursor }
			pageInfo { hasNextPage endCursor }
		} }`,
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
