// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gopherium/alphone/internal/graphres"
)

// doubledFragmentsRequest returns a graph post reading a type through 40 fragments, each spreading the next twice.
func doubledFragmentsRequest(t *testing.T) *http.Request {
	t.Helper()
	var document strings.Builder
	document.WriteString(`{ __type(name: "Query") { ...F1 } }`)
	for level := 1; level <= 40; level++ {
		fmt.Fprintf(&document, " fragment F%d on __Type { ofType { ...F%d ...F%d } }", level, level+1, level+1)
	}
	document.WriteString(" fragment F41 on __Type { name }")
	body, err := json.Marshal(map[string]string{"query": document.String()})
	if err != nil {
		t.Fatalf("encoding the request: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, crmOrigin+"/api/graphql", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func TestGraphQLPricesEachNamedFragmentOnceAndRejectsPastTheLimit(t *testing.T) {
	t.Parallel()

	guarded := newGuardedServer(t)

	answered := decodeBody[graphResponse](t,
		guarded.serve(fromBrowser(doubledFragmentsRequest(t), crmOrigin, "same-origin", guarded.cookie)))

	want := fmt.Sprintf("operation has complexity %d, which exceeds the limit of %d", 1<<41, graphres.ComplexityLimit)
	if len(answered.Errors) != 1 {
		t.Fatalf("errors = %+v, want the one rejection past the limit", answered.Errors)
	}
	rejected := answered.Errors[0]
	if rejected.Message != want || rejected.Extensions["code"] != "COMPLEXITY_LIMIT_EXCEEDED" {
		t.Errorf("error = %+v, want COMPLEXITY_LIMIT_EXCEEDED with %q", rejected, want)
	}
}
