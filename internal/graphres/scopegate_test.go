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

	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/credential"
	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/role"
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

// gatedAsRole runs one operation through the scope gate as a session standing in one tier.
func gatedAsRole(t *testing.T, query string, tier role.Role) *graphql.Response {
	t.Helper()
	return gatedWith(t, query, standingAs(t.Context(), tier))
}

// standingAs returns ctx carrying an identity standing in one tier.
func standingAs(ctx context.Context, tier role.Role) context.Context {
	return authkit.WithIdentity(ctx, authkit.Identity{Role: tier.String()})
}

// gatedAsTokenOf runs one operation through the gate as a token whose owner stands in one tier.
func gatedAsTokenOf(t *testing.T, query string, held apitoken.Scopes, tier role.Role) *graphql.Response {
	t.Helper()
	ctx := standingAs(t.Context(), tier)
	return gatedWith(t, query, credential.WithToken(ctx, credential.Token{Name: "probe", Scopes: held}))
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

func TestScopeGateHoldsASessionToNoScopeAtAll(t *testing.T) {
	t.Parallel()

	answered := gatedWith(t, `mutation { createContact }`, t.Context())

	if len(answered.Errors) != 0 {
		t.Errorf("errors = %v, want none, a session is the authority tokens narrow", answered.Errors)
	}
}

func TestScopeGateRefusesAnAdminFieldToAMemberSession(t *testing.T) {
	t.Parallel()

	answered := gatedAsRole(t, `mutation { createUser }`, role.Member)

	if got, want := refusalOf(t, answered), "admin required"; got != want {
		t.Errorf("refusal = %q, want %q", got, want)
	}
	if got := answered.Errors[0].Extensions["code"]; got != "UNAUTHORIZED" {
		t.Errorf("code = %v, want UNAUTHORIZED", got)
	}
	if got, want := answered.Errors[0].Extensions["scope"], "users:write"; got != want {
		t.Errorf("scope = %v, want %q, a caller learns what the field wanted", got, want)
	}
	if got, want := answered.Errors[0].Extensions["capability"], "manage_users"; got != want {
		t.Errorf("capability = %v, want %q, a caller learns what its role lacked", got, want)
	}
}

func TestScopeGateNamesTheCapabilityARoleLacks(t *testing.T) {
	t.Parallel()

	answered := gatedAsRole(t, `mutation { needsReports }`, role.Admin)

	if got, want := answered.Errors[0].Extensions["capability"], "manage_reports"; got != want {
		t.Errorf("capability = %v, want %q", got, want)
	}
}

func TestAScopeRefusalNamesNoCapability(t *testing.T) {
	t.Parallel()

	answered := gatedAsToken(t, `mutation { createContact }`, apitoken.ParseScopes("tasks:read"))

	if _, named := answered.Errors[0].Extensions["capability"]; named {
		t.Error("a scope refusal named a capability, want the extension only where a role fell short")
	}
}

func TestScopeGateRefusesEveryUserManagementFieldToAMember(t *testing.T) {
	t.Parallel()

	for _, field := range []string{"createUser", "setUserRole", "setUserDisabled"} {
		answered := gatedAsRole(t, `mutation { `+field+` }`, role.Member)

		if got, want := refusalOf(t, answered), "admin required"; got != want {
			t.Errorf("%s refusal = %q, want %q", field, got, want)
		}
	}
}

func TestScopeGateRefusesAFieldWhoseCapabilityTheRoleLacks(t *testing.T) {
	t.Parallel()

	answered := gatedAsRole(t, `mutation { needsReports }`, role.Admin)

	if got, want := refusalOf(t, answered), "admin required"; got != want {
		t.Errorf("refusal = %q, want %q, an admin holding no manage_reports is refused", got, want)
	}
}

func TestScopeGateLetsARoleHoldingTheCapabilityThrough(t *testing.T) {
	t.Parallel()

	answered := gatedAsRole(t, `mutation { needsReports }`, stewardRole)

	if len(answered.Errors) != 0 {
		t.Errorf("errors = %v, want none, the declared role holds manage_reports", answered.Errors)
	}
}

func TestScopeGateLetsAnAdminSessionReachAnAdminField(t *testing.T) {
	t.Parallel()

	answered := gatedAsRole(t, `mutation { createUser }`, role.Admin)

	if len(answered.Errors) != 0 {
		t.Errorf("errors = %v, want none, an admin manages users", answered.Errors)
	}
}

func TestScopeGateReadsAnUnstampedSessionAsAMember(t *testing.T) {
	t.Parallel()

	answered := gatedWith(t, `mutation { createUser }`, t.Context())

	if got, want := refusalOf(t, answered), "admin required"; got != want {
		t.Errorf("refusal = %q, want %q, losing the stamp demotes rather than widens", got, want)
	}
}

func TestScopeGateLetsAMemberSessionWorkTheProduct(t *testing.T) {
	t.Parallel()

	answered := gatedAsRole(t, `mutation { createContact createTask }`, role.Member)

	if len(answered.Errors) != 0 {
		t.Errorf("errors = %v, want none, a member holds no token to be narrowed by", answered.Errors)
	}
}

func TestScopeGateLetsAnAccountHoldingNoRoleWorkTheProduct(t *testing.T) {
	t.Parallel()

	answered := gatedAsRole(t, `mutation { createContact createTask }`, "")

	if len(answered.Errors) != 0 {
		t.Errorf("errors = %v, want none, no field of the product declares a capability", answered.Errors)
	}
}

func TestScopeGateRefusesUserManagementToAnAccountHoldingNoRole(t *testing.T) {
	t.Parallel()

	answered := gatedAsRole(t, `mutation { createUser }`, "")

	if got, want := refusalOf(t, answered), "admin required"; got != want {
		t.Errorf("refusal = %q, want %q", got, want)
	}
}

func TestScopeGateLetsAMemberSessionManageItsOwnTokens(t *testing.T) {
	t.Parallel()

	answered := gatedAsRole(t, `{ apiTokens }`, role.Member)

	if len(answered.Errors) != 0 {
		t.Errorf("errors = %v, want none, token management is session only, not admin only", answered.Errors)
	}
}

func TestScopeGateHoldsAWildcardTokenToItsOwnersRole(t *testing.T) {
	t.Parallel()

	answered := gatedAsTokenOf(t, `mutation { createUser }`, apitoken.Full(), role.Member)

	if got, want := refusalOf(t, answered), "admin required"; got != want {
		t.Errorf("refusal = %q, want %q, effective access is the role and the token together", got, want)
	}
}

func TestScopeGateLetsAnAdminsScopedTokenReachAnAdminField(t *testing.T) {
	t.Parallel()

	answered := gatedAsTokenOf(t,
		`mutation { createUser }`, apitoken.ParseScopes("users:write"), role.Admin)

	if len(answered.Errors) != 0 {
		t.Errorf("errors = %v, want none, effective access is what the role and the token both allow",
			answered.Errors)
	}
}

func TestScopeGateLetsAMemberTokenListTheUsers(t *testing.T) {
	t.Parallel()

	answered := gatedAsTokenOf(t, `{ users }`, apitoken.ParseScopes("users:read"), role.Member)

	if len(answered.Errors) != 0 {
		t.Errorf("errors = %v, want none, a member sees its colleagues", answered.Errors)
	}
}

func TestScopeGateRefusesAnAdminsNarrowTokenOnItsScope(t *testing.T) {
	t.Parallel()

	answered := gatedAsTokenOf(t,
		`mutation { createUser }`, apitoken.ParseScopes("contacts:read"), role.Admin)

	if got, want := refusalOf(t, answered), "scope required: users:write"; got != want {
		t.Errorf("refusal = %q, want %q, the token check answers first", got, want)
	}
}

func TestScopeGateNamesTheScopeBeforeTheTierWhenBothRefuse(t *testing.T) {
	t.Parallel()

	answered := gatedAsTokenOf(t,
		`mutation { createUser }`, apitoken.ParseScopes("contacts:read"), role.Member)

	if got, want := refusalOf(t, answered), "scope required: users:write"; got != want {
		t.Errorf("refusal = %q, want %q, the message the n8n docs quote stays put", got, want)
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
