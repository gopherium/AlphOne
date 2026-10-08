// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"bytes"
	"context"
	"io"
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

// GraphBounds caps how many graph operations one caller runs at once, how long each runs and how large its body is.
type GraphBounds struct {
	// OperationsPerUser caps the graph operations one caller runs at once.
	OperationsPerUser int
	// OperationTimeout bounds how long one graph operation runs.
	OperationTimeout time.Duration
	// BodyMaxBytes caps the body of a JSON graph request.
	BodyMaxBytes int64
	// UploadMaxBytes caps the body of a multipart graph request.
	UploadMaxBytes int64
	// RetryAfter is how soon a caller the operation budget refused may try again.
	RetryAfter time.Duration
}

// DefaultGraphBounds are the graph bounds a zero field falls back to.
var DefaultGraphBounds = GraphBounds{
	OperationsPerUser: 20,
	OperationTimeout:  60 * time.Second,
	BodyMaxBytes:      1 << 20,
	UploadMaxBytes:    6 << 20,
	RetryAfter:        time.Second,
}

// withDefaults returns the bounds with every zero field taken from DefaultGraphBounds.
func (b GraphBounds) withDefaults() GraphBounds {
	if b.OperationsPerUser == 0 {
		b.OperationsPerUser = DefaultGraphBounds.OperationsPerUser
	}
	if b.OperationTimeout == 0 {
		b.OperationTimeout = DefaultGraphBounds.OperationTimeout
	}
	if b.BodyMaxBytes == 0 {
		b.BodyMaxBytes = DefaultGraphBounds.BodyMaxBytes
	}
	if b.UploadMaxBytes == 0 {
		b.UploadMaxBytes = DefaultGraphBounds.UploadMaxBytes
	}
	if b.RetryAfter == 0 {
		b.RetryAfter = DefaultGraphBounds.RetryAfter
	}
	return b
}

// Graph endpoint budget overflow answers.
const (
	overflowOperations = "too many concurrent operations"
	overflowStreams    = "too many concurrent streams"
)

// graphServer returns the gqlgen server answering over one executable schema with every graph guard.
func graphServer(schema graphql.ExecutableSchema, scopes graphres.ScopeMap) *handler.Server {
	srv := handler.New(schema)
	srv.AddTransport(subscriptionSSE{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.MultipartForm{})
	srv.Use(extension.Introspection{})
	srv.Use(extension.FixedComplexityLimit(graphres.ComplexityLimit))
	srv.AroundOperations(graphres.AnonymousGate)
	srv.AroundOperations(graphres.ScopeGate(scopes))
	srv.SetErrorPresenter(graphres.PresentError)
	return srv
}

// tenantGraphs returns the graph servers answering each tenant over its own widened schema.
func tenantGraphs(root graph.ResolverRoot, sources []sdk.FieldSource, held int) *dyngraph.Graphs[*handler.Server] {
	scopes := graphres.NewScopeMap(graphres.ExecutableSchema(root).Schema())
	return dyngraph.New(func(widened *ast.Schema) graphql.ExecutableSchema {
		return graphres.ExecutableSchemaOver(root, widened)
	}, func(schema graphql.ExecutableSchema) *handler.Server {
		return graphServer(schema, scopes)
	}, held, sources...)
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

// newGraphQLHandler serves the guarded GraphQL endpoint over the composed resolver root, each tenant its own fields.
func newGraphQLHandler(
	root graph.ResolverRoot,
	tenants TenantStore,
	bounds GraphBounds,
	streamLifetime time.Duration,
	maxStreams int,
	sources []sdk.FieldSource,
	held int,
) http.Handler {
	graphs := tenantGraphs(root, sources, held)
	loaded := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := graphres.WithHTTP(r.Context(), w, r)
		ctx = graphres.WithClientIP(ctx, ratelimit.ClientIP(r))
		ctx = sdk.WithRequestScope(ctx, sdk.NewRequestScope())
		answer := graphs.Plain()
		if user := authkit.IdentityFromContext(r.Context()); user.ID != uuid.Nil {
			standing, err := withStandingTenant(sdk.WithUser(ctx, user.ID), tenants, user.ID)
			if err != nil {
				http.Error(w, "no tenant resolved", http.StatusInternalServerError)
				return
			}
			ctx = standing
			answer = graphs.For(ctx)
		}
		answer.ServeHTTP(w, r.WithContext(ctx))
	})
	operations, streams := graphPolicies(bounds, streamLifetime, maxStreams)
	return withOperationGuards(loaded, operations, streams, bounds)
}

// graphPolicies returns the budget the endpoint holds operations to beside the
// one it holds streams to.
func graphPolicies(bounds GraphBounds, streamLifetime time.Duration, maxStreams int) (graphPolicy, graphPolicy) {
	return graphPolicy{
		limiter:    newStreamLimiter(bounds.OperationsPerUser),
		lifetime:   bounds.OperationTimeout,
		retryAfter: bounds.RetryAfter,
		overflow:   overflowOperations,
	}, graphPolicy{
		limiter:    newStreamLimiter(maxStreams),
		lifetime:   streamLifetime,
		retryAfter: streamLifetime,
		overflow:   overflowStreams,
	}
}

// graphBodyLimit returns the body budget the bounds give the request content type.
func graphBodyLimit(r *http.Request, bounds GraphBounds) int64 {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		return bounds.UploadMaxBytes
	}
	return bounds.BodyMaxBytes
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

// readAhead returns a body replaying everything read from body, then the error the read ended on.
func readAhead(body io.ReadCloser) io.ReadCloser {
	read, err := io.ReadAll(body)
	if err != nil {
		return io.NopCloser(io.MultiReader(bytes.NewReader(read), failedRead{err: err}))
	}
	return io.NopCloser(bytes.NewReader(read))
}

// failedRead is a reader answering the error a body read ended on.
type failedRead struct {
	err error
}

// Read answers the error the body read ended on.
func (f failedRead) Read([]byte) (int, error) {
	return 0, f.err
}

// withOperationGuards bounds a graph request's body, lifetime, and per user
// concurrency under the policy of its kind.
func withOperationGuards(next http.Handler, operations, streams graphPolicy, bounds GraphBounds) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		policy := operations
		if acceptsEventStream(r) {
			policy = streams
		}
		r.Body = readAhead(http.MaxBytesReader(w, r.Body, graphBodyLimit(r, bounds)))
		user := authkit.IdentityFromContext(r.Context())
		if !policy.limiter.acquire(user.ID) {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(policy.retryAfter)))
			authkit.RespondError(w, http.StatusTooManyRequests, authkit.ErrorResponse{Message: policy.overflow})
			return
		}
		defer policy.limiter.release(user.ID)
		ctx, cancel := context.WithTimeout(r.Context(), policy.lifetime)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
