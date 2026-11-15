// SPDX-License-Identifier: Elastic-2.0

// Package features_test runs the Gherkin specs of record against a real server.
package features_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/jackc/pgx/v5"

	"github.com/gopherium/alphone/internal/testdb"
)

// wipTag marks the scenarios no stage has implemented yet.
const wipTag = "@wip"

// endedScenarioFeature is a feature of one scenario that reads the database of its world.
const endedScenarioFeature = `Feature: A scenario ends with its world

  Scenario: The scenario reads its database
    When the scenario reads its database
`

// sessionsOnDatabase counts the sessions the server holds on one database.
const sessionsOnDatabase = "SELECT count(*) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()"

// sessionSettleBudget bounds how long a check waits for the sessions of a closed pool to leave the server.
const sessionSettleBudget = 5 * time.Second

// sessionSettlePause spaces two counts of a check waiting on the server.
const sessionSettlePause = 50 * time.Millisecond

// runFeature runs one feature file as its own suite, refusing undefined steps.
func runFeature(t *testing.T, path string, initialize func(*godog.ScenarioContext)) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	tags := "~" + wipTag
	if os.Getenv("ALPHONE_BDD_WIP") != "" {
		tags = ""
	}
	suite := godog.TestSuite{
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			initialize(sc)
			sc.After(endWorld)
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{path},
			Tags:     tags,
			Strict:   true,
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Error("the feature did not pass")
	}
}

// sessionsOn answers the sessions the test server holds on database, waiting a short while for them to leave.
func sessionsOn(t *testing.T, database string) int {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), testdb.Setting(t, "ALPHONE_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("connecting to the test server: %v", err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	deadline := time.Now().Add(sessionSettleBudget)
	for {
		var held int
		if err := conn.QueryRow(t.Context(), sessionsOnDatabase, database).Scan(&held); err != nil {
			t.Fatalf("counting the sessions on %s: %v", database, err)
		}
		if held == 0 || time.Now().After(deadline) {
			return held
		}
		time.Sleep(sessionSettlePause)
	}
}

func TestAnEndedScenarioHoldsNoSessionOnTheDatabaseOfItsWorld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ended-scenario.feature")
	if err := os.WriteFile(path, []byte(endedScenarioFeature), 0o600); err != nil {
		t.Fatalf("writing the feature: %v", err)
	}
	var database string
	runFeature(t, path, func(sc *godog.ScenarioContext) {
		sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
			return context.WithValue(ctx, worldKey{}, newImportWorld(t)), nil
		})
		sc.When(`^the scenario reads its database$`, func(ctx context.Context) error {
			w := worldFrom(ctx)
			database = w.pool.Config().ConnConfig.Database
			_, err := w.pool.Exec(ctx, "SELECT 1")
			return err
		})
	})

	if held := sessionsOn(t, database); held != 0 {
		t.Errorf("the database %s holds %d sessions after its scenario ended, want none", database, held)
	}
}

// initializeSession registers the MCP session steps.
func initializeSession(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) { registerSessionSteps(sc, t) }
}

// initializeWorkload registers the workload summary steps.
func initializeWorkload(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) { registerWorkloadSteps(sc, t) }
}

// initializeTasks registers the task listing steps.
func initializeTasks(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) { registerTaskSteps(sc, t) }
}

// initializeContacts registers the contact reading steps.
func initializeContacts(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) { registerContactSteps(sc, t) }
}

// initializeFieldsCatalog registers the field catalogue steps.
func initializeFieldsCatalog(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) { registerFieldsCatalogSteps(sc, t) }
}

// initializeFieldsValues registers the field value steps.
func initializeFieldsValues(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) { registerFieldsValuesSteps(sc, t) }
}

// initializeFieldsGraph registers the widened graph steps.
func initializeFieldsGraph(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) { registerFieldsGraphSteps(sc, t) }
}

// initializeFieldsTenants registers the steps of fields kept per tenant.
func initializeFieldsTenants(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) { registerFieldsTenantsSteps(sc, t) }
}

// initializeFieldsRepeater registers the repeater field steps.
func initializeFieldsRepeater(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) { registerFieldsRepeaterSteps(sc, t) }
}

// initializeTenants registers the tenant seam steps.
func initializeTenants(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) { registerTenantSteps(sc, t) }
}

// initializeLocale registers the locale resolution steps.
func initializeLocale(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) { registerLocaleSteps(sc, t) }
}

// initializeImportFields registers the import mapping steps.
func initializeImportFields(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) { registerImportFieldsSteps(sc, t) }
}

// initializeTokens registers the token scope steps.
func initializeTokens(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) { registerTokenSteps(sc, t) }
}

