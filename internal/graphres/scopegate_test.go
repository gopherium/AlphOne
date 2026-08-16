// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"context"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/validator/rules"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/credential"
	"github.com/gopherium/alphone/internal/graphres"
)

// loadScopedSchema parses the miniature schema the gate tests run against.
func loadScopedSchema(t *testing.T) *ast.Schema {
	t.Helper()
	schema, err := gqlparser.LoadSchema(&ast.Source{Name: "scoped", Input: scopedSchema})
	if err != nil {
		t.Fatalf("loading the scoped schema: %v", err)
	}
	return schema
}

// passed is the answer the gate lets through.
func passed(context.Context) graphql.ResponseHandler {
	return graphql.OneShot(&graphql.Response{Data: []byte(`{"reached":true}`)})
}

// gatedAsToken runs one operation through the scope gate as a token holding scopes.
func gatedAsToken(t *testing.T, query string, held apitoken.Scopes) *graphql.Response {
	t.Helper()
	return gatedWith(t, query, credential.WithToken(t.Context(), credential.Token{
		Name: "probe", Scopes: held,
	}))
}

// gatedWith runs one operation through the scope gate on the given context.
func gatedWith(t *testing.T, query string, ctx context.Context) *graphql.Response {
	t.Helper()
	schema := loadScopedSchema(t)
	doc, err := gqlparser.LoadQueryWithRules(schema, query, rules.NewDefaultRules())
	if err != nil {
		t.Fatalf("parsing %q: %v", query, err)
	}
	ctx = graphql.WithOperationContext(ctx, &graphql.OperationContext{
		Doc: doc, Operation: doc.Operations[0], Variables: map[string]any{},
	})
	return graphres.ScopeGate(graphres.NewScopeMap(schema))(ctx, passed)(ctx)
}

// refusalOf returns the single error message of a refused answer.
func refusalOf(t *testing.T, answered *graphql.Response) string {
	t.Helper()
	if len(answered.Errors) != 1 {
		t.Fatalf("errors = %v, want exactly one refusal", answered.Errors)
	}
	return answered.Errors[0].Message
}

func TestScopeGateLetsASessionReachEverything(t *testing.T) {
	t.Parallel()

	answered := gatedWith(t, `mutation { createContact }`, t.Context())

	if len(answered.Errors) != 0 {
		t.Errorf("errors = %v, want none, a session is the authority tokens narrow", answered.Errors)
	}
}

func TestScopeGateLetsATokenReachWhatItHolds(t *testing.T) {
	t.Parallel()

	answered := gatedAsToken(t, `{ contacts }`, apitoken.ParseScopes("contacts:read"))

	if len(answered.Errors) != 0 {
		t.Errorf("errors = %v, want none", answered.Errors)
	}
}

func TestScopeGateRefusesAWriteToAReadToken(t *testing.T) {
	t.Parallel()

	answered := gatedAsToken(t, `mutation { createContact }`, apitoken.ParseScopes("contacts:read"))

	if got := refusalOf(t, answered); !strings.Contains(got, "contacts:write") {
		t.Errorf("refusal = %q, want it to name contacts:write", got)
	}
	if got := answered.Errors[0].Extensions["code"]; got != "UNAUTHORIZED" {
		t.Errorf("code = %v, want UNAUTHORIZED", got)
	}
}

func TestScopeGateSeesThroughATopLevelFragmentSpread(t *testing.T) {
	t.Parallel()

	answered := gatedAsToken(t,
		`mutation { ...writes } fragment writes on Mutation { createContact }`,
		apitoken.ParseScopes("contacts:read"))

	if got := refusalOf(t, answered); !strings.Contains(got, "contacts:write") {
		t.Errorf("refusal = %q, want a fragment wrapped field checked like any other", got)
	}
}

func TestScopeGateSeesThroughAnInlineFragment(t *testing.T) {
	t.Parallel()

	answered := gatedAsToken(t, `{ ... on Query { contacts } }`, apitoken.ParseScopes("tasks:read"))

	if got := refusalOf(t, answered); !strings.Contains(got, "contacts:read") {
		t.Errorf("refusal = %q, want an inline fragment checked like any other", got)
	}
}

func TestScopeGateSeesThroughANestedFragmentSpread(t *testing.T) {
	t.Parallel()

	answered := gatedAsToken(t,
		`{ ...outer } fragment outer on Query { ...inner } fragment inner on Query { contacts }`,
		apitoken.ParseScopes("tasks:read"))

	if got := refusalOf(t, answered); !strings.Contains(got, "contacts:read") {
		t.Errorf("refusal = %q, want nesting to be no escape", got)
	}
}

func TestScopeGateRefusesEveryFieldOfAMixedOperation(t *testing.T) {
	t.Parallel()

	answered := gatedAsToken(t, `{ contacts me }`, apitoken.ParseScopes("tasks:read"))

	if got := refusalOf(t, answered); !strings.Contains(got, "contacts:read") {
		t.Errorf("refusal = %q, want the unheld field to refuse the whole operation", got)
	}
}

func TestScopeGateLetsAnyTokenReachTheAuthArea(t *testing.T) {
	t.Parallel()

	answered := gatedAsToken(t, `{ me }`, apitoken.ParseScopes("tasks:read"))

	if len(answered.Errors) != 0 {
		t.Errorf("errors = %v, want none, every authenticated caller reads its own identity", answered.Errors)
	}
}

func TestScopeGateLetsIntrospectionThrough(t *testing.T) {
	t.Parallel()

	answered := gatedAsToken(t, `{ __schema { queryType { name } } }`, apitoken.ParseScopes("tasks:read"))

	if len(answered.Errors) != 0 {
		t.Errorf("errors = %v, want none, no schema declares the meta fields", answered.Errors)
	}
}

func TestScopeGateRefusesTokenManagementToAWildcardToken(t *testing.T) {
	t.Parallel()

	answered := gatedAsToken(t, `{ apiTokens }`, apitoken.Full())

	if got := refusalOf(t, answered); !strings.Contains(got, "tokens:read") {
		t.Errorf("refusal = %q, want token management to need a session", got)
	}
}

func TestScopeGateRefusesAFieldTheSchemaDoesNotScope(t *testing.T) {
	t.Parallel()

	answered := gatedAsToken(t, `{ unscoped }`, apitoken.Full())

	if got := refusalOf(t, answered); !strings.Contains(got, "unscoped") {
		t.Errorf("refusal = %q, want an unscoped field refused, the gate fails closed", got)
	}
}

func TestScopeGateRefusesAnOperationItCannotRead(t *testing.T) {
	t.Parallel()

	ctx := credential.WithToken(t.Context(), credential.Token{Name: "probe", Scopes: apitoken.Full()})
	ctx = graphql.WithOperationContext(ctx, &graphql.OperationContext{})

	answered := graphres.ScopeGate(graphres.NewScopeMap(loadScopedSchema(t)))(ctx, passed)(ctx)

	if len(answered.Errors) != 1 {
		t.Errorf("errors = %v, want a refusal when there is no operation to read", answered.Errors)
	}
}
