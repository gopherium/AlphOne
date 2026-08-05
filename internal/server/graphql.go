// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"net/http"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"

	"github.com/gopherium/alphone/graph"
	"github.com/gopherium/alphone/internal/graphres"
)

// newGraphQLHandler serves the GraphQL endpoint over the core resolvers.
func newGraphQLHandler(version string) http.Handler {
	schema := graph.NewExecutableSchema(graph.Config{Resolvers: &graphres.Resolver{Version: version}})
	srv := handler.New(schema)
	srv.AddTransport(transport.POST{})
	srv.Use(extension.Introspection{})
	srv.SetErrorPresenter(graphres.PresentError)
	return srv
}
