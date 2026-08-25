// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// pluginStores lists every plugin store beside the schema and tables it must filter.
var pluginStores = []struct {
	source string
	schema string
	tables []string
}{
	{
		source: filepath.Join("..", "..", "plugins", "fields", "store.go"),
		schema: "plugin_fields",
		tables: []string{"definitions", "contact_values"},
	},
	{
		source: filepath.Join("..", "..", "plugins", "importer", "store.go"),
		schema: "plugin_importer",
		tables: []string{"imports", "import_rows"},
	},
	{
		source: filepath.Join("..", "..", "plugins", "whatsapp", "store.go"),
		schema: "plugin_whatsapp",
		tables: []string{"conversations", "messages", "media"},
	},
	{
		source: filepath.Join("..", "..", "plugins", "whatsapp", "media.go"),
		schema: "plugin_whatsapp",
		tables: []string{"conversations", "messages", "media"},
	},
}

// sqlLiterals returns every string a source builds, joining concatenated parts.
func sqlLiterals(t *testing.T, source string) []string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), source, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", source, err)
	}
	var held []string
	ast.Inspect(parsed, func(node ast.Node) bool {
		joined, ok := joinedString(node)
		if ok {
			held = append(held, joined)
			return false
		}
		return true
	})
	return held
}

// joinedString returns the text one literal or concatenation of literals spells.
func joinedString(node ast.Node) (string, bool) {
	switch held := node.(type) {
	case *ast.BasicLit:
		if held.Kind != token.STRING {
			return "", false
		}
		text, err := strconv.Unquote(held.Value)
		if err != nil {
			return "", false
		}
		return text, true
	case *ast.BinaryExpr:
		if held.Op != token.ADD {
			return "", false
		}
		left, leftOK := joinedString(held.X)
		right, rightOK := joinedString(held.Y)
		if !leftOK && !rightOK {
			return "", false
		}
		return left + right, true
	}
	return "", false
}

func TestTheWalkerJoinsAConcatenatedStatement(t *testing.T) {
	t.Parallel()

	parsed, err := parser.ParseExpr(`"SELECT id FROM t " + "WHERE tenant_id = $1"`)
	if err != nil {
		t.Fatalf("parsing the expression: %v", err)
	}

	joined, ok := joinedString(parsed)

	if !ok || joined != "SELECT id FROM t WHERE tenant_id = $1" {
		t.Errorf("joinedString() = %q ok=%t, want the whole statement", joined, ok)
	}
}

func TestEveryPluginStatementFiltersItsTableByTenant(t *testing.T) {
	t.Parallel()

	for _, held := range pluginStores {
		statements := sqlLiterals(t, held.source)
		if len(statements) == 0 {
			t.Errorf("%s: no statements parsed, want the store walked", held.source)
			continue
		}
		for _, statement := range statements {
			if strings.Contains(statement, "tenant_id") {
				continue
			}
			for _, table := range held.tables {
				named := regexp.MustCompile(`\b` + held.schema + `\.` + table + `\b`)
				if named.MatchString(statement) {
					t.Errorf("%s touches %s.%s without filtering by tenant_id: %s",
						held.source, held.schema, table, statement)
				}
			}
		}
	}
}
