// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"testing"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/role"
)

// scopedSchema is the miniature schema every scope gate test runs against.
const scopedSchema = `
directive @scope(area: String!, write: Boolean!, admin: Boolean = false, capability: String) on FIELD_DEFINITION
type Query {
  contacts: String! @scope(area: "contacts", write: false)
  me: String! @scope(area: "auth", write: false)
  apiTokens: String! @scope(area: "tokens", write: false)
  users: String! @scope(area: "users", write: false)
  unscoped: String!
}
type Mutation {
  createContact: String! @scope(area: "contacts", write: true)
  createTask: String! @scope(area: "tasks", write: true)
  createUser: String! @scope(area: "users", write: true, capability: "manage_users")
  setUserRole: String! @scope(area: "users", write: true, capability: "manage_users")
  setUserDisabled: String! @scope(area: "users", write: true, admin: true)
  needsReports: String! @scope(area: "users", write: true, capability: "manage_reports")
}
type Subscription {
  coreEvent: String! @scope(area: "events", write: false)
}
`

// newScopeMap builds the scope map of the miniature schema.
func newScopeMap(t *testing.T) graphres.ScopeMap {
	t.Helper()
	schema, err := gqlparser.LoadSchema(&ast.Source{Name: "scoped", Input: scopedSchema})
	if err != nil {
		t.Fatalf("loading the scoped schema: %v", err)
	}
	return graphres.NewScopeMap(schema)
}

func TestScopeMapReadsEveryRootOperation(t *testing.T) {
	t.Parallel()

	scopes := newScopeMap(t)

	if got, want := len(scopes), 11; got != want {
		t.Errorf("scope map holds %d fields, want %d across query, mutation and subscription", got, want)
	}
	if !scopes.Allows(ast.Query, "contacts", apitoken.ParseScopes("contacts:read")) {
		t.Error("contacts:read does not reach the contacts query, want it allowed")
	}
	if scopes.Allows(ast.Mutation, "createContact", apitoken.ParseScopes("contacts:read")) {
		t.Error("contacts:read reaches createContact, want writing refused")
	}
	if !scopes.Allows(ast.Subscription, "coreEvent", apitoken.ParseScopes("events:read")) {
		t.Error("events:read does not reach coreEvent, want it allowed")
	}
}

func TestScopeMapReadsASchemaWithoutEveryRootType(t *testing.T) {
	t.Parallel()

	source := `
directive @scope(area: String!, write: Boolean!) on FIELD_DEFINITION
type Query { version: String! @scope(area: "meta", write: false) }
`
	schema, err := gqlparser.LoadSchema(&ast.Source{Name: "queryonly", Input: source})
	if err != nil {
		t.Fatalf("loading the query only schema: %v", err)
	}

	scopes := graphres.NewScopeMap(schema)

	if got, want := len(scopes), 1; got != want {
		t.Errorf("scope map holds %d fields, want %d", got, want)
	}
	if !scopes.Allows(ast.Query, "version", apitoken.ParseScopes("meta:read")) {
		t.Error("meta:read does not reach version, want it allowed")
	}
}

func TestScopeMapRefusesAFieldItDoesNotKnow(t *testing.T) {
	t.Parallel()

	scopes := newScopeMap(t)

	if scopes.Allows(ast.Query, "unscoped", apitoken.Full()) {
		t.Error("an unscoped field is allowed under the wildcard, want it refused, the gate fails closed")
	}
	if scopes.Allows(ast.Query, "neverDeclared", apitoken.Full()) {
		t.Error("an unknown field is allowed, want it refused")
	}
}

func TestScopeMapLetsEveryTokenReachTheAuthArea(t *testing.T) {
	t.Parallel()

	scopes := newScopeMap(t)

	if !scopes.Allows(ast.Query, "me", apitoken.ParseScopes("contacts:read")) {
		t.Error("a token cannot read me, want the auth area open to any authenticated caller")
	}
}

