// SPDX-License-Identifier: Elastic-2.0

package mcp

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// workloadPage bounds how deep a workload count reads.
const workloadPage = 200

// workloadDocument counts today's open tasks beside the open backlog.
const workloadDocument = `query($today: Date!, $first: Int!) {
	due: tasks(date: $today, status: "open", first: $first) { edges { node { id } } }
	overdue: tasks(dueBefore: $today, status: "open", first: $first) { edges { node { id } } }
}`

// edgeCount is a connection read only for how many rows it carries.
type edgeCount struct {
	Edges []struct{} `json:"edges"`
}

// workloadData is what the workload document answers with.
type workloadData struct {
	Due     edgeCount `json:"due"`
	Overdue edgeCount `json:"overdue"`
}

// workload answers how much work the caller holds.
func (t *tools) workload(ctx context.Context, _ WorkloadInput) (*mcp.CallToolResult, WorkloadOutput, error) {
	var data workloadData
	variables := map[string]any{
		"today": time.Now().UTC().Format(time.DateOnly),
		"first": workloadPage,
	}
	if err := t.execute(ctx, workloadDocument, variables, &data); err != nil {
		return nil, WorkloadOutput{}, err
	}
	out := WorkloadOutput{
		DueToday: len(data.Due.Edges),
		Overdue:  len(data.Overdue.Edges),
	}
	out.Capped = out.DueToday >= workloadPage || out.Overdue >= workloadPage
	return nil, out, nil
}
