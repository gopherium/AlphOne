// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

// registerTokenConnectorSteps binds the steps an MCP connector drives with a scoped token.
func registerTokenConnectorSteps(sc *godog.ScenarioContext) {
	sc.Given(`^an MCP session connected with that token$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		if err := w.connect(ctx, w.scopedSecret); err != nil {
			return err
		}
		if w.session == nil {
			return fmt.Errorf("the agent never connected: %v", w.connErr)
		}
		return nil
	})

	sc.When(`^the agent calls the tool listing today's tasks$`, func(ctx context.Context) error {
		return worldFrom(ctx).callTool(ctx, "list_my_tasks", map[string]any{})
	})

	sc.Then(`^the tool answers the error naming "([^"]*)"$`, func(ctx context.Context, scope string) error {
		w := worldFrom(ctx)
		if w.called == nil {
			return fmt.Errorf("the agent called no tool")
		}
		if !w.called.IsError {
			return fmt.Errorf("the tool answered no error: %s", contentText(w.called))
		}
		if answered := contentText(w.called); !strings.Contains(answered, scope) {
			return fmt.Errorf("the tool answered %q, want it to name %q", answered, scope)
		}
		return nil
	})
}
