// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/google/uuid"

	"github.com/gopherium/alphone/graph"
)

// ComplexityLimit caps the priced cost of one operation.
const ComplexityLimit = 2500

// pageCost resolves the first argument into a row multiplier.
func pageCost(first *int) int {
	if first == nil {
		return defaultPageSize
	}
	return *first
}

// ExecutableSchema builds the priced executable schema over the resolver.
func ExecutableSchema(r *Resolver) graphql.ExecutableSchema {
	cfg := graph.Config{Resolvers: r}
	cfg.Complexity.Query.Contacts = func(child int, _ *string, first *int, _ *string) int {
		return pageCost(first) * child
	}
	cfg.Complexity.Query.Tasks = func(child int, _, _ *time.Time, _ *uuid.UUID, _ *string, first *int, _ *string) int {
		return pageCost(first) * child
	}
	cfg.Complexity.Contact.Tasks = func(child int, _ *string, first *int, _ *string) int {
		return pageCost(first) * child
	}
	return graph.NewExecutableSchema(cfg)
}
