// SPDX-License-Identifier: Elastic-2.0

package graph_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"

	"github.com/gopherium/alphone/internal/role"
)

// scopeDirective names the directive every root field declares its area with.
const scopeDirective = "scope"

// rootOperations names the types whose fields a caller reaches directly.
var rootOperations = map[string]bool{"Query": true, "Mutation": true, "Subscription": true}

// scopeGlobs returns every SDL glob a root field may be declared in.
func scopeGlobs(t *testing.T) []string {
	t.Helper()
	globs := []string{filepath.Join("schema", "*.graphqls")}
	for _, dir := range pluginGraphDirs(t) {
		globs = append(globs, filepath.Join(dir, "*.graphqls"))
	}
	return globs
}

// scopedFieldsIn checks the scope of every root field of one SDL source and counts them.
func scopedFieldsIn(t *testing.T, name, source string) int {
	t.Helper()
	counted, problems := rootFieldScopes(t, name, source)
	for _, problem := range problems {
		t.Errorf("%s: %s", name, problem)
	}
	return counted
}

// scopeProblemsIn returns every scope problem of one SDL source.
func scopeProblemsIn(t *testing.T, name, source string) []string {
	t.Helper()
	_, problems := rootFieldScopes(t, name, source)
	return problems
}

// rootFieldScopes counts the root fields of one SDL source beside their scope problems.
func rootFieldScopes(t *testing.T, name, source string) (int, []string) {
	t.Helper()
	doc, err := parser.ParseSchema(&ast.Source{Name: name, Input: source})
	if err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	counted := 0
	var problems []string
	for _, def := range append(doc.Definitions, doc.Extensions...) {
		if !rootOperations[def.Name] {
			continue
		}
		for _, field := range def.Fields {
			problems = append(problems, scopeProblems(def.Name, field, ownsItsCapabilities(name))...)
			counted++
		}
	}
	return counted, problems
}

// ownsItsCapabilities reports whether the SDL belongs to a plugin declaring its own capabilities.
func ownsItsCapabilities(name string) bool {
	slashed := strings.ReplaceAll(name, "\\", "/")
	return strings.HasPrefix(slashed, "../plugins/") || strings.HasPrefix(slashed, "../enterprise/")
}

// scopeProblems reports how one root field's scope declaration falls short.
func scopeProblems(operation string, field *ast.FieldDefinition, ownsCapabilities bool) []string {
	declared := field.Directives.ForName(scopeDirective)
	if declared == nil {
		return []string{fmt.Sprintf(
			"%s.%s declares no @scope, every root field names the area it acts in", operation, field.Name)}
	}
	var problems []string
	if area := declared.Arguments.ForName("area"); area == nil || area.Value.Raw == "" {
		problems = append(problems, fmt.Sprintf("%s.%s declares an empty area", operation, field.Name))
	}
	problems = append(problems, accessProblems(operation, field, declared)...)
	problems = append(problems, capabilityProblems(operation, field, declared, ownsCapabilities)...)
	return problems
}

// capabilityProblems reports how one root field's capability declaration falls short.
func capabilityProblems(
	operation string,
	field *ast.FieldDefinition,
	declared *ast.Directive,
	ownsCapabilities bool,
) []string {
	if admin := declared.Arguments.ForName("admin"); admin != nil && admin.Value.Raw == "true" {
		return []string{fmt.Sprintf(
			"%s.%s reserves itself with admin: true, want a capability the role table knows",
			operation, field.Name)}
	}
	needed := declared.Arguments.ForName("capability")
	if needed == nil {
		return nil
	}
	if needed.Value.Kind == ast.NullValue || needed.Value.Raw == "" {
		return []string{fmt.Sprintf("%s.%s declares an empty capability", operation, field.Name)}
	}
	if ownsCapabilities {
		return nil
	}
	if !slices.Contains(role.Capabilities(), role.Capability(needed.Value.Raw)) {
		return []string{fmt.Sprintf("%s.%s needs capability %q, which the role table does not know",
			operation, field.Name, needed.Value.Raw)}
	}
	return nil
}

// accessProblems reports how one root field's write flag falls short of its operation.
func accessProblems(operation string, field *ast.FieldDefinition, declared *ast.Directive) []string {
	write := declared.Arguments.ForName("write")
	if write == nil {
		return []string{fmt.Sprintf("%s.%s declares no write access", operation, field.Name)}
	}
	if want := operation == "Mutation"; (write.Value.Raw == "true") != want {
		return []string{fmt.Sprintf("%s.%s declares write %s, want %v for a %s field",
			operation, field.Name, write.Value.Raw, want, operation)}
	}
	return nil
}

func TestEveryRootFieldDeclaresTheScopeItNeeds(t *testing.T) {
	t.Parallel()

	counted := 0
	for _, glob := range scopeGlobs(t) {
		files, err := filepath.Glob(glob)
		if err != nil {
			t.Fatalf("globbing %s: %v", glob, err)
		}
		for _, path := range files {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading %s: %v", path, err)
			}
			counted += scopedFieldsIn(t, path, string(raw))
		}
	}

	if counted == 0 {
		t.Fatal("checked 0 root fields, the globs are broken")
	}
}

