// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"context"
	"fmt"
	"time"

	"github.com/cucumber/godog"

	"github.com/gopherium/alphone/internal/task"
)

// seedTimed stores one task for the owner from the input, due at the HH:MM time clock names, in the given status.
func (w *world) seedTimed(ctx context.Context, input task.Input, clock, status string) error {
	dueTime, err := task.ParseClock(clock)
	if err != nil {
		return fmt.Errorf("parsing the time %q: %w", clock, err)
	}
	input.DueTime = dueTime
	return w.seedInput(ctx, input, status)
}

// seedDated stores one open task for the owner from the input, due on the YYYY-MM-DD day at the HH:MM time.
func (w *world) seedDated(ctx context.Context, input task.Input, day, clock string) error {
	due, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return fmt.Errorf("parsing %q: %w", day, err)
	}
	input.DueOn = due
	return w.seedTimed(ctx, input, clock, task.StatusOpen)
}

// registerTimedTaskSteps binds the steps that seed tasks due at a time of day.
func registerTimedTaskSteps(sc *godog.ScenarioContext) {
	sc.Given(`^an open task "([^"]*)" due today at (\d{2}:\d{2})$`,
		func(ctx context.Context, title, clock string) error {
			return worldFrom(ctx).seedTimed(ctx, task.Input{Title: title, DueOn: today()}, clock, task.StatusOpen)
		})

	sc.Given(`^an open task "([^"]*)" due today at (\d{2}:\d{2}) with priority (\d)$`,
		func(ctx context.Context, title, clock string, priority int) error {
			raised := task.Input{Title: title, DueOn: today(), Priority: priority}
			return worldFrom(ctx).seedTimed(ctx, raised, clock, task.StatusOpen)
		})

	sc.Given(`^an open task "([^"]*)" due today at (\d{2}:\d{2}) linked to that contact$`,
		func(ctx context.Context, title, clock string) error {
			w := worldFrom(ctx)
			linked := task.Input{Title: title, DueOn: today(), ContactID: w.lastContact}
			return w.seedTimed(ctx, linked, clock, task.StatusOpen)
		})

	sc.Given(`^an open task "([^"]*)" due on (\d{4}-\d{2}-\d{2}) at (\d{2}:\d{2})$`,
		func(ctx context.Context, title, day, clock string) error {
			return worldFrom(ctx).seedDated(ctx, task.Input{Title: title}, day, clock)
		})

	sc.Given(`^an open task "([^"]*)" due on (\d{4}-\d{2}-\d{2}) at (\d{2}:\d{2}) with priority (\d)$`,
		func(ctx context.Context, title, day, clock string, priority int) error {
			return worldFrom(ctx).seedDated(ctx, task.Input{Title: title, Priority: priority}, day, clock)
		})

	sc.Given(`^a done task "([^"]*)" due today at (\d{2}:\d{2})$`,
		func(ctx context.Context, title, clock string) error {
			return worldFrom(ctx).seedTimed(ctx, task.Input{Title: title, DueOn: today()}, clock, task.StatusDone)
		})
}
