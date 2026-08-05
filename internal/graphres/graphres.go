// SPDX-License-Identifier: Elastic-2.0

// Package graphres resolves the GraphQL schema over the core services.
package graphres

import (
	"context"

	"github.com/gopherium/alphone/graph"
)

// Resolver is the root resolver serving the core schema.
type Resolver struct {
	// Version is the reported application version.
	Version string
}

// Query returns the query resolver set.
func (r *Resolver) Query() graph.QueryResolver {
	return queryResolver{root: r}
}

// queryResolver serves the Query root fields.
type queryResolver struct {
	root *Resolver
}

// Version reports the application version.
func (q queryResolver) Version(context.Context) (string, error) {
	return q.root.Version, nil
}