func TestScopeCheckingSeesEveryDeclarationForm(t *testing.T) {
	t.Parallel()

	synthetic := `
type Query { one: String! @scope(area: "meta", write: false) }
extend type Mutation { two: String! @scope(area: "meta", write: true) }
extend type Subscription { three: String! @scope(area: "events", write: false) }
type Contact { unscoped: String! }
`

	if got := scopedFieldsIn(t, "synthetic", synthetic); got != 3 {
		t.Errorf("counted %d root fields, want 3 across every operation, types excluded", got)
	}
}

func TestScopeCheckingAcceptsACapability(t *testing.T) {
	t.Parallel()

	synthetic := `
type Mutation { one: String! @scope(area: "users", write: true, capability: "manage_users") }
extend type Mutation { two: String! @scope(area: "users", write: true, admin: false) }
`

	if got := scopedFieldsIn(t, "synthetic", synthetic); got != 2 {
		t.Errorf("counted %d root fields, want 2 with the capability present", got)
	}
}

func TestOnlyUserManagementNeedsTheManageUsersCapability(t *testing.T) {
	t.Parallel()

	reserved := map[string]bool{}
	files, err := filepath.Glob(filepath.Join("schema", "*.graphqls"))
	if err != nil {
		t.Fatalf("globbing the core schema: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("globbed 0 core schema files, the glob is broken")
	}
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		for _, field := range gatedFieldsIn(t, path, string(raw)) {
			reserved[field] = true
		}
	}

	want := map[string]bool{
		"setUserDisabled": true,
		"setUserRole":     true,
		"invite":          true,
		"resendInvite":    true,
	}
	if len(reserved) != len(want) {
		t.Errorf("fields needing manage_users = %v, want %v", reserved, want)
	}
	for field := range want {
		if !reserved[field] {
			t.Errorf("%s needs no capability, want user management gated on manage_users", field)
		}
	}
	if reserved["users"] {
		t.Error("the users listing needs a capability, want a member reading its colleagues")
	}
}

// gatedFieldsIn names the root fields of one SDL source needing the manage users capability.
func gatedFieldsIn(t *testing.T, name, source string) []string {
	t.Helper()
	doc, err := parser.ParseSchema(&ast.Source{Name: name, Input: source})
	if err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	var reserved []string
	for _, def := range append(doc.Definitions, doc.Extensions...) {
		if !rootOperations[def.Name] {
			continue
		}
		for _, field := range def.Fields {
			declared := field.Directives.ForName(scopeDirective)
			if declared == nil {
				continue
			}
			needed := declared.Arguments.ForName("capability")
			if needed != nil && needed.Value.Raw == string(role.ManageUsers) {
				reserved = append(reserved, field.Name)
			}
		}
	}
	return reserved
}

func TestScopeCheckingFlagsACapabilityTheTableDoesNotKnow(t *testing.T) {
	t.Parallel()

	synthetic := `type Mutation { one: String! @scope(area: "users", write: true, capability: "manage_moons") }`

	if got := scopeProblemsIn(t, "synthetic", synthetic); len(got) == 0 {
		t.Error("a capability no role holds raised no problem, want it flagged")
	}
}

func TestScopeCheckingAcceptsACapabilityAPluginOwns(t *testing.T) {
	t.Parallel()

	synthetic := `type Mutation { one: String! @scope(area: "tenants", write: true, capability: "manage_tenants") }`

	if got := scopeProblemsIn(t, "../enterprise/tenancy/graph/schema.graphqls", synthetic); len(got) != 0 {
		t.Errorf("problems = %v, want a plugin free to name a capability it declares itself", got)
	}
}

func TestScopeCheckingAcceptsAPluginPathSeparatedByBackslashes(t *testing.T) {
	t.Parallel()

	synthetic := `type Mutation { one: String! @scope(area: "tenants", write: true, capability: "manage_tenants") }`

	if got := scopeProblemsIn(t, `..\enterprise\tenancy\graph\schema.graphqls`, synthetic); len(got) != 0 {
		t.Errorf("problems = %v, want a plugin recognised whichever separator the host globs with", got)
	}
}

func TestScopeCheckingFlagsANullCapability(t *testing.T) {
	t.Parallel()

	synthetic := `type Mutation { one: String! @scope(area: "users", write: true, capability: null) }`

	if got := scopeProblemsIn(t, "synthetic", synthetic); len(got) == 0 {
		t.Error("a null capability raised no problem, want a field naming one the table knows")
	}
}

func TestScopeCheckingRefusesTheBareAdminFlag(t *testing.T) {
	t.Parallel()

	synthetic := `type Mutation { one: String! @scope(area: "users", write: true, admin: true) }`

	if got := scopeProblemsIn(t, "synthetic", synthetic); len(got) == 0 {
		t.Error("a field reserved with admin: true raised no problem, want a capability required")
	}
}
