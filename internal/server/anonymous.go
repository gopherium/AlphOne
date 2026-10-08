// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/lexer"

	"github.com/gopherium/alphone/internal/graphres"
)

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

// documentFits reports whether the body is a JSON graph request whose document is within the anonymous caps.
func documentFits(body []byte, bounds GraphBounds) bool {
	var request struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return false
	}
	if int64(len(request.Query)) > bounds.AnonymousQueryMaxBytes {
		return false
	}
	return tokensWithin(request.Query, bounds.AnonymousMaxTokens)
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
