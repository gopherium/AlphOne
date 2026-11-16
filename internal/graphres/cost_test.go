// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/complexity"
	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	gqlparser "github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/validator/rules"

	"github.com/gopherium/alphone/internal/graphres"
)

// pricedDocument is a query document with the variables it is priced with.
type pricedDocument struct {
	document  string
	variables map[string]any
}

// pricedDocuments names each document by the condition its price proves.
func pricedDocuments() map[string]pricedDocument {
	return map[string]pricedDocument{
		"plain fields":                    {document: `{ version locale tenant { id name } }`},
		"a mutation":                      {document: `mutation { setLocale(locale: "en") }`},
		"a page sized by a limit literal": {document: `{ contactPage(limit: 7) { items { id name } } }`},
		"a page sized by a limit variable": {
			document:  `query($n: Int) { contactPage(limit: $n) { items { id } } }`,
			variables: map[string]any{"n": int64(9)},
		},
		"a page of tasks under each row of a contact page": {
			document: `{ contactPage(limit: 20) { items { id tasks(first: 5) { edges { node { id title } } } } } }`,
		},
		"a named fragment spread once": {
			document: `{ contactPage(limit: 3) { items { ...Row } } } fragment Row on Contact { id name }`,
		},
		"one named fragment spread several times": {
			document: `{ a: contactPage(limit: 3) { items { ...Row } } b: contactPage(limit: 4) { items { ...Row ...Row } }
				contact(id: "00000000-0000-0000-0000-000000000001") { ...Row } } fragment Row on Contact { id name }`,
		},
		"fragments spreading fragments": {
			document: `{ contactPage(limit: 2) { items { ...Outer ...Outer } } }
				fragment Outer on Contact { ...Inner tasks(first: 3) { edges { node { ...TaskRow } } } }
				fragment Inner on Contact { id name }
				fragment TaskRow on Task { id title contact { ...Inner } }`,
		},
		"fragments each spreading the next twice": {document: doublingFragments(6)},
		"inline fragments": {
			document: `{ contactPage(limit: 2) { items {
				... on Contact { id } ... { name tasks(first: 4) { edges { cursor } } }
			} } }`,
		},
		"fields under skip and include": {
			document: `query($on: Boolean!) { version @skip(if: true) locale @include(if: $on)
				contactPage(limit: 4) @include(if: false) { items { id } } }`,
			variables: map[string]any{"on": false},
		},
		"typename fields": {document: `{ __typename contactPage(limit: 2) { items { __typename } } }`},
		"a type read":     {document: `{ __type(name: "Contact") { name fields { name type { name ofType { name } } } } }`},
		"a schema read":   {document: `{ __schema { types { name fields { name } } } version }`},
		"aliases": {
			document: `{ first: contactPage(limit: 2) { items { id } }
				second: contactPage(limit: 3) { items { one: name two: id } } }`,
		},
		"a price past the largest int": {
			document: `query($n: Int) { whatsAppConversations(limit: $n) { id status }
				importJob(id: "00000000-0000-0000-0000-000000000001") { rows(limit: $n) { id } } }`,
			variables: map[string]any{"n": int64(math.MaxInt64)},
		},
	}
}

// doublingFragments returns a document reading the Query type through levels fragments, each spreading the next twice.
func doublingFragments(levels int) string {
	var document strings.Builder
	document.WriteString(`{ __type(name: "Query") { ...F1 } }`)
	for level := 1; level <= levels; level++ {
		fmt.Fprintf(&document, " fragment F%d on __Type { ofType { ...F%d ...F%d } }", level, level+1, level+1)
	}
	fmt.Fprintf(&document, " fragment F%d on __Type { name }", levels+1)
	return document.String()
}

// pricedOperation returns the operation of the document validated against schema.
func pricedOperation(t *testing.T, schema graphql.ExecutableSchema, document string) *ast.OperationDefinition {
	t.Helper()
	query, err := gqlparser.LoadQueryWithRules(schema.Schema(), document, rules.NewDefaultRules())
	if err != nil {
		t.Fatalf("parsing %s: %v", document, err)
	}
	return query.Operations[0]
}

func TestOperationCostMatchesTheCostGqlgenGives(t *testing.T) {
	t.Parallel()

	schema := graphres.ExecutableSchema(composedRoot(t, &graphres.Resolver{}))

	for name, priced := range pricedDocuments() {
		operation := pricedOperation(t, schema, priced.document)

		got := graphres.OperationCost(t.Context(), schema, operation, priced.variables)

		if want := complexity.Calculate(t.Context(), schema, operation, priced.variables); got != want {
			t.Errorf("%s costs %d, want the %d gqlgen gives", name, got, want)
		}
	}
}

