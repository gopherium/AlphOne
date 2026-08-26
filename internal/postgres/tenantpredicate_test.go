// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// tenantGuardedTables lists the tables every named query must filter by tenant.
var tenantGuardedTables = []string{
	"contacts",
	"contact_identities",
	"tasks",
	"api_tokens",
	"user_settings",
	"webhook_subscriptions",
	"webhook_deliveries",
}

// authenticatingQueries names the queries resolving a credential before any tenant is known.
var authenticatingQueries = map[string]bool{
	"GetAPITokenByHash": true,
	"TouchAPIToken":     true,
}

// workerQueries names the queries a background worker runs across every tenant.
var workerQueries = map[string]bool{
	"ClaimWebhookDeliveries": true,
	"SettleWebhookDelivery":  true,
}

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

// tenantParameter matches a tenant_id held against a value the caller supplies.
var tenantParameter = regexp.MustCompile(`tenant_id\s*=\s*(\$\d+|@\w+)`)

// conflictArbiter captures the columns an ON CONFLICT clause arbitrates on.
var conflictArbiter = regexp.MustCompile(`(?is)ON\s+CONFLICT\s*\(([^)]*)\)`)

// insertedColumns captures the columns an INSERT names for its table.
var insertedColumns = regexp.MustCompile(`(?is)INSERT\s+INTO\s+\w+\.\w+\s*\(([^)]*)\)`)

// tenantSafe reports whether a statement holds its rows to the caller's tenant.
func tenantSafe(statement string) bool {
	if arbiter := conflictArbiter.FindStringSubmatch(statement); arbiter != nil &&
		!strings.Contains(arbiter[1], "tenant_id") {
		return false
	}
	if tenantParameter.MatchString(statement) {
		return true
	}
	if columns := insertedColumns.FindStringSubmatch(statement); columns != nil {
		return strings.Contains(columns[1], "tenant_id")
	}
	return false
}

// unguarded returns the guarded tables a query touches without holding them to the caller's tenant.
func unguarded(query string, guarded []string) []string {
	if tenantSafe(query) {
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

func TestTheGateRefusesATenantIdThatHoldsNothingToTheCaller(t *testing.T) {
	t.Parallel()

	joining := `SELECT conv.id FROM plugin_whatsapp.conversations conv
		LEFT JOIN LATERAL (SELECT 1 FROM plugin_whatsapp.messages m
			WHERE m.conversation_id = conv.id AND m.tenant_id = conv.tenant_id) x ON TRUE
		WHERE conv.contact_id = ANY($1)`

	if tenantSafe(joining) {
		t.Error("tenantSafe() admitted a column to column tenant join, want the caller's tenant demanded")
	}
}

func TestTheGateRefusesAConflictArbiterSpanningTenants(t *testing.T) {
	t.Parallel()

	spanning := `INSERT INTO core.user_settings (user_id, key, value, tenant_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, key) DO UPDATE SET value = EXCLUDED.value`

	if tenantSafe(spanning) {
		t.Error("tenantSafe() admitted an arbiter spanning tenants, want tenant_id demanded in it")
	}
}

func TestTheGateAdmitsAConflictArbiterCarryingTheTenant(t *testing.T) {
	t.Parallel()

	held := `INSERT INTO core.contact_identities (id, contact_id, channel, identifier, tenant_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (tenant_id, channel, identifier) DO NOTHING`

	if !tenantSafe(held) {
		t.Error("tenantSafe() refused a tenant composite arbiter, want it admitted")
	}
}

func TestTheGateAdmitsAnInsertStampingItsTenant(t *testing.T) {
	t.Parallel()

	stamping := `INSERT INTO core.contacts (id, name, created_at, tenant_id) VALUES ($1, $2, $3, $4)`

	if !tenantSafe(stamping) {
		t.Error("tenantSafe() refused an insert stamping its tenant, want it admitted")
	}
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
		if authenticatingQueries[name] || workerQueries[name] {
			continue
		}
		for _, table := range unguarded(query, tenantGuardedTables) {
			t.Errorf("%s touches %s without filtering by tenant_id", name, table)
		}
	}
}

func TestEveryExemptedQueryStillExists(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("queries.sql")
	if err != nil {
		t.Fatalf("reading queries.sql: %v", err)
	}
	queries := namedQueries(string(source))

	for name := range authenticatingQueries {
		if _, held := queries[name]; !held {
			t.Errorf("%s is exempted but no longer exists, want the exemption dropped", name)
		}
	}
	for name := range workerQueries {
		if _, held := queries[name]; !held {
			t.Errorf("%s is exempted but no longer exists, want the exemption dropped", name)
		}
	}
}
