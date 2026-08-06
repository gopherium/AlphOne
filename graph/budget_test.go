// SPDX-License-Identifier: Elastic-2.0

package graph_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

// Root field budgets per schema owner.
const (
	coreRootFieldBudget   = 20
	pluginRootFieldBudget = 10
)

// rootFieldCount counts the Query and Mutation fields declared in the SDL files matching glob.
func rootFieldCount(t *testing.T, glob string) int {
	t.Helper()
	files, err := filepath.Glob(glob)
	if err != nil {
		t.Fatalf("globbing %s: %v", glob, err)
	}
	count := 0
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		count += rootFieldsIn(t, path, string(raw))
	}
	return count
}

// rootFieldsIn counts the Query and Mutation fields of one SDL source.
func rootFieldsIn(t *testing.T, name, source string) int {
	t.Helper()
	doc, err := parser.ParseSchema(&ast.Source{Name: name, Input: source})
	if err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	count := 0
	for _, def := range append(doc.Definitions, doc.Extensions...) {
		if def.Name == "Query" || def.Name == "Mutation" {
			count += len(def.Fields)
		}
	}
	return count
}

func TestCoreStaysWithinItsRootFieldBudget(t *testing.T) {
	t.Parallel()

	count := rootFieldCount(t, "schema/*.graphqls")

	if count == 0 {
		t.Fatal("counted 0 core root fields, the glob is broken")
	}
	if count > coreRootFieldBudget {
		t.Errorf("core declares %d root fields, the budget is %d, moves are deliberate edits", count, coreRootFieldBudget)
	}
}

func TestEveryPluginStaysWithinItsRootFieldBudget(t *testing.T) {
	t.Parallel()

	dirs, err := filepath.Glob("../plugins/*/graph")
	if err != nil {
		t.Fatalf("globbing plugin schemas: %v", err)
	}
	for _, dir := range dirs {
		count := rootFieldCount(t, filepath.Join(dir, "*.graphqls"))
		if count > pluginRootFieldBudget {
			t.Errorf("%s declares %d root fields, the budget is %d", dir, count, pluginRootFieldBudget)
		}
	}
}

func TestRootFieldCountingSeesEveryDeclarationForm(t *testing.T) {
	t.Parallel()

	synthetic := `
type Query { one: String! two: String! }
extend type Query { three: String! }
extend type Mutation { four: String! }
`

	if got := rootFieldsIn(t, "synthetic", synthetic); got != 4 {
		t.Errorf("synthetic root fields = %d, want 4 across base and extensions", got)
	}
}
