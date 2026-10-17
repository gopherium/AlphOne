// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/google/uuid"

	alphonemcp "github.com/gopherium/alphone/internal/mcp"
	"github.com/gopherium/alphone/internal/task"
)

// seedTask stores one named task for the owner at normal priority.
func (w *world) seedTask(ctx context.Context, title string, due time.Time, status string, contactID uuid.UUID) error {
	return w.seedInput(ctx, task.Input{Title: title, DueOn: due, ContactID: contactID}, status)
}

// seedInput stores one task for the owner from the given input, in the given status.
func (w *world) seedInput(ctx context.Context, input task.Input, status string) error {
	input.AssigneeID = w.ownerID
	created, err := task.New(input)
	if err != nil {
		return fmt.Errorf("building %q: %w", input.Title, err)
	}
	created.Status = status
	if _, _, err := w.tasks.Create(ctx, created); err != nil {
		return fmt.Errorf("storing %q: %w", input.Title, err)
	}
	return nil
}

// tasksAnswer decodes the task list the last call answered with.
func (w *world) tasksAnswer() (alphonemcp.TasksOutput, error) {
	var out alphonemcp.TasksOutput
	if w.called == nil {
		return out, fmt.Errorf("no tool was called")
	}
	if w.called.IsError {
		return out, fmt.Errorf("the tool failed: %s", contentText(w.called))
	}
	raw, err := json.Marshal(w.called.StructuredContent)
	if err != nil {
		return out, fmt.Errorf("encoding the structured answer: %w", err)
	}
	return out, json.Unmarshal(raw, &out)
}

// findTask returns the listed task carrying the given title.
func findTask(out alphonemcp.TasksOutput, title string) (alphonemcp.TaskItem, bool) {
	for _, item := range out.Tasks {
		if item.Title == title {
			return item, true
		}
	}
	return alphonemcp.TaskItem{}, false
}

// registerTaskSteps binds the task listing steps.
func registerTaskSteps(sc *godog.ScenarioContext, t *testing.T) {
	registerWorkloadSteps(sc, t)
	registerTimedTaskSteps(sc)
	registerTaskSeedSteps(sc)
	registerTaskCallSteps(sc)
	registerTaskAnswerSteps(sc)
	registerTaskDetailSteps(sc)
}

// registerTaskSeedSteps binds the steps that seed a contact and tasks with no time of day.
func registerTaskSeedSteps(sc *godog.ScenarioContext) {
	sc.Given(`^a contact "([^"]*)"$`, func(ctx context.Context, name string) error {
		_, err := worldFrom(ctx).seedContact(ctx, name)
		return err
	})

	sc.Given(`^an open task "([^"]*)" due today linked to that contact$`,
		func(ctx context.Context, title string) error {
			w := worldFrom(ctx)
			return w.seedTask(ctx, title, today(), task.StatusOpen, w.lastContact)
		})

	sc.Given(`^an open task "([^"]*)" due today$`, func(ctx context.Context, title string) error {
		return worldFrom(ctx).seedTask(ctx, title, today(), task.StatusOpen, uuid.Nil)
	})

	sc.Given(`^an open task "([^"]*)" due yesterday$`, func(ctx context.Context, title string) error {
		return worldFrom(ctx).seedTask(ctx, title, today().AddDate(0, 0, -1), task.StatusOpen, uuid.Nil)
	})

	sc.Given(`^an open task "([^"]*)" due on (\d{4}-\d{2}-\d{2})$`,
		func(ctx context.Context, title, day string) error {
			due, err := time.Parse(time.DateOnly, day)
			if err != nil {
				return fmt.Errorf("parsing %q: %w", day, err)
			}
			return worldFrom(ctx).seedTask(ctx, title, due, task.StatusOpen, uuid.Nil)
		})

	sc.Given(`^a done task "([^"]*)" due today$`, func(ctx context.Context, title string) error {
		return worldFrom(ctx).seedTask(ctx, title, today(), task.StatusDone, uuid.Nil)
	})

	sc.Given(`^an open task "([^"]*)" due today with priority (\d)$`,
		func(ctx context.Context, title string, priority int) error {
			raised := task.Input{Title: title, DueOn: today(), Priority: priority}
			return worldFrom(ctx).seedInput(ctx, raised, task.StatusOpen)
		})

	sc.Given(`^an open task "([^"]*)" due yesterday with priority (\d)$`,
		func(ctx context.Context, title string, priority int) error {
			raised := task.Input{Title: title, DueOn: today().AddDate(0, 0, -1), Priority: priority}
			return worldFrom(ctx).seedInput(ctx, raised, task.StatusOpen)
		})

	sc.Given(`^a done task "([^"]*)" due today with priority (\d)$`,
		func(ctx context.Context, title string, priority int) error {
			raised := task.Input{Title: title, DueOn: today(), Priority: priority}
			return worldFrom(ctx).seedInput(ctx, raised, task.StatusDone)
		})

	sc.Given(`^an open task "([^"]*)" due tomorrow linked to that contact with priority (\d)$`,
		func(ctx context.Context, title string, priority int) error {
			w := worldFrom(ctx)
			raised := task.Input{Title: title, DueOn: today().AddDate(0, 0, 1), Priority: priority, ContactID: w.lastContact}
			return w.seedInput(ctx, raised, task.StatusOpen)
		})
}

