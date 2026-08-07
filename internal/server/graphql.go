// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"

	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/ratelimit"

	"github.com/gopherium/alphone/graph"
	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/sdk"
)

// Graph endpoint guard bounds.
const (
	graphOperationTimeout   = 60 * time.Second
	graphMaxConcurrentOps   = 5
	graphJSONBodyLimit      = 1 << 20
	graphMultipartBodyLimit = 6 << 20
)

// newGraphQLHandler serves the guarded GraphQL endpoint over the composed resolver root.
func newGraphQLHandler(root graph.ResolverRoot) http.Handler {
	srv := handler.New(graphres.ExecutableSchema(root))
	srv.AddTransport(transport.POST{})
	srv.Use(extension.Introspection{})
	srv.Use(extension.FixedComplexityLimit(graphres.ComplexityLimit))
	srv.AroundOperations(graphres.AnonymousGate)
	srv.SetErrorPresenter(graphres.PresentError)
	loaded := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := graphres.WithHTTP(r.Context(), w, r)
		ctx = graphres.WithClientIP(ctx, ratelimit.ClientIP(r))
		ctx = sdk.WithRequestScope(ctx, sdk.NewRequestScope())
		srv.ServeHTTP(w, r.WithContext(ctx))
	})
	return withOperationGuards(loaded, newStreamLimiter(graphMaxConcurrentOps), graphOperationTimeout)
}

// graphBodyLimit returns the body budget of the request content type.
func graphBodyLimit(r *http.Request) int64 {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		return graphMultipartBodyLimit
	}
	return graphJSONBodyLimit
}

// withOperationGuards bounds a graph request's body, lifetime, and per user concurrency.
func withOperationGuards(next http.Handler, ops *streamLimiter, timeout time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := authkit.IdentityFromContext(r.Context())
		if !ops.acquire(user.ID) {
			w.Header().Set("Retry-After", strconv.Itoa(int(timeout.Seconds())))
			authkit.RespondError(w, http.StatusTooManyRequests, "too many concurrent operations")
			return
		}
		defer ops.release(user.ID)
		r.Body = http.MaxBytesReader(w, r.Body, graphBodyLimit(r))
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
