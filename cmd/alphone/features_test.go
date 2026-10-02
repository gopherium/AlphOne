// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"os"
	"testing"

	"github.com/cucumber/godog"
)

// operatorCommandsFeature is the feature the command line answers to.
const operatorCommandsFeature = "../../test/features/features/operator-commands.feature"

// wipTag marks the scenarios whose steps are not bound yet.
const wipTag = "@wip"

// initializeOperatorCommands binds the operator command steps to a scenario.
func initializeOperatorCommands(*godog.ScenarioContext) {}

func TestOperatorCommands(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	tags := "~" + wipTag
	if os.Getenv("ALPHONE_BDD_WIP") != "" {
		tags = ""
	}
	suite := godog.TestSuite{
		ScenarioInitializer: initializeOperatorCommands,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{operatorCommandsFeature},
			Tags:     tags,
			Strict:   true,
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Error("the feature did not pass")
	}
}
