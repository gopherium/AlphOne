// SPDX-License-Identifier: AGPL-3.0-or-later

package fields

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// guardedTables lists the plugin tables every statement must filter by tenant.
var guardedTables = []string{"definitions", "contact_values"}

// statements matches every SQL literal a Go source holds.
var statements = regexp.MustCompile("(?s)`([^`]*plugin_fields\\.[^`]*)`")

// unguarded returns the guarded tables a statement touches without naming tenant_id.
func unguarded(statement string) []string {
	if strings.Contains(statement, "tenant_id") {
		return nil
	}
	var touched []string
	for _, table := range guardedTables {
		if regexp.MustCompile(`\bplugin_fields\.` + table + `\b`).MatchString(statement) {
			touched = append(touched, table)
		}
	}
	return touched
}

func TestTheGateNamesAStatementMissingItsTenant(t *testing.T) {
	t.Parallel()

	touched := unguarded("SELECT id FROM plugin_fields.definitions")

	if len(touched) != 1 || touched[0] != "definitions" {
		t.Errorf("unguarded() = %v, want the touched table named", touched)
	}
}

func TestTheGateAdmitsAStatementCarryingItsTenant(t *testing.T) {
	t.Parallel()

	held := "SELECT id FROM plugin_fields.definitions WHERE tenant_id = $1"

	if touched := unguarded(held); len(touched) != 0 {
		t.Errorf("unguarded() = %v, want a filtered statement admitted", touched)
	}
}

func TestEveryStatementFiltersItsTableByTenant(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatalf("reading store.go: %v", err)
	}
	held := statements.FindAllStringSubmatch(string(source), -1)
	if len(held) == 0 {
		t.Fatal("no statements parsed, want the store walked")
	}

	for _, match := range held {
		for _, table := range unguarded(match[1]) {
			t.Errorf("a statement touches %s without filtering by tenant_id: %s", table, match[1])
		}
	}
}
