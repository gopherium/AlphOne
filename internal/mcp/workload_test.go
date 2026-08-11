// SPDX-License-Identifier: Elastic-2.0

package mcp

import (
	"fmt"
	"strings"
	"testing"
)

// countingGraph answers a workload document with the given edge counts.
func countingGraph(due, overdue int) string {
	edges := func(n int) string {
		rows := make([]string, n)
		for i := range rows {
			rows[i] = `{"node":{"id":"t"}}`
		}
		return "[" + strings.Join(rows, ",") + "]"
	}
	return fmt.Sprintf(`{"data":{"due":{"edges":%s},"overdue":{"edges":%s}}}`, edges(due), edges(overdue))
}

func TestWorkloadCountsTheAnswer(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		due, overdue int
		capped       bool
	}{
		"work on both":       {due: 10, overdue: 3},
		"a free day":         {due: 0, overdue: 0},
		"only overdue":       {due: 0, overdue: 2},
		"a capped day":       {due: workloadPage, overdue: 0, capped: true},
		"a capped backlog":   {due: 0, overdue: workloadPage, capped: true},
		"both counts capped": {due: workloadPage, overdue: workloadPage, capped: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			run := &tools{graph: answering(countingGraph(tc.due, tc.overdue))}

			result, out, err := run.workload(t.Context(), WorkloadInput{})

			if err != nil {
				t.Fatalf("workload() error = %v, want nil", err)
			}
			if result != nil {
				t.Errorf("result = %v, want nil so the SDK mirrors the data", result)
			}
			if out.DueToday != tc.due || out.Overdue != tc.overdue {
				t.Errorf("counts = %d and %d, want %d and %d", out.DueToday, out.Overdue, tc.due, tc.overdue)
			}
			if out.Capped != tc.capped {
				t.Errorf("capped = %t, want %t", out.Capped, tc.capped)
			}
		})
	}
}

func TestWorkloadReportsAGraphRefusal(t *testing.T) {
	t.Parallel()

	run := &tools{graph: answering(`{"errors":[{"message":"refused"}]}`)}

	_, _, err := run.workload(t.Context(), WorkloadInput{})

	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Errorf("error = %v, want the refusal", err)
	}
}

func TestWorkloadAsksForTodayAndTheBacklog(t *testing.T) {
	t.Parallel()

	var body string
	run := &tools{graph: recording(&body, countingGraph(0, 0))}

	if _, _, err := run.workload(t.Context(), WorkloadInput{}); err != nil {
		t.Fatalf("workload() error = %v, want nil", err)
	}

	for _, want := range []string{`"today"`, "due:", "overdue:", `status: \"open\"`, `"first":200`} {
		if !strings.Contains(body, want) {
			t.Errorf("document = %s, want it to carry %s", body, want)
		}
	}
}
