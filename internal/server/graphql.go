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
	"github.com/google/uuid"

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

// Graph endpoint budget overflow answers.
const (
	overflowOperations = "too many concurrent operations"
	overflowStreams    = "too many concurrent streams"
)

// graphPolicy is the budget one kind of graph request is held to.
type graphPolicy struct {
	// limiter caps concurrent requests of this kind per user.
	limiter *streamLimiter
	// lifetime bounds one request of this kind.
	lifetime time.Duration
	// overflow is the answer when the budget is spent.
	overflow string
}

// newGraphQLHandler serves the guarded GraphQL endpoint over the composed resolver root.
func newGraphQLHandler(root graph.ResolverRoot, streamLifetime time.Duration, maxStreams int) http.Handler {
	srv := handler.New(graphres.ExecutableSchema(root))
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.MultipartForm{})
	srv.Use(extension.Introspection{})
	srv.Use(extension.FixedComplexityLimit(graphres.ComplexityLimit))
	srv.AroundOperations(graphres.AnonymousGate)
	srv.SetErrorPresenter(graphres.PresentError)
	loaded := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := graphres.WithHTTP(r.Context(), w, r)
		ctx = graphres.WithClientIP(ctx, ratelimit.ClientIP(r))
		ctx = sdk.WithRequestScope(ctx, sdk.NewRequestScope())
		if user := authkit.IdentityFromContext(r.Context()); user.ID != uuid.Nil {
			ctx = sdk.WithUser(ctx, user.ID)
		}
		srv.ServeHTTP(w, r.WithContext(ctx))
	})
	operations, streams := graphPolicies(streamLifetime, maxStreams)
	return withOperationGuards(loaded, operations, streams)
}

// graphPolicies returns the budget the endpoint holds operations to beside the
// one it holds streams to.
func graphPolicies(streamLifetime time.Duration, maxStreams int) (graphPolicy, graphPolicy) {
	return graphPolicy{
			limiter:  newStreamLimiter(graphMaxConcurrentOps),
			lifetime: graphOperationTimeout,
			overflow: overflowOperations,
		}, graphPolicy{
			limiter:  newStreamLimiter(maxStreams),
			lifetime: streamLifetime,
			overflow: overflowStreams,
		}
}

// graphBodyLimit returns the body budget of the request content type.
func graphBodyLimit(r *http.Request) int64 {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		return graphMultipartBodyLimit
	}
	return graphJSONBodyLimit
}

// acceptsEventStream reports whether the request asks for a stream.
func acceptsEventStream(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/event-stream")
}

// withOperationGuards bounds a graph request's body, lifetime, and per user
// concurrency under the policy of its kind.
func withOperationGuards(next http.Handler, operations, streams graphPolicy) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		policy := operations
		if acceptsEventStream(r) {
			policy = streams
		}
		user := authkit.IdentityFromContext(r.Context())
		if !policy.limiter.acquire(user.ID) {
			w.Header().Set("Retry-After", strconv.Itoa(int(policy.lifetime.Seconds())))
			authkit.RespondError(w, http.StatusTooManyRequests, policy.overflow)
			return
		}
		defer policy.limiter.release(user.ID)
		r.Body = http.MaxBytesReader(w, r.Body, graphBodyLimit(r))
		ctx, cancel := context.WithTimeout(r.Context(), policy.lifetime)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