func TestScopeMapRefusesTokenManagementToEveryToken(t *testing.T) {
	t.Parallel()

	scopes := newScopeMap(t)

	if scopes.Allows(ast.Query, "apiTokens", apitoken.Full()) {
		t.Error("a wildcard token reaches token management, want a session required")
	}
	if scopes.Allows(ast.Query, "apiTokens", apitoken.ParseScopes("tokens:read")) {
		t.Error("a tokens scoped token reaches token management, want a session required")
	}
}

func TestScopeMapReadsTheCapabilityAFieldDeclares(t *testing.T) {
	t.Parallel()

	scopes := newScopeMap(t)

	if got := scopes.Capability(ast.Mutation, "createUser"); got != role.ManageUsers {
		t.Errorf("createUser needs %q, want %q", got, role.ManageUsers)
	}
	if got := scopes.Capability(ast.Mutation, "setUserRole"); got != role.ManageUsers {
		t.Errorf("setUserRole needs %q, want %q", got, role.ManageUsers)
	}
	if got := scopes.Capability(ast.Mutation, "setUserDisabled"); got != role.ManageUsers {
		t.Errorf("setUserDisabled needs %q, want a bare admin flag read as %q", got, role.ManageUsers)
	}
	if got := scopes.Capability(ast.Mutation, "createContact"); got != "" {
		t.Errorf("createContact needs %q, want no capability of an unmarked field", got)
	}
	if got := scopes.Capability(ast.Query, "users"); got != "" {
		t.Errorf("the users listing needs %q, want a member reading its colleagues", got)
	}
}

func TestScopeMapReadsANullCapabilityAsNoneDeclared(t *testing.T) {
	t.Parallel()

	schema, err := gqlparser.LoadSchema(&ast.Source{Name: "null", Input: `
directive @scope(area: String!, write: Boolean!, admin: Boolean = false, capability: String) on FIELD_DEFINITION
type Mutation {
  bare: String! @scope(area: "users", write: true, capability: null)
  flagged: String! @scope(area: "users", write: true, admin: true, capability: null)
}
`})
	if err != nil {
		t.Fatalf("loading the schema: %v", err)
	}
	scopes := graphres.NewScopeMap(schema)

	if got := scopes.Capability(ast.Mutation, "bare"); got != "" {
		t.Errorf("bare needs %q, want none, a null names no capability", got)
	}
	if got := scopes.Capability(ast.Mutation, "flagged"); got != role.ManageUsers {
		t.Errorf("flagged needs %q, want the admin flag to still stand in", got)
	}
}

func TestScopeMapReadsTheAdminFlagAFieldDeclares(t *testing.T) {
	t.Parallel()

	scopes := newScopeMap(t)

	if !scopes.AdminOnly(ast.Mutation, "setUserDisabled") {
		t.Error("setUserDisabled is not admin only, want the declared flag read")
	}
	if scopes.AdminOnly(ast.Mutation, "createContact") {
		t.Error("createContact is admin only, want an unmarked field open to every tier")
	}
	if scopes.AdminOnly(ast.Query, "users") {
		t.Error("the users listing is admin only, want a member reading its colleagues")
	}
	if scopes.AdminOnly(ast.Query, "unscoped") {
		t.Error("an unscoped field is admin only, want a field nobody marked open to every tier")
	}
	if scopes.AdminOnly(ast.Query, "neverDeclared") {
		t.Error("an unknown field is admin only, want the role check to leave it to the token check")
	}
}

func TestScopeMapNamesTheScopeAFieldNeeds(t *testing.T) {
	t.Parallel()

	scopes := newScopeMap(t)

	if got, want := scopes.Needed(ast.Mutation, "createTask"), "tasks:write"; got != want {
		t.Errorf("Needed() = %q, want %q", got, want)
	}
	if got, want := scopes.Needed(ast.Query, "contacts"), "contacts:read"; got != want {
		t.Errorf("Needed() = %q, want %q", got, want)
	}
	if got, want := scopes.Needed(ast.Query, "neverDeclared"), "neverDeclared"; got != want {
		t.Errorf("Needed() = %q, want %q for a field the schema does not scope", got, want)
	}
}
