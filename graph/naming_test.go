// SPDX-License-Identifier: Elastic-2.0

package graph_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

// pluginTypeAllowlist names the deliberate exceptions to the plugin prefix rule.
var pluginTypeAllowlist = map[string][]string{
	"importer": {
		"ImportJob", "ImportRow", "ImportField", "ImportContact",
		"ImportAssignment", "ImportAssignmentInput", "ImportCommitPayload",
	},
	"fields": {"FieldDefinition", "FieldKind"},
}

// misnamedTypes reports the types source defines outside the plugin's prefix and allowlist.
func misnamedTypes(t *testing.T, pluginID, name, source string) []string {
	t.Helper()
	doc, err := parser.ParseSchema(&ast.Source{Name: name, Input: source})
	if err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	allowed := map[string]bool{}
	for _, typeName := range pluginTypeAllowlist[pluginID] {
		allowed[typeName] = true
	}
	var offenses []string
	for _, def := range doc.Definitions {
		if allowed[def.Name] || hasPluginPrefix(def.Name, pluginID) {
			continue
		}
		offenses = append(offenses, fmt.Sprintf(
			"%s defines %s, want the %s prefix or an allowlist entry", name, def.Name, pluginID,
		))
	}
	return offenses
}

// hasPluginPrefix reports whether the type name starts with the plugin id ignoring case.
func hasPluginPrefix(typeName, pluginID string) bool {
	return len(typeName) >= len(pluginID) && strings.EqualFold(typeName[:len(pluginID)], pluginID)
}

func TestPluginTypesCarryTheirPluginPrefix(t *testing.T) {
	t.Parallel()

	dirs, err := filepath.Glob("../plugins/*/graph")
	if err != nil {
		t.Fatalf("globbing plugin schemas: %v", err)
	}
	if len(dirs) == 0 {
		t.Fatal("found no plugin graph dirs, the glob is broken")
	}
	for _, dir := range dirs {
		pluginID := filepath.Base(filepath.Dir(dir))
		files, err := filepath.Glob(filepath.Join(dir, "*.graphqls"))
		if err != nil {
			t.Fatalf("globbing %s: %v", dir, err)
		}
		for _, path := range files {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading %s: %v", path, err)
			}
			for _, offense := range misnamedTypes(t, pluginID, path, string(raw)) {
				t.Error(offense)
			}
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

	offenses := misnamedTypes(t, "feed", "plugins/feed/graph/schema.graphqls", synthetic)

	if len(offenses) != 1 {
		t.Fatalf("offenses = %v, want exactly the rogue type flagged", offenses)
	}
	if offense := offenses[0]; !strings.Contains(offense, "Rogue") ||
		!strings.Contains(offense, "plugins/feed/graph/schema.graphqls") {
		t.Errorf("offense = %q, want the type and its file named", offense)
	}
}

func TestAllowlistedTypesPassTheDetection(t *testing.T) {
	t.Parallel()

	synthetic := `type ImportJob { id: ID! }`

	if offenses := misnamedTypes(t, "importer", "synthetic", synthetic); len(offenses) != 0 {
		t.Errorf("offenses = %v, want the allowlisted type accepted", offenses)
	}
}
