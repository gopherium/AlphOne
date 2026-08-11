// SPDX-License-Identifier: Elastic-2.0

// Package features_test runs the Gherkin specs of record against a real server.
package features_test

import (
	"os"
	"testing"

	"github.com/cucumber/godog"
)

// wipTag marks the scenarios no stage has implemented yet.
const wipTag = "@wip"

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
		ScenarioInitializer: initialize,
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

// initializeSession registers the MCP session steps.
func initializeSession(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) { registerSessionSteps(sc, t) }
}

// initializeWorkload registers the workload summary steps.
func initializeWorkload(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) { registerWorkloadSteps(sc, t) }
}

// initializeTasks registers the task listing steps.
func initializeTasks(*godog.ScenarioContext) {}

// initializeContacts registers the contact reading steps.
func initializeContacts(*godog.ScenarioContext) {}

func TestMCPSession(t *testing.T) {
	runFeature(t, "features/mcp-session.feature", initializeSession(t))
}

func TestMCPWorkload(t *testing.T) {
	runFeature(t, "features/mcp-workload.feature", initializeWorkload(t))
}

func TestMCPTasks(t *testing.T) {
	runFeature(t, "features/mcp-tasks.feature", initializeTasks)
}

func TestMCPContacts(t *testing.T) {
	runFeature(t, "features/mcp-contacts.feature", initializeContacts)
}
