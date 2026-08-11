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
	"github.com/modelcontextprotocol/go-sdk/mcp"

	alphonemcp "github.com/gopherium/alphone/internal/mcp"
	"github.com/gopherium/alphone/internal/task"
)

// today returns the calendar day the scenarios count against.
func today() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

// seedTasks stores count tasks for the assignee, due on the given day.
func (w *world) seedTasks(ctx context.Context, assignee uuid.UUID, count int, due time.Time, status string) error {
	for i := range count {
		created, err := task.New(task.Input{
			Title:      fmt.Sprintf("Task %d", i+1),
			DueOn:      due,
			AssigneeID: assignee,
		})
		if err != nil {
			return fmt.Errorf("building the task: %w", err)
		}
		created.Status = status
		if _, _, err := w.tasks.Create(ctx, created); err != nil {
			return fmt.Errorf("storing the task: %w", err)
		}
	}
	return nil
}

// callTool calls one tool and remembers the result for the assertions.
func (w *world) callTool(ctx context.Context, name string, arguments map[string]any) error {
	if w.session == nil {
		return fmt.Errorf("the agent never connected: %v", w.connErr)
	}
	result, err := w.session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return fmt.Errorf("calling %s: %w", name, err)
	}
	w.called = result
	return nil
}

// contentText joins every text block of a tool result.
func contentText(result *mcp.CallToolResult) string {
	var parts []string
	for _, block := range result.Content {
		if text, ok := block.(*mcp.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// workloadAnswer decodes the structured workload the last call answered with.
func (w *world) workloadAnswer() (alphonemcp.WorkloadOutput, error) {
	var out alphonemcp.WorkloadOutput
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

// registerWorkloadSteps binds the workload summary steps.
func registerWorkloadSteps(sc *godog.ScenarioContext, t *testing.T) {
	registerSessionSteps(sc, t)

	sc.Given(`^the agent is connected with the token$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		if err := w.connect(ctx, w.secret); err != nil {
			return err
		}
		if w.session == nil {
			return fmt.Errorf("the agent could not connect: %v", w.connErr)
		}
		return nil
	})

	sc.Given(`^(\d+) open tasks due today$`, func(ctx context.Context, count int) error {
		w := worldFrom(ctx)
		return w.seedTasks(ctx, w.ownerID, count, today(), task.StatusOpen)
	})

	sc.Given(`^(\d+) open tasks due yesterday$`, func(ctx context.Context, count int) error {
		w := worldFrom(ctx)
		return w.seedTasks(ctx, w.ownerID, count, today().AddDate(0, 0, -1), task.StatusOpen)
	})

	sc.Given(`^(\d+) done tasks due today$`, func(ctx context.Context, count int) error {
		w := worldFrom(ctx)
		return w.seedTasks(ctx, w.ownerID, count, today(), task.StatusDone)
	})

	sc.Given(`^a second user holding (\d+) open tasks due today$`, func(ctx context.Context, count int) error {
		w := worldFrom(ctx)
		other, err := w.addUser(ctx, "second@example.com", "Ada Lovelace")
		if err != nil {
			return err
		}
		return w.seedTasks(ctx, other, count, today(), task.StatusOpen)
	})

	sc.When(`^the agent calls workload_summary$`, func(ctx context.Context) error {
		return worldFrom(ctx).callTool(ctx, "workload_summary", nil)
	})

	sc.Then(`^the structured answer counts (\d+) due today and (\d+) overdue$`,
		func(ctx context.Context, due, overdue int) error {
			answer, err := worldFrom(ctx).workloadAnswer()
			if err != nil {
				return err
			}
			if answer.DueToday != due || answer.Overdue != overdue {
				return fmt.Errorf("counts = %d due and %d overdue, want %d and %d",
					answer.DueToday, answer.Overdue, due, overdue)
			}
			return nil
		})

	sc.Then(`^the structured answer is marked capped$`, func(ctx context.Context) error {
		answer, err := worldFrom(ctx).workloadAnswer()
		if err != nil {
			return err
		}
		if !answer.Capped {
			return fmt.Errorf("capped = false, want the answer marked capped")
		}
		return nil
	})
}
