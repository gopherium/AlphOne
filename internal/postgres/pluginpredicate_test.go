// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// repositoryRoot is the tree holding every plugin root, seen from this package.
var repositoryRoot = filepath.Join("..", "..")

// pluginManifest is the part of a plugin.json the store checks read, beside the folder holding it.
type pluginManifest struct {
	dir     string
	ID      string `json:"id"`
	Backend string `json:"backend"`
}

// pluginManifests returns the manifest of every plugin folder under the named roots of a tree, in folder order.
func pluginManifests(t *testing.T, tree string, roots ...string) []pluginManifest {
	t.Helper()
	var held []pluginManifest
	for _, root := range roots {
		paths, err := filepath.Glob(filepath.Join(tree, root, "*", "plugin.json"))
		if err != nil {
			t.Fatalf("globbing the manifests under %s: %v", root, err)
		}
		for _, path := range paths {
			held = append(held, readManifest(t, path))
		}
	}
	return held
}

// readManifest returns the manifest one plugin.json holds.
func readManifest(t *testing.T, path string) pluginManifest {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	manifest := pluginManifest{dir: filepath.Dir(path)}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return manifest
}

// backendPlugins returns the manifest of every plugin under the plugins root of a tree that names a backend.
func backendPlugins(t *testing.T, tree string) []pluginManifest {
	t.Helper()
	return slices.DeleteFunc(pluginManifests(t, tree, "plugins"), func(manifest pluginManifest) bool {
		return manifest.Backend == ""
	})
}

// unwalked names the plugin folders that hold no store code.
var unwalked = []string{"frontend", "node_modules", "e2e", "testdata"}

// storeSources returns every Go file under one plugin folder that is not a test, the unwalked folders left out.
func storeSources(t *testing.T, dir string) []string {
	t.Helper()
	var sources []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case entry.IsDir() && slices.Contains(unwalked, entry.Name()):
			return filepath.SkipDir
		case strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go"):
			sources = append(sources, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", dir, err)
	}
	return sources
}

// located is one statement beside the source building it.
type located struct {
	source    string
	statement string
}

// pluginStatements returns every statement the store code of one plugin folder builds.
func pluginStatements(t *testing.T, dir string) []located {
	t.Helper()
	var held []located
	for _, source := range storeSources(t, dir) {
		for _, statement := range sqlLiterals(t, source) {
			held = append(held, located{source: source, statement: statement})
		}
	}
	return held
}

// crossTenantFragments returns the fragments one plugin folder lists as statements answering before any tenant.
func crossTenantFragments(t *testing.T, dir string) []string {
	t.Helper()
	path := filepath.Join(dir, "testdata", "crosstenant.txt")
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var fragments []string
	for _, line := range strings.Split(string(raw), "\n") {
		if fragment := strings.TrimSpace(line); fragment != "" {
			fragments = append(fragments, fragment)
		}
	}
	return fragments
}

// pluginTable captures every plugin table a statement names, beside its bare name.
var pluginTable = regexp.MustCompile(`\bplugin_[a-z0-9_]+\.([a-z_][a-z0-9_]*)\b`)

// coreGuardedTable captures every core table a statement names that the tenant filter guards.
var coreGuardedTable = regexp.MustCompile(`\bcore\.(` + strings.Join(tenantGuardedTables, "|") + `)\b`)

// untenantedTables returns every table one statement touches without holding it to the caller's tenant.
func untenantedTables(statement string) []string {
	var touched []string
	for _, named := range slices.Concat(
		pluginTable.FindAllStringSubmatch(statement, -1), coreGuardedTable.FindAllStringSubmatch(statement, -1),
	) {
		if named[1] != "goose_db_version" && !tenantSafe(statement, named[1]) {
			touched = append(touched, named[0])
		}
	}
	slices.Sort(touched)
	return slices.Compact(touched)
}

// namedByAny reports whether any fragment names the statement.
func namedByAny(fragments []string, statement string) bool {
	return slices.ContainsFunc(fragments, func(fragment string) bool { return strings.Contains(statement, fragment) })
}

// matchingStatements counts the distinct statements a fragment names.
func matchingStatements(statements []located, fragment string) int {
	matched := map[string]bool{}
	for _, held := range statements {
		if strings.Contains(held.statement, fragment) {
			matched[held.statement] = true
		}
	}
	return len(matched)
}

// sqlLiterals returns every statement a source builds, resolving each const in the function that declares it.
func sqlLiterals(t *testing.T, source string) []string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), source, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", source, err)
	}
	shared := stringConsts(parsed, map[string]string{})
	var held []string
	for _, declared := range parsed.Decls {
		consts := shared
		if function, ok := declared.(*ast.FuncDecl); ok {
			consts = stringConsts(function, shared)
		}
		held = append(held, literalsIn(declared, consts)...)
	}
	return held
}

