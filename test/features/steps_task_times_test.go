// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/cucumber/godog"

	"github.com/gopherium/alphone/internal/task"
)

// createTaskDocument creates a task through the graph.
const createTaskDocument = `mutation($input: CreateTaskInput!) { createTask(input: $input) { task { id } } }`

// updateTaskDocument updates a task through the graph.
const updateTaskDocument = `mutation($id: UUID!, $input: UpdateTaskInput!) {
	updateTask(id: $id, input: $input) { id }
}`

// heldTasksQuery reads every task the owner holds through the graph.
const heldTasksQuery = `query { tasks(dueBefore: "9999-12-31", status: "all") {
	edges { node { id title dueOn dueTime } }
} }`

// noTime is how a step writes the time of a task that holds none.
const noTime = "no time"

// timedTask is one task by its title, day and time of day.
type timedTask struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	DueOn   string  `json:"dueOn"`
	DueTime *string `json:"dueTime"`
}

// clock returns the time of day the task holds, or [noTime].
func (t timedTask) clock() string {
	if t.DueTime == nil {
		return noTime
	}
	return *t.DueTime
}

// heldTasksAnswer is the envelope the task time steps read the owner's tasks from.
type heldTasksAnswer struct {
	Data struct {
		Tasks struct {
			Edges []struct {
				Node timedTask `json:"node"`
			} `json:"edges"`
		} `json:"tasks"`
	} `json:"data"`
}

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

// createVariables returns the variables of a create of the task want describes.
func createVariables(want timedTask) map[string]any {
	input := map[string]any{"title": want.Title, "dueOn": want.DueOn}
	if want.DueTime != nil {
		input["dueTime"] = *want.DueTime
	}
	return map[string]any{"input": input}
}

// acceptedWrite posts a graph write as the owner and reports any error the graph answers with.
func (w *world) acceptedWrite(ctx context.Context, document string, variables map[string]any) error {
	answer, err := w.operation(ctx, document, variables)
	if err != nil {
		return err
	}
	if len(answer.Errors) > 0 {
		return fmt.Errorf("the graph refused the write, answered %s", w.answered)
	}
	return nil
}

// heldTaskTitled reads the owner's task titled title through the graph.
func (w *world) heldTaskTitled(ctx context.Context, title string) (timedTask, error) {
	if _, err := w.operation(ctx, heldTasksQuery, map[string]any{}); err != nil {
		return timedTask{}, err
	}
	var answer heldTasksAnswer
	if err := json.Unmarshal(w.answered, &answer); err != nil {
		return timedTask{}, fmt.Errorf("decoding %s: %w", w.answered, err)
	}
	for _, edge := range answer.Data.Tasks.Edges {
		if edge.Node.Title == title {
			return edge.Node, nil
		}
	}
	return timedTask{}, fmt.Errorf("the owner holds no task titled %q, answered %s", title, w.answered)
}

// updateHeldTask applies the update input to the owner's task titled title through the graph.
func (w *world) updateHeldTask(ctx context.Context, title string, input map[string]any) error {
	held, err := w.heldTaskTitled(ctx, title)
	if err != nil {
		return err
	}
	return w.acceptedWrite(ctx, updateTaskDocument, map[string]any{"id": held.ID, "input": input})
}

// wantTaskDue checks through the graph that the owner's task titled as want is due on its day at its time.
func (w *world) wantTaskDue(ctx context.Context, want timedTask) error {
	held, err := w.heldTaskTitled(ctx, want.Title)
	if err != nil {
		return err
	}
	if held.DueOn != want.DueOn || held.clock() != want.clock() {
		return fmt.Errorf("task %q is due on %s at %s, want %s at %s",
			want.Title, held.DueOn, held.clock(), want.DueOn, want.clock())
	}
	return nil
}

