// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/google/uuid"
	"github.com/vektah/gqlparser/v2/ast"

	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/ratelimit"

	"github.com/gopherium/alphone/graph"
	"github.com/gopherium/alphone/internal/dyngraph"
	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/sdk"
)

// Graph endpoint guard bounds.
const (
	graphOperationTimeout   = 60 * time.Second
	graphMaxConcurrentOps   = 20
	graphJSONBodyLimit      = 1 << 20
	graphMultipartBodyLimit = 6 << 20
)

// Graph endpoint budget overflow answers.
const (
	overflowOperations = "too many concurrent operations"
	overflowStreams    = "too many concurrent streams"
)

// graphRetryAfter is how soon a caller may retry an operation the budget refused.
const graphRetryAfter = time.Second

// executableSchema serves the compiled schema, widened when field sources exist.
func executableSchema(root graph.ResolverRoot, sources []sdk.FieldSource) graphql.ExecutableSchema {
	if len(sources) == 0 {
		return graphres.ExecutableSchema(root)
	}
	return dyngraph.New(func(widened *ast.Schema) graphql.ExecutableSchema {
		return graphres.ExecutableSchemaOver(root, widened)
	}, sources...)
}

// graphPolicy is the budget one kind of graph request is held to.
type graphPolicy struct {
	// limiter caps concurrent requests of this kind per user.
	limiter *streamLimiter
	// lifetime bounds one request of this kind.
	lifetime time.Duration
	// retryAfter is how soon a slot of this kind is expected to free.
	retryAfter time.Duration
	// overflow is the answer when the budget is spent.
	overflow string
}

// retryAfterSeconds returns the whole seconds a retry hint rounds up to, never below one.
func retryAfterSeconds(hint time.Duration) int {
	seconds := int((hint + time.Second - 1) / time.Second)
	if seconds < 1 {
		return 1
	}
	return seconds
}

// newGraphQLHandler serves the guarded GraphQL endpoint over the composed resolver root.
func newGraphQLHandler(
	root graph.ResolverRoot,
	tenants TenantStore,
	streamLifetime time.Duration,
	maxStreams int,
	sources []sdk.FieldSource,
) http.Handler {
	schema := executableSchema(root, sources)
	srv := handler.New(schema)
	srv.AddTransport(subscriptionSSE{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.MultipartForm{})
	srv.Use(extension.Introspection{})
	srv.Use(extension.FixedComplexityLimit(graphres.ComplexityLimit))
	srv.AroundOperations(graphres.AnonymousGate)
	srv.AroundOperations(graphres.ScopeGate(graphres.NewScopeMap(schema.Schema())))
	srv.SetErrorPresenter(graphres.PresentError)
	loaded := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := graphres.WithHTTP(r.Context(), w, r)
		ctx = graphres.WithClientIP(ctx, ratelimit.ClientIP(r))
		ctx = sdk.WithRequestScope(ctx, sdk.NewRequestScope())
		if user := authkit.IdentityFromContext(r.Context()); user.ID != uuid.Nil {
			standing, err := withStandingTenant(sdk.WithUser(ctx, user.ID), tenants, user.ID)
			if err != nil {
				http.Error(w, "no tenant resolved", http.StatusInternalServerError)
				return
			}
			ctx = standing
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
		limiter:    newStreamLimiter(graphMaxConcurrentOps),
		lifetime:   graphOperationTimeout,
		retryAfter: graphRetryAfter,
		overflow:   overflowOperations,
	}, graphPolicy{
		limiter:    newStreamLimiter(maxStreams),
		lifetime:   streamLifetime,
		retryAfter: streamLifetime,
		overflow:   overflowStreams,
	}
}

// graphBodyLimit returns the body budget of the request content type.
func graphBodyLimit(r *http.Request) int64 {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		return graphMultipartBodyLimit
	}
	return graphJSONBodyLimit
}

// jsonAnswerTypes lists the answer media types a caller offers to read JSON with.
var jsonAnswerTypes = []string{
	"application/json",
	"application/graphql-response+json",
	"application/graphql+json",
}

// acceptsJSON reports whether the caller reads a JSON answer.
func acceptsJSON(accept string) bool {
	for _, mediaType := range jsonAnswerTypes {
		if strings.Contains(accept, mediaType) {
			return true
		}
	}
	return false
}

// subscriptionSSE serves the event stream transport to callers reading no JSON answer.
type subscriptionSSE struct {
	transport.SSE
}

// Supports reports whether the caller asks for a stream and reads no JSON answer.
func (t subscriptionSSE) Supports(r *http.Request) bool {
	return t.SSE.Supports(r) && !acceptsJSON(r.Header.Get("Accept"))
}

// acceptsEventStream reports whether the request asks for a stream and reads no JSON answer.
func acceptsEventStream(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "text/event-stream") && !acceptsJSON(accept)
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
			w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(policy.retryAfter)))
			authkit.RespondError(w, http.StatusTooManyRequests, authkit.ErrorResponse{Message: policy.overflow})
			return
		}
		defer policy.limiter.release(user.ID)
		r.Body = http.MaxBytesReader(w, r.Body, graphBodyLimit(r))
		ctx, cancel := context.WithTimeout(r.Context(), policy.lifetime)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
