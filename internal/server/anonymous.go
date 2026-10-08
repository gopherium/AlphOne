// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"sync"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/lexer"

	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/internal/graphres"
)

// serveAnonymous answers a graph request with no identity from a held answer, its slot freed before it is sent.
func serveAnonymous(next http.Handler, w http.ResponseWriter, r *http.Request, policy graphPolicy, bounds GraphBounds) {
	if !admitAnonymous(w, r, bounds) {
		return
	}
	free, claimed := policy.claim(r, authkit.Identity{})
	if !claimed {
		refuseOverflow(w, policy)
		return
	}
	answer := &heldAnswer{header: http.Header{}, limit: bounds.AnonymousAnswerMaxBytes}
	holdAnswer(next, answer, r, policy.lifetime, free)
	answer.send(answerWithin(w, policy.lifetime), r)
}

// holdAnswer runs the request into the held answer under the lifetime, freeing its slot once it returns.
func holdAnswer(next http.Handler, answer *heldAnswer, r *http.Request, lifetime time.Duration, free func()) {
	defer free()
	ctx, cancel := context.WithTimeout(r.Context(), lifetime)
	defer cancel()
	next.ServeHTTP(answer, r.WithContext(ctx))
}

// heldAnswer is a graph answer held in memory, up to a limit, before it reaches the caller.
type heldAnswer struct {
	header http.Header
	status int
	body   bytes.Buffer
	limit  int64
	over   bool
}

// Header returns the header the held answer carries.
func (a *heldAnswer) Header() http.Header {
	return a.header
}

// WriteHeader holds the status the answer carries.
func (a *heldAnswer) WriteHeader(status int) {
	a.status = status
}

// Write holds p, dropping the whole answer once it outgrows its limit.
func (a *heldAnswer) Write(p []byte) (int, error) {
	if a.over || int64(a.body.Len()+len(p)) > a.limit {
		a.over = true
		a.body.Reset()
		return len(p), nil
	}
	return a.body.Write(p)
}

// Flush leaves the held answer whole until it is sent.
func (a *heldAnswer) Flush() {}

// send writes the held answer to the caller, or the refusal in its place once it outgrew its limit.
func (a *heldAnswer) send(w http.ResponseWriter, r *http.Request) {
	if a.over {
		refuseAnonymous(w, r)
		return
	}
	maps.Copy(w.Header(), a.header)
	if a.status != 0 {
		w.WriteHeader(a.status)
	}
	_, _ = w.Write(a.body.Bytes())
}

// admitAnonymous reads the body of a graph request with no identity, refusing it and reporting false past a cap.
func admitAnonymous(w http.ResponseWriter, r *http.Request, bounds GraphBounds) bool {
	if carriesForm(r) {
		refuseAnonymous(w, r)
		return false
	}
	read, err := io.ReadAll(http.MaxBytesReader(w, r.Body, bounds.AnonymousBodyMaxBytes))
	if err != nil || !documentFits(read, bounds) {
		refuseAnonymous(w, r)
		return false
	}
	r.Body = io.NopCloser(bytes.NewReader(read))
	return true
}

// documentFits reports whether the body is a graph request the graph decodes with a document within the anonymous caps.
func documentFits(body []byte, bounds GraphBounds) bool {
	var params graphql.RawParams
	if err := json.Unmarshal(body, &params); err != nil {
		return false
	}
	if int64(len(params.Query)) > bounds.AnonymousQueryMaxBytes {
		return false
	}
	return tokensWithin(params.Query, bounds.AnonymousMaxTokens)
}

// tokensWithin reports whether the document holds at most limit tokens, one the lexer refuses counting as within.
func tokensWithin(document string, limit int) bool {
	tokens := lexer.New(&ast.Source{Input: document})
	for range limit + 1 {
		token, err := tokens.ReadToken()
		if err != nil || token.Kind == lexer.EOF {
			return true
		}
	}
	return false
}

// refuseAnonymous answers a graph request with no identity the gate's unauthenticated error, as one event to a stream.
func refuseAnonymous(w http.ResponseWriter, r *http.Request) {
	refusal, _ := json.Marshal(graphql.Response{Errors: gqlerror.List{graphres.UnauthenticatedError()}})
	if acceptsEventStream(r) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "event: next\ndata: %s\n\nevent: complete\n\n", refusal)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(refusal)
}

// addressLimiter caps the concurrent requests of callers with no identity per client address and across every address.
type addressLimiter struct {
	mu         sync.Mutex
	perAddress int
	ceiling    int
	total      int
	counts     map[string]int
}

// newAddressLimiter returns an addressLimiter admitting perAddress requests from one address and ceiling in all.
func newAddressLimiter(perAddress, ceiling int) *addressLimiter {
	return &addressLimiter{perAddress: perAddress, ceiling: ceiling, counts: map[string]int{}}
}

// acquire reserves a request slot for the address, reporting whether one was free.
func (l *addressLimiter) acquire(address string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.total >= l.ceiling || l.counts[address] >= l.perAddress {
		return false
	}
	l.counts[address]++
	l.total++
	return true
}

// release frees a request slot the address held.
func (l *addressLimiter) release(address string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.counts[address]--
	l.total--
	if l.counts[address] == 0 {
		delete(l.counts, address)
	}
}