// registerTaskCallSteps binds the steps that call list_my_tasks.
func registerTaskCallSteps(sc *godog.ScenarioContext) {
	sc.When(`^the agent calls list_my_tasks$`, func(ctx context.Context) error {
		return worldFrom(ctx).callTool(ctx, "list_my_tasks", nil)
	})

	sc.When(`^the agent calls list_my_tasks for (\d{4}-\d{2}-\d{2})$`,
		func(ctx context.Context, day string) error {
			return worldFrom(ctx).callTool(ctx, "list_my_tasks", map[string]any{"date": day})
		})

	sc.When(`^the agent calls list_my_tasks for work due before today$`, func(ctx context.Context) error {
		return worldFrom(ctx).callTool(ctx, "list_my_tasks",
			map[string]any{"due_before": today().Format(time.DateOnly)})
	})

	sc.When(`^the agent calls list_my_tasks with status "([^"]*)"$`,
		func(ctx context.Context, status string) error {
			return worldFrom(ctx).callTool(ctx, "list_my_tasks", map[string]any{"status": status})
		})

	sc.When(`^the agent calls list_my_tasks with both a date and due_before$`, func(ctx context.Context) error {
		day := today().Format(time.DateOnly)
		return worldFrom(ctx).callTool(ctx, "list_my_tasks",
			map[string]any{"date": day, "due_before": day})
	})
}

// registerTaskAnswerSteps binds the steps that read which tasks the answer lists and in what order.
func registerTaskAnswerSteps(sc *godog.ScenarioContext) {
	sc.Then(`^the answer lists (\d+) tasks?$`, func(ctx context.Context, count int) error {
		out, err := worldFrom(ctx).tasksAnswer()
		if err != nil {
			return err
		}
		if len(out.Tasks) != count {
			return fmt.Errorf("listed %d tasks, want %d", len(out.Tasks), count)
		}
		return nil
	})

	sc.Then(`^the answer lists the tasks in order:$`, func(ctx context.Context, table *godog.Table) error {
		out, err := worldFrom(ctx).tasksAnswer()
		if err != nil {
			return err
		}
		titles := make([]string, 0, len(out.Tasks))
		for _, item := range out.Tasks {
			titles = append(titles, item.Title)
		}
		if want := tableColumn(table); !slices.Equal(titles, want) {
			return fmt.Errorf("tasks = %v, want %v", titles, want)
		}
		return nil
	})

	sc.Then(`^the task "([^"]*)" is listed$`, func(ctx context.Context, title string) error {
		out, err := worldFrom(ctx).tasksAnswer()
		if err != nil {
			return err
		}
		if _, found := findTask(out, title); !found {
			return fmt.Errorf("the answer holds no task titled %q", title)
		}
		return nil
	})
}

// registerTaskDetailSteps binds the steps that read the contact of a listed task and a refused call.
func registerTaskDetailSteps(sc *godog.ScenarioContext) {
	sc.Then(`^the task "([^"]*)" names its contact "([^"]*)"$`,
		func(ctx context.Context, title, name string) error {
			out, err := worldFrom(ctx).tasksAnswer()
			if err != nil {
				return err
			}
			item, found := findTask(out, title)
			if !found {
				return fmt.Errorf("the answer holds no task titled %q", title)
			}
			if item.ContactName != name {
				return fmt.Errorf("contact name = %q, want %q", item.ContactName, name)
			}
			if item.ContactID == "" {
				return fmt.Errorf("the task carries no contact id beside the name")
			}
			return nil
		})

	sc.Then(`^the tool fails with a validation error$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		if w.called == nil {
			return fmt.Errorf("no tool was called")
		}
		if !w.called.IsError {
			return fmt.Errorf("the tool succeeded, want it refused")
		}
		if !strings.Contains(contentText(w.called), "VALIDATION") {
			return fmt.Errorf("failure = %q, want it refused as invalid", contentText(w.called))
		}
		return nil
	})
}
