// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/google/uuid"
	"github.com/vektah/gqlparser/v2/ast"

	"github.com/gopherium/alphone/graph"
)

// ComplexityLimit caps the priced cost of one operation.
const ComplexityLimit = 2500

// pageCost resolves a page size argument into a row multiplier, a missing or empty one priced as the unsized page.
func pageCost(first *int, unsized int) int {
	if first == nil || *first < 1 {
		return unsized
	}
	return *first
}

// boundedQueries is a query resolver set naming the page bounds it answers.
type boundedQueries interface {
	pageBounds() Paging
}

// pageBounds returns the page bounds the core lists answer.
func (q QueryResolvers) pageBounds() Paging {
	return q.root.Paging
}

// pagingOf returns the page bounds the root's core queries answer, the defaults for a root naming none.
func pagingOf(root graph.ResolverRoot) Paging {
	if root == nil {
		return Paging{}
	}
	if queries, ok := root.Query().(boundedQueries); ok {
		return queries.pageBounds()
	}
	return Paging{}
}

// ExecutableSchema builds the priced executable schema over the resolver root.
func ExecutableSchema(root graph.ResolverRoot) graphql.ExecutableSchema {
	return ExecutableSchemaOver(root, nil)
}

// ExecutableSchemaOver builds the priced executable schema serving schema in place of the compiled in one.
func ExecutableSchemaOver(root graph.ResolverRoot, schema *ast.Schema) graphql.ExecutableSchema {
	cfg := graph.Config{Resolvers: root, Schema: schema}
	unsized := pagingOf(root).size()
	cfg.Complexity.Query.Contacts = func(child int, _ *string, first *int, _ *string) int {
		return pageCost(first, unsized) * child
	}
	cfg.Complexity.Query.Tasks = func(child int, _, _ *time.Time, _ *uuid.UUID, _ *string, first *int, _ *string) int {
		return pageCost(first, unsized) * child
	}
	cfg.Complexity.Contact.Tasks = func(child int, _ *string, first *int, _ *string) int {
		return pageCost(first, unsized) * child
	}
	cfg.Complexity.Query.WhatsAppConversations = func(child int, limit *int) int {
		return pageCost(limit, DefaultPageSize) * child
	}
	cfg.Complexity.WhatsAppConversation.Messages = func(child int, limit *int) int {
		return pageCost(limit, DefaultPageSize) * child
	}
	return graph.NewExecutableSchema(cfg)
}
