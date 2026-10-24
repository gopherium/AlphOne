// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gopherium/alphone/internal/tenant"
)

// keepsMigrations reports whether one plugin folder holds a migrations folder.
func keepsMigrations(t *testing.T, dir string) bool {
	t.Helper()
	path := filepath.Join(dir, "migrations")
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false
	}
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return info.IsDir()
}

// pluginMigrationDirs returns the migrations folder of every plugin under each plugin root of a tree that keeps one.
func pluginMigrationDirs(t *testing.T, tree string) []string {
	t.Helper()
	var dirs []string
	for _, manifest := range pluginManifests(t, tree, "plugins", "enterprise") {
		if keepsMigrations(t, manifest.dir) {
			dirs = append(dirs, filepath.Join(manifest.dir, "migrations"))
		}
	}
	return dirs
}

func TestPluginMigrationDirsFollowTheManifests(t *testing.T) {
	t.Parallel()

	tree := t.TempDir()
	writeTreeFile(t, filepath.Join(tree, "plugins", "demo", "plugin.json"), `{"id": "demo"}`)
	writeTreeFile(t, filepath.Join(tree, "plugins", "demo", "migrations", "00001_create.sql"), "SELECT 1;\n")
	writeTreeFile(t, filepath.Join(tree, "plugins", "quiet", "plugin.json"), `{"id": "quiet"}`)
	writeTreeFile(t, filepath.Join(tree, "plugins", "stray", "migrations", "00001_create.sql"), "SELECT 1;\n")
	writeTreeFile(t, filepath.Join(tree, "enterprise", "extra", "plugin.json"), `{"id": "extra"}`)
	writeTreeFile(t, filepath.Join(tree, "enterprise", "extra", "migrations", "00001_create.sql"), "SELECT 1;\n")

	got := pluginMigrationDirs(t, tree)

	want := []string{
		filepath.Join(tree, "plugins", "demo", "migrations"),
		filepath.Join(tree, "enterprise", "extra", "migrations"),
	}
	if !slices.Equal(got, want) {
		t.Errorf("pluginMigrationDirs() = %q, want %q", got, want)
	}
}

func TestEveryMigrationNamesTheDefaultTenantTheCodeHolds(t *testing.T) {
	t.Parallel()

	held := tenant.DefaultID.String()
	named := 0
	for _, root := range append([]string{"migrations"}, pluginMigrationDirs(t, repositoryRoot)...) {
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatalf("reading %s: %v", root, err)
		}
		for _, entry := range entries {
			source, err := os.ReadFile(filepath.Join(root, entry.Name()))
			if err != nil {
				t.Fatalf("reading %s: %v", entry.Name(), err)
			}
			for _, line := range strings.Split(string(source), "\n") {
				if !strings.Contains(line, "0000-7000-8000-0000") {
					continue
				}
				if !strings.Contains(line, held) {
					t.Errorf("%s names a tenant the code does not hold: %s", entry.Name(), line)
					continue
				}
				named++
			}
		}
	}
	if named == 0 {
		t.Error("no migration named the default tenant, want the seed and every column default")
	}
}
