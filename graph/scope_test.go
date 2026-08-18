// SPDX-License-Identifier: Elastic-2.0

package graph_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
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
			problems = append(problems, scopeProblems(def.Name, field)...)
			counted++
		}
	}
	return counted, problems
}

// scopeProblems reports how one root field's scope declaration falls short.
func scopeProblems(operation string, field *ast.FieldDefinition) []string {
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
	if admin := declared.Arguments.ForName("admin"); admin != nil && !isBoolean(admin.Value) {
		problems = append(problems, fmt.Sprintf(
			"%s.%s declares admin %s, want true or false", operation, field.Name, admin.Value.Raw))
	}
	return problems
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

// isBoolean reports whether one directive argument value is a boolean literal.
func isBoolean(value *ast.Value) bool {
	return value != nil && value.Kind == ast.BooleanValue
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

func TestScopeCheckingAcceptsAnAdminFlag(t *testing.T) {
	t.Parallel()

	synthetic := `
type Mutation { one: String! @scope(area: "users", write: true, admin: true) }
extend type Mutation { two: String! @scope(area: "users", write: true, admin: false) }
`

	if got := scopedFieldsIn(t, "synthetic", synthetic); got != 2 {
		t.Errorf("counted %d root fields, want 2 with the admin flag present", got)
	}
}

func TestOnlyUserManagementIsReservedToAdmins(t *testing.T) {
	t.Parallel()

	reserved := map[string]bool{}
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
			for _, field := range adminFieldsIn(t, path, string(raw)) {
				reserved[field] = true
			}
		}
	}

	want := map[string]bool{"createUser": true, "setUserDisabled": true}
	if len(reserved) != len(want) {
		t.Errorf("admin only fields = %v, want %v", reserved, want)
	}
	for field := range want {
		if !reserved[field] {
			t.Errorf("%s is not admin only, want user management reserved to admins", field)
		}
	}
	if reserved["users"] {
		t.Error("the users listing is admin only, want a member reading its colleagues")
	}
}

// adminFieldsIn names the root fields of one SDL source reserved to the admin tier.
func adminFieldsIn(t *testing.T, name, source string) []string {
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
			if admin := declared.Arguments.ForName("admin"); admin != nil && admin.Value.Raw == "true" {
				reserved = append(reserved, field.Name)
			}
		}
	}
	return reserved
}

func TestScopeCheckingFlagsAMalformedAdminFlag(t *testing.T) {
	t.Parallel()

	synthetic := `type Mutation { one: String! @scope(area: "users", write: true, admin: 1) }`

	if got := scopeProblemsIn(t, "synthetic", synthetic); len(got) == 0 {
		t.Error("a malformed admin flag raised no problem, want it flagged")
	}
}