// answeredTaskFields returns the keys the last task list or contact answer gives the task titled title.
func (w *world) answeredTaskFields(title string) (map[string]any, error) {
	raw, err := w.structuredAnswer()
	if err != nil {
		return nil, err
	}
	var answer struct {
		Tasks     []map[string]any `json:"tasks"`
		OpenTasks []map[string]any `json:"open_tasks"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil {
		return nil, fmt.Errorf("decoding %s: %w", raw, err)
	}
	for _, fields := range slices.Concat(answer.Tasks, answer.OpenTasks) {
		if fields["title"] == title {
			return fields, nil
		}
	}
	return nil, fmt.Errorf("the answer holds no task titled %q, answered %s", title, raw)
}

// registerTimedTaskSteps binds the steps that seed, write and read tasks due at a time of day.
func registerTimedTaskSteps(sc *godog.ScenarioContext) {
	registerTimedSeedSteps(sc)
	registerTimedWriteSteps(sc)
	registerTimedReadSteps(sc)
	registerTimedAnswerSteps(sc)
}

// registerTimedAnswerSteps binds the steps that read the time of day a tool answer gives a task.
func registerTimedAnswerSteps(sc *godog.ScenarioContext) {
	sc.Then(`^the answer gives "([^"]*)" the time "(\d{2}:\d{2})"$`,
		func(ctx context.Context, title, clock string) error {
			fields, err := worldFrom(ctx).answeredTaskFields(title)
			if err != nil {
				return err
			}
			if fields["due_time"] != clock {
				return fmt.Errorf("task %q answers due_time %v, want %s", title, fields["due_time"], clock)
			}
			return nil
		})

	sc.Then(`^the answer gives "([^"]*)" no time$`, func(ctx context.Context, title string) error {
		fields, err := worldFrom(ctx).answeredTaskFields(title)
		if err != nil {
			return err
		}
		if dueTime, held := fields["due_time"]; held {
			return fmt.Errorf("task %q answers due_time %v, want no due_time key", title, dueTime)
		}
		return nil
	})
}

// registerTimedSeedSteps binds the steps that seed tasks due at a time of day.
func registerTimedSeedSteps(sc *godog.ScenarioContext) {
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

// registerTimedWriteSteps binds the steps that create and update tasks with a time of day through the graph.
func registerTimedWriteSteps(sc *godog.ScenarioContext) {
	sc.When(`^the owner creates the task "([^"]*)" due on (\d{4}-\d{2}-\d{2})$`,
		func(ctx context.Context, title, day string) error {
			created := createVariables(timedTask{Title: title, DueOn: day})
			_, err := worldFrom(ctx).operation(ctx, createTaskDocument, created)
			return err
		})

	createTimed := func(ctx context.Context, title, day, clock string) error {
		created := createVariables(timedTask{Title: title, DueOn: day, DueTime: &clock})
		_, err := worldFrom(ctx).operation(ctx, createTaskDocument, created)
		return err
	}
	sc.When(`^the owner creates the task "([^"]*)" due on (\d{4}-\d{2}-\d{2}) at (\d{2}:\d{2})$`, createTimed)
	sc.When(`^the owner creates the task "([^"]*)" due on (\d{4}-\d{2}-\d{2}) at "([^"]*)"$`, createTimed)

	sc.Given(`^the owner created the task "([^"]*)" due on (\d{4}-\d{2}-\d{2}) at (\d{2}:\d{2})$`,
		func(ctx context.Context, title, day, clock string) error {
			created := createVariables(timedTask{Title: title, DueOn: day, DueTime: &clock})
			return worldFrom(ctx).acceptedWrite(ctx, createTaskDocument, created)
		})

	sc.When(`^the owner moves the task "([^"]*)" to (\d{4}-\d{2}-\d{2})$`,
		func(ctx context.Context, title, day string) error {
			return worldFrom(ctx).updateHeldTask(ctx, title, map[string]any{"dueOn": day})
		})

	sc.When(`^the owner sets the time of the task "([^"]*)" to (\d{2}:\d{2})$`,
		func(ctx context.Context, title, clock string) error {
			return worldFrom(ctx).updateHeldTask(ctx, title, map[string]any{"dueTime": clock})
		})

	sc.When(`^the owner clears the time of the task "([^"]*)"$`, func(ctx context.Context, title string) error {
		return worldFrom(ctx).updateHeldTask(ctx, title, map[string]any{"dueTime": nil})
	})
}

// registerTimedReadSteps binds the steps that read the day and time of a task and a refused time through the graph.
func registerTimedReadSteps(sc *godog.ScenarioContext) {
	sc.Then(`^the task "([^"]*)" is due on (\d{4}-\d{2}-\d{2}) at (\d{2}:\d{2})$`,
		func(ctx context.Context, title, day, clock string) error {
			return worldFrom(ctx).wantTaskDue(ctx, timedTask{Title: title, DueOn: day, DueTime: &clock})
		})

	sc.Then(`^the task "([^"]*)" is due on (\d{4}-\d{2}-\d{2}) with no time$`,
		func(ctx context.Context, title, day string) error {
			return worldFrom(ctx).wantTaskDue(ctx, timedTask{Title: title, DueOn: day})
		})

	sc.Then(`^the graph refuses the value as malformed$`, func(ctx context.Context) error {
		return worldFrom(ctx).refusedForReason("VALIDATION", "value_malformed")
	})
}
