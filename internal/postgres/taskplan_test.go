// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/task"
	"github.com/gopherium/alphone/internal/testdb"
)

// listingRowCompare is the left side of the keyset compare every task listing must hold inside an Index Cond.
const listingRowCompare = "ROW(CASE WHEN (status = 'open'::text) THEN ((- priority))::integer ELSE 0 END, " +
	"due_on, CASE WHEN (status = 'open'::text) THEN COALESCE(due_time, '24:00:00'::time without time zone) " +
	"ELSE '24:00:00'::time without time zone END, id) > ROW("

// sentQuery is one statement a pool sent with its arguments.
type sentQuery struct {
	sql  string
	args []any
}

// queryCapture records every statement a pool sends.
type queryCapture struct {
	mu   sync.Mutex
	sent []sentQuery
}

// TraceQueryStart records the statement and its arguments.
func (c *queryCapture) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, sentQuery{sql: data.SQL, args: data.Args})
	return ctx
}

// TraceQueryEnd records nothing at the end of a statement.
func (*queryCapture) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// last returns the statement the pool sent most recently.
func (c *queryCapture) last() sentQuery {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sent[len(c.sent)-1]
}

// planNode is one node of a JSON query plan.
type planNode struct {
	NodeType  string     `json:"Node Type"`
	IndexName string     `json:"Index Name"`
	IndexCond string     `json:"Index Cond"`
	Plans     []planNode `json:"Plans"`
}

// nodes returns the node and every node below it.
func (n planNode) nodes() []planNode {
	all := []planNode{n}
	for _, child := range n.Plans {
		all = append(all, child.nodes()...)
	}
	return all
}

// newTracedPool returns a pool over a fresh migrated test database that reports every statement to tracer.
func newTracedPool(t *testing.T, tracer pgx.QueryTracer) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	config, err := pgxpool.ParseConfig(testdb.New(t, testdb.Migrator()))
	if err != nil {
		t.Fatalf("parsing the pool config: %v", err)
	}
	config.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// planOf returns the plan Postgres picks for the sent statement with sequential and bitmap scans turned off.
func planOf(t *testing.T, pool *pgxpool.Pool, sent sentQuery) planNode {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("starting the explain transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(t.Context())) }()
	for _, setting := range []string{"SET LOCAL enable_seqscan = off", "SET LOCAL enable_bitmapscan = off"} {
		if _, err := tx.Exec(t.Context(), setting); err != nil {
			t.Fatalf("%s: %v", setting, err)
		}
	}
	var raw []byte
	if err := tx.QueryRow(t.Context(), "EXPLAIN (FORMAT JSON)\n"+sent.sql, sent.args...).Scan(&raw); err != nil {
		t.Fatalf("explaining the listing: %v", err)
	}
	var plans []struct {
		Plan planNode `json:"Plan"`
	}
	if err := json.Unmarshal(raw, &plans); err != nil || len(plans) != 1 {
		t.Fatalf("reading the plan %s: %v", raw, err)
	}
	return plans[0].Plan
}

// wantIndexOrder fails the test when the plan sorts or holds the keyset compare anywhere but the Index Cond of index.
func wantIndexOrder(t *testing.T, plan planNode, index string) {
	t.Helper()
	var compared []string
	for _, node := range plan.nodes() {
		if strings.Contains(node.NodeType, "Sort") {
			t.Errorf("plan holds a %s node, want the index order", node.NodeType)
		}
		if strings.Contains(node.IndexCond, listingRowCompare) {
			compared = append(compared, node.IndexName)
		}
	}
	if !slices.Equal(compared, []string{index}) {
		t.Errorf("indexes holding the keyset compare %q = %v, want [%s], plan %+v", listingRowCompare, compared, index, plan)
	}
}

// listing reads one task listing and names the index its plan walks.
type listing struct {
	read  func(status string, page task.Page) error
	index string
}

func TestOpenAndDoneTaskListingsSkipTheSortInTheCustomPlan(t *testing.T) {
	t.Parallel()

	capture := &queryCapture{}
	pool := newTracedPool(t, capture)
	store := postgres.NewTaskStore(pool)
	held := mustContact(t, "Maria Perez")
	if err := postgres.NewContactStore(pool).Create(t.Context(), held); err != nil {
		t.Fatalf("storing the contact: %v", err)
	}
	assignee := uuid.Must(uuid.NewV7())
	stored := storeAll(t, store, assignee, []task.Input{
		{Title: "Call her at nine", DueOn: taskDay, ContactID: held.ID, Priority: 1, DueTime: clockAt(t, "09:00")},
		{Title: "Send her the quote", DueOn: taskDay, ContactID: held.ID},
	})
	completed(t, pool, stored[1].ID)
	stored[1].Status = task.StatusDone
	lists := map[string]listing{
		"day": {
			read: func(status string, page task.Page) error {
				_, err := store.ListForDay(t.Context(), assignee, taskDay, status, page)
				return err
			},
			index: "tasks_assignee_status_day_rank_time_idx",
		},
		"backlog": {
			read: func(status string, page task.Page) error {
				_, err := store.ListDueBefore(t.Context(), assignee, taskDay.AddDate(0, 0, 1), status, page)
				return err
			},
			index: "tasks_assignee_status_rank_time_idx",
		},
		"contact": {
			read: func(status string, page task.Page) error {
				_, err := store.ListForContact(t.Context(), held.ID, status, page)
				return err
			},
			index: "tasks_contact_status_rank_time_idx",
		},
	}
	pages := map[string]task.Page{
		"open first page": {Limit: 51},
		"open mid walk":   task.PageAfter(stored[0], 51),
		"done first page": {Limit: 51},
		"done mid walk":   task.PageAfter(stored[1], 51),
	}

	for list, listed := range lists {
		for name, page := range pages {
			status, _, _ := strings.Cut(name, " ")
			if err := listed.read(status, page); err != nil {
				t.Fatalf("%s %s: listing error = %v, want nil", list, name, err)
			}
			t.Run(list+" "+name, func(t *testing.T) { wantIndexOrder(t, planOf(t, pool, capture.last()), listed.index) })
		}
	}
}
