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
	due: tasks(date: $today, status: "open", first: $first) {
		edges { node { id } } pageInfo { hasNextPage }
	}
	overdue: tasks(dueBefore: $today, status: "open", first: $first) {
		edges { node { id } } pageInfo { hasNextPage }
	}
}`

// edgeCount is a connection read for its row count and next page flag.
type edgeCount struct {
	Edges    []struct{} `json:"edges"`
	PageInfo pageInfo   `json:"pageInfo"`
}

// pageInfo is the part of a connection that says whether more rows exist.
type pageInfo struct {
	HasNextPage bool `json:"hasNextPage"`
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
	out.Capped = data.Due.PageInfo.HasNextPage || data.Overdue.PageInfo.HasNextPage
	return nil, out, nil
}