// literalsIn returns every statement one declaration builds from the given consts.
func literalsIn(declared ast.Node, consts map[string]string) []string {
	var held []string
	ast.Inspect(declared, func(node ast.Node) bool {
		if constant, ok := node.(*ast.GenDecl); ok && constant.Tok == token.CONST {
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

// stringConsts returns the text every const one scope spells over the consts it inherits, leaving nested functions out.
func stringConsts(scope ast.Node, inherited map[string]string) map[string]string {
	held := maps.Clone(inherited)
	ast.Inspect(scope, func(node ast.Node) bool {
		if function, ok := node.(*ast.FuncDecl); ok && function != scope {
			return false
		}
		if declared, ok := node.(*ast.GenDecl); ok && declared.Tok == token.CONST {
			recordConsts(declared, held)
		}
		return true
	})
	return held
}

// recordConsts adds the text every single-valued const of one declaration spells.
func recordConsts(declared *ast.GenDecl, held map[string]string) {
	for _, spec := range declared.Specs {
		valued, ok := spec.(*ast.ValueSpec)
		if !ok || len(valued.Names) != 1 || len(valued.Values) != 1 {
			continue
		}
		if text, ok := joinedString(valued.Values[0], held); ok {
			held[valued.Names[0].Name] = text
		}
	}
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

func TestTheWalkerReadsEachFunctionsOwnConst(t *testing.T) {
	t.Parallel()

	source := filepath.Join(t.TempDir(), "store.go")
	held := "package p\n\n" +
		"func first() string { const statement = `SELECT id FROM plugin_x.first`\n return statement }\n\n" +
		"func second() string { const statement = `SELECT id FROM plugin_x.second`\n return statement }\n"
	if err := os.WriteFile(source, []byte(held), 0o600); err != nil {
		t.Fatalf("writing the source: %v", err)
	}

	statements := sqlLiterals(t, source)

	for _, table := range []string{"plugin_x.first", "plugin_x.second"} {
		if !slices.ContainsFunc(statements, func(statement string) bool { return strings.Contains(statement, table) }) {
			t.Errorf("statements = %q, want the statement reading %s walked", statements, table)
		}
	}
}

// untenantedStatements returns one finding per plugin statement under a tree that leaves a table unfiltered.
func untenantedStatements(t *testing.T, tree string) []string {
	t.Helper()
	var findings []string
	for _, manifest := range backendPlugins(t, tree) {
		fragments := crossTenantFragments(t, manifest.dir)
		for _, held := range pluginStatements(t, manifest.dir) {
			if namedByAny(fragments, held.statement) {
				continue
			}
			for _, table := range untenantedTables(held.statement) {
				findings = append(findings, fmt.Sprintf("%s touches %s without filtering by tenant_id: %s",
					held.source, table, held.statement))
			}
		}
	}
	return findings
}

// strayFragments returns one finding per cross tenant fragment matching no statement of its plugin, or several.
func strayFragments(t *testing.T, tree string) []string {
	t.Helper()
	var findings []string
	for _, manifest := range backendPlugins(t, tree) {
		statements := pluginStatements(t, manifest.dir)
		for _, fragment := range crossTenantFragments(t, manifest.dir) {
			if matched := matchingStatements(statements, fragment); matched != 1 {
				findings = append(findings, fmt.Sprintf("%s exempts %q, which matches %d of its statements, want one",
					manifest.ID, fragment, matched))
			}
		}
	}
	return findings
}

// schema returns the schema the plugin owns when it migrates, named after its id.
func (manifest pluginManifest) schema() string {
	return "plugin_" + strings.ReplaceAll(manifest.ID, "-", "_")
}

// namesOwnTable reports whether a statement names a table of the plugin's own schema other than its version table.
func (manifest pluginManifest) namesOwnTable(held located) bool {
	return slices.ContainsFunc(pluginTable.FindAllStringSubmatch(held.statement, -1), func(named []string) bool {
		return named[0] == manifest.schema()+"."+named[1] && named[1] != "goose_db_version"
	})
}

// unreadStores returns one finding per plugin under a tree that keeps migrations while no statement names its tables.
func unreadStores(t *testing.T, tree string) []string {
	t.Helper()
	var findings []string
	for _, manifest := range backendPlugins(t, tree) {
		if !keepsMigrations(t, manifest.dir) ||
			slices.ContainsFunc(pluginStatements(t, manifest.dir), manifest.namesOwnTable) {
			continue
		}
		findings = append(findings, fmt.Sprintf(
			"%s keeps migrations but the walk read no statement naming a table of %s, want its statements as Go strings",
			manifest.ID, manifest.schema()))
	}
	return findings
}

// writeTreeFile writes one file of a temporary tree, creating the folders above it.
func writeTreeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("creating the folder of %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// writeStorePlugin writes a plugin folder with a backend whose one source builds each statement given.
func writeStorePlugin(t *testing.T, dir, source string, statements ...string) {
	t.Helper()
	id := filepath.Base(dir)
	writeTreeFile(t, filepath.Join(dir, "plugin.json"), `{"id": "`+id+`", "backend": "example.com/`+id+`"}`)
	held := "package p\n\n"
	for count, statement := range statements {
		held += "func read" + strconv.Itoa(count) + "() string { return `" + statement + "` }\n\n"
	}
	writeTreeFile(t, filepath.Join(dir, source), held)
}

func TestUntenantedStatementsNameTheStoreOfASubfolder(t *testing.T) {
	t.Parallel()

	demo := filepath.Join(t.TempDir(), "plugins", "demo")
	writeStorePlugin(t, demo, filepath.Join("poll", "poll.go"), "SELECT id FROM plugin_demo.items WHERE id = $1")

	findings := untenantedStatements(t, filepath.Dir(filepath.Dir(demo)))

	want := []string{filepath.Join(demo, "poll", "poll.go") +
		" touches plugin_demo.items without filtering by tenant_id: SELECT id FROM plugin_demo.items WHERE id = $1"}
	if !slices.Equal(findings, want) {
		t.Errorf("findings = %q, want %q", findings, want)
	}
}

func TestUntenantedStatementsNameACoreTableAPluginLeavesUnfiltered(t *testing.T) {
	t.Parallel()

	demo := filepath.Join(t.TempDir(), "plugins", "demo")
	writeStorePlugin(t, demo, "store.go", "SELECT id FROM core.contacts WHERE id = $1")

	findings := untenantedStatements(t, filepath.Dir(filepath.Dir(demo)))

	want := []string{filepath.Join(demo, "store.go") +
		" touches core.contacts without filtering by tenant_id: SELECT id FROM core.contacts WHERE id = $1"}
	if !slices.Equal(findings, want) {
		t.Errorf("findings = %q, want %q", findings, want)
	}
}

func TestUntenantedStatementsLeaveOutWhatHoldsNoStoreCode(t *testing.T) {
	t.Parallel()

	tree := t.TempDir()
	demo := filepath.Join(tree, "plugins", "demo")
	for _, source := range []string{
		"store_test.go",
		filepath.Join("frontend", "store.go"),
		filepath.Join("frontend", "node_modules", "store.go"),
		filepath.Join("node_modules", "store.go"),
		filepath.Join("e2e", "store.go"),
		filepath.Join("testdata", "store.go"),
	} {
		writeStorePlugin(t, demo, source, "SELECT id FROM plugin_demo.items WHERE id = $1")
	}
	writeStorePlugin(t, demo, "migrate.go", "SELECT max(version_id) FROM plugin_demo.goose_db_version")
	writeStorePlugin(t, filepath.Join(tree, "enterprise", "extra"), "store.go",
		"SELECT id FROM plugin_extra.items WHERE id = $1")
	writeTreeFile(t, filepath.Join(tree, "plugins", "quiet", "plugin.json"), `{"id": "quiet", "frontend": "@x/quiet"}`)
	writeTreeFile(t, filepath.Join(tree, "plugins", "quiet", "store.go"),
		"package p\n\nconst read = `SELECT id FROM plugin_quiet.items WHERE id = $1`\n")

	if findings := untenantedStatements(t, tree); len(findings) != 0 {
		t.Errorf("findings = %q, want tests, frontends, specs, test data, migrations and other roots left out", findings)
	}
}

func TestACrossTenantFragmentSilencesTheStatementItNames(t *testing.T) {
	t.Parallel()

	tree := t.TempDir()
	demo := filepath.Join(tree, "plugins", "demo")
	writeStorePlugin(t, demo, "store.go",
		"SELECT tenant_id FROM plugin_demo.numbers WHERE number = $1",
		"SELECT id FROM plugin_demo.items WHERE id = $1")
	writeTreeFile(t, filepath.Join(demo, "testdata", "crosstenant.txt"), "\nFROM plugin_demo.numbers WHERE number\n\n")

	findings := untenantedStatements(t, tree)

	want := []string{filepath.Join(demo, "store.go") +
		" touches plugin_demo.items without filtering by tenant_id: SELECT id FROM plugin_demo.items WHERE id = $1"}
	if !slices.Equal(findings, want) {
		t.Errorf("findings = %q, want only the statement no fragment names, %q", findings, want)
	}
	if stray := strayFragments(t, tree); len(stray) != 0 {
		t.Errorf("stray fragments = %q, want the blank lines skipped and the fragment matched once", stray)
	}
}

func TestACrossTenantFragmentExemptsNoStatementOfAnotherPlugin(t *testing.T) {
	t.Parallel()

	tree := t.TempDir()
	demo := filepath.Join(tree, "plugins", "demo")
	other := filepath.Join(tree, "plugins", "other")
	writeStorePlugin(t, demo, "store.go", "SELECT tenant_id FROM plugin_demo.numbers WHERE number = $1")
	writeTreeFile(t, filepath.Join(demo, "testdata", "crosstenant.txt"), "WHERE number = $1\n")
	writeStorePlugin(t, other, "store.go", "SELECT tenant_id FROM plugin_other.numbers WHERE number = $1")

	findings := untenantedStatements(t, tree)

	want := []string{filepath.Join(other, "store.go") + " touches plugin_other.numbers without filtering by tenant_id: " +
		"SELECT tenant_id FROM plugin_other.numbers WHERE number = $1"}
	if !slices.Equal(findings, want) {
		t.Errorf("findings = %q, want the other plugin's statement named, %q", findings, want)
	}
}

func TestACrossTenantFragmentMatchingNoStatementOrSeveralIsNamed(t *testing.T) {
	t.Parallel()

	tree := t.TempDir()
	demo := filepath.Join(tree, "plugins", "demo")
	writeStorePlugin(t, demo, "store.go",
		"SELECT tenant_id FROM plugin_demo.numbers WHERE number = $1",
		"SELECT tenant_id FROM plugin_demo.codes WHERE code = $1")
	writeTreeFile(t, filepath.Join(demo, "testdata", "crosstenant.txt"),
		"FROM plugin_demo.gone\nSELECT tenant_id FROM plugin_demo\n")

	stray := strayFragments(t, tree)

	want := []string{
		`demo exempts "FROM plugin_demo.gone", which matches 0 of its statements, want one`,
		`demo exempts "SELECT tenant_id FROM plugin_demo", which matches 2 of its statements, want one`,
	}
	if !slices.Equal(stray, want) {
		t.Errorf("stray fragments = %q, want %q", stray, want)
	}
}

func TestUnreadStoresNameAPluginThatMigratesWhileNoStatementNamesItsTables(t *testing.T) {
	t.Parallel()

	tree := t.TempDir()
	embedded := filepath.Join(tree, "plugins", "demo-notes")
	writeStorePlugin(t, embedded, "migrate.go", "plugin_demo_notes.goose_db_version")
	writeTreeFile(t, filepath.Join(embedded, "migrations", "00001_create_notes.sql"),
		"CREATE TABLE plugin_demo_notes.notes ();\n")
	borrower := filepath.Join(tree, "plugins", "borrower")
	writeStorePlugin(t, borrower, "store.go", "SELECT id FROM plugin_read.items WHERE tenant_id = $1")
	writeTreeFile(t, filepath.Join(borrower, "migrations", "00001_create_loans.sql"),
		"CREATE TABLE plugin_borrower.loans ();\n")
	read := filepath.Join(tree, "plugins", "read")
	writeStorePlugin(t, read, "store.go", "SELECT id FROM plugin_read.items WHERE tenant_id = $1")
	writeTreeFile(t, filepath.Join(read, "migrations", "00001_create_items.sql"), "CREATE TABLE plugin_read.items ();\n")
	writeStorePlugin(t, filepath.Join(tree, "plugins", "stateless"), "plugin.go")
	writeStorePlugin(t, filepath.Join(tree, "enterprise", "extra"), "plugin.go")
	writeTreeFile(t, filepath.Join(tree, "enterprise", "extra", "migrations", "00001_create_items.sql"),
		"CREATE TABLE plugin_extra.items ();\n")

	unread := unreadStores(t, tree)

	want := []string{
		"borrower keeps migrations but the walk read no statement naming a table of plugin_borrower, " +
			"want its statements as Go strings",
		"demo-notes keeps migrations but the walk read no statement naming a table of plugin_demo_notes, " +
			"want its statements as Go strings",
	}
	if !slices.Equal(unread, want) {
		t.Errorf("unread stores = %q, want %q", unread, want)
	}
}

func TestEveryPluginStatementFiltersItsTableByTenant(t *testing.T) {
	t.Parallel()

	if len(backendPlugins(t, repositoryRoot)) == 0 {
		t.Fatal("found no plugin with a backend, want the manifests read")
	}
	for _, finding := range unreadStores(t, repositoryRoot) {
		t.Error(finding)
	}
	for _, finding := range untenantedStatements(t, repositoryRoot) {
		t.Error(finding)
	}
}

func TestEveryCrossTenantStatementStillExists(t *testing.T) {
	t.Parallel()

	for _, finding := range strayFragments(t, repositoryRoot) {
		t.Error(finding)
	}
}
