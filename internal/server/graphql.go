// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"context"
	"mime"
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
	// AnonymousBodyMaxBytes caps the body of a graph request that carries no identity.
	AnonymousBodyMaxBytes int64
	// AnonymousPerIP caps the graph requests with no identity one client address runs at once.
	AnonymousPerIP int
	// AnonymousCeiling caps the graph requests with no identity every address runs at once together.
	AnonymousCeiling int
	// AnonymousMaxTokens caps the tokens of the document a graph request with no identity carries.
	AnonymousMaxTokens int
	// AnonymousQueryMaxBytes caps the bytes of the document a graph request with no identity carries.
	AnonymousQueryMaxBytes int64
}

// DefaultGraphBounds are the graph bounds a zero field falls back to.
var DefaultGraphBounds = GraphBounds{
	OperationsPerUser:      20,
	OperationTimeout:       60 * time.Second,
	BodyMaxBytes:           1 << 20,
	UploadMaxBytes:         6 << 20,
	RetryAfter:             time.Second,
	AnonymousBodyMaxBytes:  16 << 10,
	AnonymousPerIP:         5,
	AnonymousCeiling:       20,
	AnonymousMaxTokens:     64,
	AnonymousQueryMaxBytes: 1024,
}

// withDefaults returns the bounds with every zero field taken from DefaultGraphBounds.
func (b GraphBounds) withDefaults() GraphBounds {
	d := DefaultGraphBounds
	return GraphBounds{
		OperationsPerUser:      orDefault(b.OperationsPerUser, d.OperationsPerUser),
		OperationTimeout:       orDefault(b.OperationTimeout, d.OperationTimeout),
		BodyMaxBytes:           orDefault(b.BodyMaxBytes, d.BodyMaxBytes),
		UploadMaxBytes:         orDefault(b.UploadMaxBytes, d.UploadMaxBytes),
		RetryAfter:             orDefault(b.RetryAfter, d.RetryAfter),
		AnonymousBodyMaxBytes:  orDefault(b.AnonymousBodyMaxBytes, d.AnonymousBodyMaxBytes),
		AnonymousPerIP:         orDefault(b.AnonymousPerIP, d.AnonymousPerIP),
		AnonymousCeiling:       orDefault(b.AnonymousCeiling, d.AnonymousCeiling),
		AnonymousMaxTokens:     orDefault(b.AnonymousMaxTokens, d.AnonymousMaxTokens),
		AnonymousQueryMaxBytes: orDefault(b.AnonymousQueryMaxBytes, d.AnonymousQueryMaxBytes),
	}
}

// orDefault returns value, or fallback when value is zero.
func orDefault[T comparable](value, fallback T) T {
	var zero T
	if value == zero {
		return fallback
	}
	return value
}

// Graph endpoint budget overflow answers.
const (
	overflowOperations = "too many concurrent operations"
	overflowStreams    = "too many concurrent streams"
)

