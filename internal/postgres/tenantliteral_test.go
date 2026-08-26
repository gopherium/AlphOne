// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gopherium/alphone/internal/tenant"
)

// migrationRoots lists every migration directory naming the default tenant.
var migrationRoots = []string{
	"migrations",
	filepath.Join("..", "..", "plugins", "fields", "migrations"),
	filepath.Join("..", "..", "plugins", "importer", "migrations"),
	filepath.Join("..", "..", "plugins", "whatsapp", "migrations"),
}

func TestEveryMigrationNamesTheDefaultTenantTheCodeHolds(t *testing.T) {
	t.Parallel()

	held := tenant.DefaultID.String()
	named := 0
	for _, root := range migrationRoots {
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
