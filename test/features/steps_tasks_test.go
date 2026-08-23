// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/google/uuid"

	alphonemcp "github.com/gopherium/alphone/internal/mcp"
	"github.com/gopherium/alphone/internal/task"
)

// seedTask stores one named task for the owner and returns it.
func (w *world) seedTask(ctx context.Context, title string, due time.Time, status string, contactID uuid.UUID) error {
	created, err := task.New(task.Input{
		Title:      title,
		DueOn:      due,
		AssigneeID: w.ownerID,
		ContactID:  contactID,
	})
	if err != nil {
		return fmt.Errorf("building %q: %w", title, err)
	}
	created.Status = status
	if _, _, err := w.tasks.Create(ctx, created); err != nil {
		return fmt.Errorf("storing %q: %w", title, err)
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
