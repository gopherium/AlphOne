// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// tenantGuardedTables lists the tables every named query must filter by tenant.
var tenantGuardedTables = []string{}

// namedQueries splits a sqlc source into its named query blocks.
func namedQueries(source string) map[string]string {
	blocks := map[string]string{}
	name := ""
	held := &strings.Builder{}
	for _, line := range strings.Split(source, "\n") {
		if opened, found := strings.CutPrefix(line, "-- name: "); found {
			if name != "" {
				blocks[name] = held.String()
			}
			name = strings.Fields(opened)[0]
			held = &strings.Builder{}
			continue
		}
		held.WriteString(line)
		held.WriteString("\n")
	}
	if name != "" {
		blocks[name] = held.String()
	}
	return blocks
}

// unguarded returns the guarded tables a query touches without naming tenant_id.
func unguarded(query string, guarded []string) []string {
	if strings.Contains(query, "tenant_id") {
		return nil
	}
	var touched []string
	for _, table := range guarded {
		pattern := regexp.MustCompile(`\b(core|plugin_[a-z]+)\.` + table + `\b`)
		if pattern.MatchString(query) {
			touched = append(touched, table)
		}
	}
	return touched
}

func TestTheGateNamesAQueryTouchingAGuardedTableWithoutItsTenant(t *testing.T) {
	t.Parallel()

	violating := "SELECT id, name FROM core.contacts ORDER BY name"

	touched := unguarded(violating, []string{"contacts"})

	if len(touched) != 1 || touched[0] != "contacts" {
		t.Errorf("unguarded() = %v, want the touched table named", touched)
	}
}

func TestTheGateAdmitsAQueryCarryingItsTenantPredicate(t *testing.T) {
	t.Parallel()

	guarded := "SELECT id, name FROM core.contacts WHERE tenant_id = $1"

	if touched := unguarded(guarded, []string{"contacts"}); len(touched) != 0 {
		t.Errorf("unguarded() = %v, want a filtered query admitted", touched)
	}
}

func TestTheGateAdmitsAQueryTouchingNoGuardedTable(t *testing.T) {
	t.Parallel()

	elsewhere := "SELECT id FROM core.tenants"

	if touched := unguarded(elsewhere, []string{"contacts"}); len(touched) != 0 {
		t.Errorf("unguarded() = %v, want an untouched table ignored", touched)
	}
}

func TestEveryNamedQueryFiltersTheGuardedTablesByTenant(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("queries.sql")
	if err != nil {
		t.Fatalf("reading queries.sql: %v", err)
	}
	queries := namedQueries(string(source))
	if len(queries) == 0 {
		t.Fatal("no named queries parsed, want the sqlc source walked")
	}

	for name, query := range queries {
		for _, table := range unguarded(query, tenantGuardedTables) {
			t.Errorf("%s touches %s without filtering by tenant_id", name, table)
		}
	}
}