// graphServer returns the gqlgen server answering over one executable schema with every graph guard.
func graphServer(schema graphql.ExecutableSchema, scopes graphres.ScopeMap, bounds GraphBounds) *handler.Server {
	srv := handler.New(schema)
	srv.AddTransport(subscriptionSSE{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.MultipartForm{MaxUploadSize: bounds.UploadMaxBytes})
	srv.Use(extension.Introspection{})
	srv.Use(extension.FixedComplexityLimit(graphres.ComplexityLimit))
	srv.AroundOperations(graphres.AnonymousGate)
	srv.AroundOperations(graphres.ScopeGate(scopes))
	srv.SetErrorPresenter(graphres.PresentError)
	return srv
}

// tenantGraphs returns the graph servers answering each tenant over its own widened schema.
func tenantGraphs(
	root graph.ResolverRoot, sources []sdk.FieldSource, held int, bounds GraphBounds,
) *dyngraph.Graphs[*handler.Server] {
	scopes := graphres.NewScopeMap(graphres.ExecutableSchema(root).Schema())
	return dyngraph.New(func(widened *ast.Schema) graphql.ExecutableSchema {
		return graphres.ExecutableSchemaOver(root, widened)
	}, func(schema graphql.ExecutableSchema) *handler.Server {
		return graphServer(schema, scopes, bounds)
	}, held, sources...)
}

// graphPolicy is the budget one kind of graph request is held to.
type graphPolicy struct {
	// limiter caps concurrent requests of this kind per user.
	limiter *streamLimiter
	// anonymous caps the concurrent requests of callers with no identity per client address.
	anonymous *addressLimiter
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
	graphs := tenantGraphs(root, sources, held, bounds)
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
	anonymous := newAddressLimiter(bounds.AnonymousPerIP, bounds.AnonymousCeiling)
	return graphPolicy{
		limiter:    newStreamLimiter(bounds.OperationsPerUser),
		anonymous:  anonymous,
		lifetime:   bounds.OperationTimeout,
		retryAfter: bounds.RetryAfter,
		overflow:   overflowOperations,
	}, graphPolicy{
		limiter:    newStreamLimiter(maxStreams),
		anonymous:  anonymous,
		lifetime:   streamLifetime,
		retryAfter: streamLifetime,
		overflow:   overflowStreams,
	}
}

// claim takes a slot for the caller under the policy, answering how to free it, or false when none is free.
func (p graphPolicy) claim(r *http.Request, user authkit.Identity) (func(), bool) {
	if user.ID == uuid.Nil {
		address := ratelimit.ClientIP(r)
		if !p.anonymous.acquire(address) {
			return nil, false
		}
		return func() { p.anonymous.release(address) }, true
	}
	if !p.limiter.acquire(user.ID) {
		return nil, false
	}
	return func() { p.limiter.release(user.ID) }, true
}

// graphBodyLimit returns the body budget the bounds give a signed in request's content type.
func graphBodyLimit(r *http.Request, bounds GraphBounds) int64 {
	if carriesForm(r) {
		return bounds.UploadMaxBytes
	}
	return bounds.BodyMaxBytes
}

// carriesForm reports whether the request body is a multipart form, whatever the case of its media type.
func carriesForm(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mediaType == "multipart/form-data"
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

// withOperationGuards bounds a graph request's body, lifetime, answer, and per user
// concurrency under the policy of its kind.
func withOperationGuards(next http.Handler, operations, streams graphPolicy, bounds GraphBounds) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		policy := operations
		if acceptsEventStream(r) {
			policy = streams
		}
		user := authkit.IdentityFromContext(r.Context())
		if user.ID == uuid.Nil && !admitAnonymous(w, r, bounds) {
			return
		}
		free, claimed := policy.claim(r, user)
		if !claimed {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(policy.retryAfter)))
			authkit.RespondError(w, http.StatusTooManyRequests, authkit.ErrorResponse{Message: policy.overflow})
			return
		}
		defer free()
		if user.ID != uuid.Nil {
			r.Body = http.MaxBytesReader(w, r.Body, graphBodyLimit(r, bounds))
		}
		ctx, cancel := context.WithTimeout(r.Context(), policy.lifetime)
		defer cancel()
		next.ServeHTTP(answerWithin(w, policy.lifetime), r.WithContext(ctx))
	})
}

// answerWithin returns w giving each write of the answer the lifetime to reach the caller.
func answerWithin(w http.ResponseWriter, lifetime time.Duration) deadlineWriter {
	return deadlineWriter{ResponseWriter: w, controller: http.NewResponseController(w), lifetime: lifetime}
}

// deadlineWriter gives each write of a graph answer the request's lifetime to reach the caller.
type deadlineWriter struct {
	http.ResponseWriter
	controller *http.ResponseController
	lifetime   time.Duration
}

// Write sends p to the caller under a write deadline one lifetime away.
func (d deadlineWriter) Write(p []byte) (int, error) {
	_ = d.controller.SetWriteDeadline(time.Now().Add(d.lifetime))
	return d.ResponseWriter.Write(p)
}

// Flush sends what the answer holds buffered to the caller.
func (d deadlineWriter) Flush() {
	_ = d.controller.Flush()
}
