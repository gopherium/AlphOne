// SPDX-License-Identifier: Elastic-2.0

package graph_test

import (
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
	doc, err := parser.ParseSchema(&ast.Source{Name: name, Input: source})
	if err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	counted := 0
	for _, def := range append(doc.Definitions, doc.Extensions...) {
		if !rootOperations[def.Name] {
			continue
		}
		for _, field := range def.Fields {
			checkScope(t, name, def.Name, field)
			counted++
		}
	}
	return counted
}

// checkScope reports whether one root field declares an area and an access matching its operation.
func checkScope(t *testing.T, name, operation string, field *ast.FieldDefinition) {
	t.Helper()
	declared := field.Directives.ForName(scopeDirective)
	if declared == nil {
		t.Errorf("%s: %s.%s declares no @scope, every root field names the area it acts in", name, operation, field.Name)
		return
	}
	area := declared.Arguments.ForName("area")
	if area == nil || area.Value.Raw == "" {
		t.Errorf("%s: %s.%s declares an empty area", name, operation, field.Name)
	}
	write := declared.Arguments.ForName("write")
	if write == nil {
		t.Errorf("%s: %s.%s declares no write access", name, operation, field.Name)
		return
	}
	if want := operation == "Mutation"; (write.Value.Raw == "true") != want {
		t.Errorf("%s: %s.%s declares write %s, want %v for a %s field",
			name, operation, field.Name, write.Value.Raw, want, operation)
	}
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
