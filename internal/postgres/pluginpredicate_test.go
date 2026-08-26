// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
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
		tables: []string{"conversations", "messages", "media", "credentials"},
	},
	{
		source: filepath.Join("..", "..", "plugins", "whatsapp", "media.go"),
		schema: "plugin_whatsapp",
		tables: []string{"conversations", "messages", "media", "credentials"},
	},
	{
		source: filepath.Join("..", "..", "plugins", "whatsapp", "credentials.go"),
		schema: "plugin_whatsapp",
		tables: []string{"conversations", "messages", "media", "credentials"},
	},
	{
		source: filepath.Join("..", "..", "plugins", "whatsapp", "seed.go"),
		schema: "plugin_whatsapp",
		tables: []string{"conversations", "messages", "media", "credentials"},
	},
}

// crossTenantStatements names the plugin statements that answer before any tenant is known.
var crossTenantStatements = []string{
	"FROM plugin_whatsapp.credentials WHERE phone_number_id",
	"WHERE med.status = 'pending'",
}

// answersBeforeTheTenant reports whether a statement is one that establishes the tenant.
func answersBeforeTheTenant(statement string) bool {
	for _, named := range crossTenantStatements {
		if strings.Contains(statement, named) {
			return true
		}
	}
	return false
}

// sqlLiterals returns every statement a source builds, resolving the consts it names.
func sqlLiterals(t *testing.T, source string) []string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), source, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", source, err)
	}
	consts := stringConsts(parsed)
	var held []string
	ast.Inspect(parsed, func(node ast.Node) bool {
		if declared, ok := node.(*ast.GenDecl); ok && declared.Tok == token.CONST {
			return false
		}
		joined, ok := joinedString(node, consts)
		if ok {
			held = append(held, joined)
			return false
		}
		return true
	})
	return held
}

// stringConsts returns the text every const in a source spells.
func stringConsts(parsed *ast.File) map[string]string {
	held := map[string]string{}
	ast.Inspect(parsed, func(node ast.Node) bool {
		declared, ok := node.(*ast.GenDecl)
		if !ok || declared.Tok != token.CONST {
			return true
		}
		for _, spec := range declared.Specs {
			valued, ok := spec.(*ast.ValueSpec)
			if !ok || len(valued.Names) != 1 || len(valued.Values) != 1 {
				continue
			}
			if text, ok := joinedString(valued.Values[0], held); ok {
				held[valued.Names[0].Name] = text
			}
		}
		return true
	})
	return held
}

// joinedString returns the text one literal, named const or concatenation of them spells.
func joinedString(node ast.Node, consts map[string]string) (string, bool) {
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
	case *ast.Ident:
		text, named := consts[held.Name]
		return text, named
	case *ast.BinaryExpr:
		if held.Op != token.ADD {
			return "", false
		}
		left, leftOK := joinedString(held.X, consts)
		right, rightOK := joinedString(held.Y, consts)
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

	joined, ok := joinedString(parsed, nil)

	if !ok || joined != "SELECT id FROM t WHERE tenant_id = $1" {
		t.Errorf("joinedString() = %q ok=%t, want the whole statement", joined, ok)
	}
}

func TestTheWalkerResolvesAStatementBuiltFromAConst(t *testing.T) {
	t.Parallel()

	source := filepath.Join(t.TempDir(), "store.go")
	held := "package p\n\nconst head = `SELECT id FROM plugin_x.rows`\n\n" +
		"func read() string { return head + ` WHERE id = $1` }\n"
	if err := os.WriteFile(source, []byte(held), 0o600); err != nil {
		t.Fatalf("writing the source: %v", err)
	}

	statements := sqlLiterals(t, source)

	for _, statement := range statements {
		if strings.Contains(statement, "plugin_x.rows") && strings.Contains(statement, "WHERE id = $1") {
			return
		}
	}
	t.Errorf("statements = %q, want the const joined with the appended clause", statements)
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
			if tenantSafe(statement) || answersBeforeTheTenant(statement) {
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

func TestEveryCrossTenantStatementStillExists(t *testing.T) {
	t.Parallel()

	var walked []string
	for _, held := range pluginStores {
		walked = append(walked, sqlLiterals(t, held.source)...)
	}

	for _, named := range crossTenantStatements {
		found := false
		for _, statement := range walked {
			if strings.Contains(statement, named) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%q is exempted but no longer exists, want the exemption dropped", named)
		}
	}
}
