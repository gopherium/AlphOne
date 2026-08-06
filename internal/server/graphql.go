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
func newGraphQLHandler(cfg Config) http.Handler {
	resolver := &graphres.Resolver{Version: cfg.Version, Contacts: cfg.Contacts, Tasks: cfg.Tasks}
	schema := graph.NewExecutableSchema(graph.Config{Resolvers: resolver})
	srv := handler.New(schema)
	srv.AddTransport(transport.POST{})
	srv.Use(extension.Introspection{})
	srv.SetErrorPresenter(graphres.PresentError)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.ServeHTTP(w, r.WithContext(resolver.WithLoaders(r.Context())))
	})
}
