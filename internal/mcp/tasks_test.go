// SPDX-License-Identifier: Elastic-2.0

package mcp

import (
	"strings"
	"testing"
)

// oneTaskGraph answers the task document with a single linked task.
const oneTaskGraph = `{"data":{"tasks":{"edges":[{"node":{
	"id":"t1","title":"Call Maria Perez back","dueOn":"2026-08-11","status":"open","priority":2,
	"assigneeId":"u1","contactId":"c1","contact":{"id":"c1","name":"Maria Perez"}
}}]}}}`

func TestTasksReadsTheListedTasks(t *testing.T) {
	t.Parallel()

	run := &tools{graph: answering(oneTaskGraph)}

	result, out, err := run.tasks(t.Context(), TasksInput{})

	if err != nil {
		t.Fatalf("tasks() error = %v, want nil", err)
	}
	if result != nil {
		t.Errorf("result = %v, want nil so the SDK mirrors the data", result)
	}
	if len(out.Tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(out.Tasks))
	}
	got := out.Tasks[0]
	want := TaskItem{
		ID: "t1", Title: "Call Maria Perez back", DueOn: "2026-08-11", Status: "open",
		Priority: 2, AssigneeID: "u1", ContactID: "c1", ContactName: "Maria Perez",
	}
	if got != want {
		t.Errorf("task = %+v, want %+v", got, want)
	}
}

func TestTasksLeavesAnUnlinkedTaskWithoutAContact(t *testing.T) {
	t.Parallel()

	const unlinked = `{"data":{"tasks":{"edges":[{"node":{
		"id":"t1","title":"Send the quote","dueOn":"2026-08-11","status":"open","priority":0
	}}]}}}`
	run := &tools{graph: answering(unlinked)}

	_, out, err := run.tasks(t.Context(), TasksInput{})

	if err != nil {
		t.Fatalf("tasks() error = %v, want nil", err)
	}
	if out.Tasks[0].ContactID != "" || out.Tasks[0].ContactName != "" {
		t.Errorf("task = %+v, want no contact", out.Tasks[0])
	}
}

func TestTasksAnswersAnEmptyListNotNull(t *testing.T) {
	t.Parallel()

	run := &tools{graph: answering(`{"data":{"tasks":{"edges":[]}}}`)}

	_, out, err := run.tasks(t.Context(), TasksInput{})

	if err != nil {
		t.Fatalf("tasks() error = %v, want nil", err)
	}
	if out.Tasks == nil {
		t.Error("tasks = nil, want an empty list")
	}
	if len(out.Tasks) != 0 {
		t.Errorf("tasks = %d, want none", len(out.Tasks))
	}
}

func TestTasksChoosesTheFilter(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		in    TasksInput
		wants []string
	}{
		"today by default": {
			in:    TasksInput{},
			wants: []string{`"date"`, `"status":"open"`, `"first":50`},
		},
		"a chosen day": {
			in:    TasksInput{Date: "2026-09-01"},
			wants: []string{`"date":"2026-09-01"`},
		},
		"the backlog": {
			in:    TasksInput{DueBefore: "2026-08-11"},
			wants: []string{`"dueBefore":"2026-08-11"`},
		},
		"a chosen status": {
			in:    TasksInput{Status: "done"},
			wants: []string{`"status":"done"`},
		},
		"a chosen limit": {
			in:    TasksInput{Limit: 10},
			wants: []string{`"first":10`},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var body string
			run := &tools{graph: recording(&body, `{"data":{"tasks":{"edges":[]}}}`)}

			if _, _, err := run.tasks(t.Context(), tc.in); err != nil {
				t.Fatalf("tasks() error = %v, want nil", err)
			}

			for _, want := range tc.wants {
				if !strings.Contains(body, want) {
					t.Errorf("request = %s, want it to carry %s", body, want)
				}
			}
		})
	}
}

func TestTasksSendsOnlyTheNamedFilter(t *testing.T) {
	t.Parallel()

	var body string
	run := &tools{graph: recording(&body, `{"data":{"tasks":{"edges":[]}}}`)}

	if _, _, err := run.tasks(t.Context(), TasksInput{DueBefore: "2026-08-11"}); err != nil {
		t.Fatalf("tasks() error = %v, want nil", err)
	}

	if strings.Contains(body, `"date"`) {
		t.Errorf("request = %s, want no date beside the backlog filter", body)
	}
}

func TestTasksReportsAGraphRefusal(t *testing.T) {
	t.Parallel()

	run := &tools{graph: answering(
		`{"errors":[{"message":"exactly one filter","extensions":{"code":"VALIDATION"}}]}`)}

	_, _, err := run.tasks(t.Context(), TasksInput{Date: "2026-08-11", DueBefore: "2026-08-11"})

	if err == nil {
		t.Fatal("tasks() error = nil, want the refusal")
	}
	if !strings.Contains(err.Error(), "VALIDATION") {
		t.Errorf("error = %v, want the validation code", err)
	}
}