// initializeWebhooks registers the webhook steps.
func initializeWebhooks(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) { registerWebhookSteps(sc, t) }
}

// initializeWebhookManagement registers the steps managing webhooks as an admin or a member.
func initializeWebhookManagement(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) { registerWebhookManagementSteps(sc, t) }
}

// initializeCrossOrigin registers the cross-origin write steps.
func initializeCrossOrigin(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) { registerCrossOriginSteps(sc, t) }
}

// initializeRoles registers the role steps.
func initializeRoles(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) { registerRoleSteps(sc, t) }
}

func initializeMail(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) { registerMailSteps(sc, t) }
}

// initializeIdentityFeed registers the identity feed steps beside the tenant ones.
func initializeIdentityFeed(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) {
		registerTenantSteps(sc, t)
		bindIdentityFeedSteps(sc)
	}
}

// initializeContactPage registers the contact page steps.
func initializeContactPage(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) { registerContactPageSteps(sc, t) }
}

// initializeAdminSettings registers the admin settings steps.
func initializeAdminSettings(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) { registerAdminSettingsSteps(sc, t) }
}

// initializeLegalLinks registers the legal links steps.
func initializeLegalLinks(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) { registerLegalLinksSteps(sc, t) }
}

func TestMCPSession(t *testing.T) {
	runFeature(t, "features/mcp-session.feature", initializeSession(t))
}

func TestMCPWorkload(t *testing.T) {
	runFeature(t, "features/mcp-workload.feature", initializeWorkload(t))
}

func TestMCPTasks(t *testing.T) {
	runFeature(t, "features/mcp-tasks.feature", initializeTasks(t))
}

func TestMCPContacts(t *testing.T) {
	runFeature(t, "features/mcp-contacts.feature", initializeContacts(t))
}

func TestTaskTimes(t *testing.T) {
	runFeature(t, "features/task-times.feature", initializeTasks(t))
}

func TestFieldsCatalog(t *testing.T) {
	runFeature(t, "features/fields-catalog.feature", initializeFieldsCatalog(t))
}

func TestFieldsValues(t *testing.T) {
	runFeature(t, "features/fields-values.feature", initializeFieldsValues(t))
}

func TestFieldsGraph(t *testing.T) {
	runFeature(t, "features/fields-graph.feature", initializeFieldsGraph(t))
}

func TestFieldsTenants(t *testing.T) {
	runFeature(t, "features/fields-tenants.feature", initializeFieldsTenants(t))
}

func TestFieldsRepeater(t *testing.T) {
	runFeature(t, "features/fields-repeater.feature", initializeFieldsRepeater(t))
}

func TestTenants(t *testing.T) {
	runFeature(t, "features/tenants.feature", initializeTenants(t))
}

func TestLocaleResolution(t *testing.T) {
	runFeature(t, "features/locale.feature", initializeLocale(t))
}

func TestImportFields(t *testing.T) {
	runFeature(t, "features/import-fields.feature", initializeImportFields(t))
}

func TestImportRowReasons(t *testing.T) {
	runFeature(t, "features/import-row-reasons.feature", initializeImportFields(t))
}

func TestTokens(t *testing.T) {
	runFeature(t, "features/tokens.feature", initializeTokens(t))
}

func TestWebhooks(t *testing.T) {
	runFeature(t, "features/webhooks.feature", initializeWebhooks(t))
}

func TestWebhookEvents(t *testing.T) {
	runFeature(t, "features/webhook-events.feature", initializeWebhooks(t))
}

func TestWebhookManagement(t *testing.T) {
	runFeature(t, "features/webhook-management.feature", initializeWebhookManagement(t))
}

func TestCrossOriginWrites(t *testing.T) {
	runFeature(t, "features/cross-origin-writes.feature", initializeCrossOrigin(t))
}

func TestRoles(t *testing.T) {
	runFeature(t, "features/roles.feature", initializeRoles(t))
}

func TestMail(t *testing.T) {
	runFeature(t, "features/mail.feature", initializeMail(t))
}

func TestContactPage(t *testing.T) {
	runFeature(t, "features/contact-page.feature", initializeContactPage(t))
}

func TestAdminSettings(t *testing.T) {
	runFeature(t, "features/admin-settings.feature", initializeAdminSettings(t))
}

func TestLegalLinks(t *testing.T) {
	runFeature(t, "features/legal-links.feature", initializeLegalLinks(t))
}

func TestIdentityFeed(t *testing.T) {
	runFeature(t, "features/identity-feed.feature", initializeIdentityFeed(t))
}
