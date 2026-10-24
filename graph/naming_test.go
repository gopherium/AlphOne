// SPDX-License-Identifier: Elastic-2.0

package graph_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

// typeNaming is the prefix one plugin's types carry beside the type names it excepts.
type typeNaming struct {
	pluginID   string
	exceptions []string
}

// misnamedTypes reports the types source defines outside the plugin's prefix and its exceptions.
func misnamedTypes(t *testing.T, naming typeNaming, name, source string) []string {
	t.Helper()
	doc, err := parser.ParseSchema(&ast.Source{Name: name, Input: source})
	if err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	var offenses []string
	for _, def := range doc.Definitions {
		if slices.Contains(naming.exceptions, def.Name) || hasPluginPrefix(def.Name, naming.pluginID) {
			continue
		}
		offenses = append(offenses, fmt.Sprintf(
			"%s defines %s, want the %s prefix or a line in testdata/unprefixedtypes.txt", name, def.Name, naming.pluginID,
		))
	}
	return offenses
}

// hasPluginPrefix reports whether the type name starts with the plugin id ignoring case.
func hasPluginPrefix(typeName, pluginID string) bool {
	return len(typeName) >= len(pluginID) && strings.EqualFold(typeName[:len(pluginID)], pluginID)
}

// typeExceptions returns the type names one plugin folder lists in its testdata as free of the plugin prefix.
func typeExceptions(t *testing.T, pluginDir string) []string {
	t.Helper()
	path := filepath.Join(pluginDir, "testdata", "unprefixedtypes.txt")
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var names []string
	for _, line := range strings.Split(string(raw), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// definedTypes returns the name of every type the schema files of one plugin folder define.
func definedTypes(t *testing.T, pluginDir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(pluginDir, "graph", "*.graphqls"))
	if err != nil {
		t.Fatalf("globbing the schema of %s: %v", pluginDir, err)
	}
	var names []string
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		doc, err := parser.ParseSchema(&ast.Source{Name: path, Input: string(raw)})
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		for _, def := range doc.Definitions {
			names = append(names, def.Name)
		}
	}
	return names
}

// staleTypeExceptions returns the exceptions of one plugin folder that name no type its graph folder defines.
func staleTypeExceptions(t *testing.T, pluginDir string) []string {
	t.Helper()
	defined := definedTypes(t, pluginDir)
	return slices.DeleteFunc(typeExceptions(t, pluginDir), func(name string) bool {
		return slices.Contains(defined, name)
	})
}

// writePluginFiles writes every file of a temporary plugin folder, keyed by path, creating the folders above them.
func writePluginFiles(t *testing.T, files map[string]string) {
	t.Helper()
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("creating the folder of %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
	}
}

func TestTypeExceptionsAreReadFromThePluginTestdata(t *testing.T) {
	t.Parallel()

	pluginDir := filepath.Join(t.TempDir(), "plugins", "feed")
	writePluginFiles(t, map[string]string{
		filepath.Join(pluginDir, "testdata", "unprefixedtypes.txt"): "\nNewsItem\n\nNewsKind\n",
	})

	got := typeExceptions(t, pluginDir)

	if want := []string{"NewsItem", "NewsKind"}; !slices.Equal(got, want) {
		t.Errorf("typeExceptions() = %q, want %q", got, want)
	}
	if none := typeExceptions(t, t.TempDir()); len(none) != 0 {
		t.Errorf("typeExceptions() without a file = %q, want none", none)
	}
}

func TestATypeExceptionNamingNoDefinedTypeIsStale(t *testing.T) {
	t.Parallel()

	pluginDir := filepath.Join(t.TempDir(), "plugins", "feed")
	writePluginFiles(t, map[string]string{
		filepath.Join(pluginDir, "graph", "schema.graphqls"):        "type NewsItem { id: ID! }\n",
		filepath.Join(pluginDir, "testdata", "unprefixedtypes.txt"): "NewsItem\nNewsGone\n",
	})

	if stale := staleTypeExceptions(t, pluginDir); !slices.Equal(stale, []string{"NewsGone"}) {
		t.Errorf("staleTypeExceptions() = %q, want the exception naming no type", stale)
	}
}

func TestPluginTypesCarryTheirPluginPrefix(t *testing.T) {
	t.Parallel()

	dirs := pluginGraphDirs(t)
	if len(dirs) == 0 {
		t.Fatal("found no plugin graph dirs, the glob is broken")
	}
	for _, dir := range dirs {
		naming := typeNaming{pluginID: filepath.Base(filepath.Dir(dir)), exceptions: typeExceptions(t, filepath.Dir(dir))}
		files, err := filepath.Glob(filepath.Join(dir, "*.graphqls"))
		if err != nil {
			t.Fatalf("globbing %s: %v", dir, err)
		}
		for _, path := range files {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading %s: %v", path, err)
			}
			for _, offense := range misnamedTypes(t, naming, path, string(raw)) {
				t.Error(offense)
			}
		}
	}
}

func TestEveryTypeExceptionNamesATypeItsPluginDefines(t *testing.T) {
	t.Parallel()

	for _, dir := range pluginGraphDirs(t) {
		for _, name := range staleTypeExceptions(t, filepath.Dir(dir)) {
			t.Errorf("%s excepts %s, which its schema no longer defines, want the line dropped", filepath.Dir(dir), name)
		}
	}
}

func TestMisnamedTypeDetectionFlagsARogueType(t *testing.T) {
	t.Parallel()

	synthetic := `
type Rogue { id: ID! }
type FeedItem { id: ID! }
extend type Query { feedItems: [FeedItem!]! }
`

	offenses := misnamedTypes(t, typeNaming{pluginID: "feed"}, "plugins/feed/graph/schema.graphqls", synthetic)

	if len(offenses) != 1 {
		t.Fatalf("offenses = %v, want exactly the rogue type flagged", offenses)
	}
	if offense := offenses[0]; !strings.Contains(offense, "Rogue") ||
		!strings.Contains(offense, "plugins/feed/graph/schema.graphqls") {
		t.Errorf("offense = %q, want the type and its file named", offense)
	}
}

func TestExceptedTypesPassTheDetection(t *testing.T) {
	t.Parallel()

	synthetic := `type ImportJob { id: ID! }`
	naming := typeNaming{pluginID: "importer", exceptions: []string{"ImportJob"}}

	if offenses := misnamedTypes(t, naming, "synthetic", synthetic); len(offenses) != 0 {
		t.Errorf("offenses = %v, want the excepted type accepted", offenses)
	}
}