// countingSchema is an executable schema counting the fields it prices.
type countingSchema struct {
	graphql.ExecutableSchema
	priced int
}

// Complexity counts the field before the schema prices it.
func (s *countingSchema) Complexity(
	ctx context.Context, typeName, field string, child int, args map[string]any,
) (int, bool) {
	s.priced++
	return s.ExecutableSchema.Complexity(ctx, typeName, field, child, args)
}

func TestOperationCostPricesEachNamedFragmentOnce(t *testing.T) {
	t.Parallel()

	schema := &countingSchema{ExecutableSchema: graphres.ExecutableSchema(composedRoot(t, &graphres.Resolver{}))}
	operation := pricedOperation(t, schema, doublingFragments(40))

	got := graphres.OperationCost(t.Context(), schema, operation, nil)

	if got != 1<<41 {
		t.Errorf("40 levels of doubled fragments cost %d, want %d", got, 1<<41)
	}
	if schema.priced != 42 {
		t.Errorf("priced %d fields, want the 42 fields the document holds", schema.priced)
	}
}

func TestTheComposedSchemaDeclaresNoInterfaceSoTheCostPricesFieldsAsAskedOnObjectTypes(t *testing.T) {
	t.Parallel()

	for name, definition := range composedSchema(t).Types {
		if definition.Kind == ast.Interface {
			t.Errorf("%s is an interface, want only object types for the cost to price fields on as asked", name)
		}
	}
}

func TestTheCostLimitNamesItself(t *testing.T) {
	t.Parallel()

	if got := (&graphres.CostLimit{}).ExtensionName(); got != "CostLimit" {
		t.Errorf("ExtensionName() = %q, want CostLimit", got)
	}
}

// limitedServer returns a graph server over schema answering posts under the limit extension.
func limitedServer(schema graphql.ExecutableSchema, limit graphql.HandlerExtension) http.Handler {
	srv := handler.New(schema)
	srv.AddTransport(transport.POST{})
	srv.Use(limit)
	return srv
}

// postedAnswer returns the status and the body the server answers the request body with.
func postedAnswer(t *testing.T, srv http.Handler, body map[string]any) (int, string) {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encoding the request: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	srv.ServeHTTP(recorder, request)
	return recorder.Code, recorder.Body.String()
}

// limitedRequest is a request body with a part of the answer the limit gives it.
type limitedRequest struct {
	body     map[string]any
	answered string
}

// limitedRequests names each request by what the limit answers it.
func limitedRequests() map[string]limitedRequest {
	twoOperations := `query Cheap { __typename } query Dear { contactPage(limit: 2000) { items { id name } } }`
	rejected := `operation has complexity 6000, which exceeds the limit of 2500`
	return map[string]limitedRequest{
		"a page past the limit": {
			body: map[string]any{"query": `{ contactPage(limit: 2000) { items { id name } } }`}, answered: rejected,
		},
		"a variable past the limit": {body: map[string]any{
			"query":     `query($n: Int) { contactPage(limit: $n) { items { id name } } }`,
			"variables": map[string]any{"n": 2000},
		}, answered: rejected},
		"the named operation past the limit": {
			body: map[string]any{"query": twoOperations, "operationName": "Dear"}, answered: rejected,
		},
		"the named operation within the limit": {
			body: map[string]any{"query": twoOperations, "operationName": "Cheap"}, answered: `"__typename":"Query"`,
		},
		"a typename read": {body: map[string]any{"query": `{ __typename }`}, answered: `"__typename":"Query"`},
	}
}

func TestCostLimitAnswersAsTheLimitOfGqlgen(t *testing.T) {
	t.Parallel()

	schema := graphres.ExecutableSchema(composedRoot(t, &graphres.Resolver{}))
	ours := limitedServer(schema, &graphres.CostLimit{})
	theirs := limitedServer(schema, extension.FixedComplexityLimit(graphres.ComplexityLimit))

	for name, request := range limitedRequests() {
		status, answered := postedAnswer(t, ours, request.body)
		wantStatus, want := postedAnswer(t, theirs, request.body)

		if status != wantStatus || answered != want {
			t.Errorf("%s answered %d %s, want %d %s", name, status, answered, wantStatus, want)
		}
		if !strings.Contains(answered, request.answered) {
			t.Errorf("%s answered %s, want it to hold %s", name, answered, request.answered)
		}
	}
}
