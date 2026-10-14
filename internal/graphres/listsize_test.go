// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/99designs/gqlgen/complexity"
	"github.com/99designs/gqlgen/graphql"
	gqlparser "github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/validator/rules"

	"github.com/gopherium/alphone/internal/graphres"
)

// markedSchema is a schema whose lists price themselves with @listSize, one by one argument and one by either of two.
const markedSchema = `
directive @listSize(slicingArguments: [String!]!) on FIELD_DEFINITION
type Query {
  things(limit: Int, offset: Int): [Thing!]! @listSize(slicingArguments: ["limit"])
  either(first: Int, last: Int): [Thing!]! @listSize(slicingArguments: ["first", "last"])
  plain(limit: Int): [Thing!]!
}
type Thing { id: ID! name: String! }
`

// markedExecutable returns the priced executable schema serving markedSchema.
func markedExecutable(t *testing.T) graphql.ExecutableSchema {
	t.Helper()
	schema, err := gqlparser.LoadSchema(&ast.Source{Name: "marked", Input: markedSchema})
	if err != nil {
		t.Fatalf("loading the marked schema: %v", err)
	}
	return graphres.ExecutableSchemaOver(nil, schema)
}

// costWith computes the priced complexity of a query document over schema with vars.
func costWith(t *testing.T, schema graphql.ExecutableSchema, doc string, vars map[string]any) int {
	t.Helper()
	query, err := gqlparser.LoadQueryWithRules(schema.Schema(), doc, rules.NewDefaultRules())
	if err != nil {
		t.Fatalf("parsing document: %v", err)
	}
	return complexity.Calculate(t.Context(), schema, query.Operations[0], vars)
}

func TestAMarkedListIsPricedByTheRowsItAsksFor(t *testing.T) {
	t.Parallel()

	schema := markedExecutable(t)

	for name, tc := range map[string]struct {
		doc  string
		want int
	}{
		"its limit times its row":          {`{ things(limit: 7) { id name } }`, 14},
		"the default page without a limit": {`{ things { id name } }`, graphres.DefaultPageSize * 2},
		"the default page for a zero":      {`{ things(limit: 0) { id } }`, graphres.DefaultPageSize},
		"the largest slicing argument":     {`{ either(first: 3, last: 8) { id } }`, 8},
		"an unmarked list as one row":      {`{ plain(limit: 7) { id } }`, 2},
	} {
		if got := costWith(t, schema, tc.doc, nil); got != tc.want {
			t.Errorf("%s: %s costs %d, want %d", name, tc.doc, got, tc.want)
		}
	}
}

func TestAMarkedListReadsItsLimitFromAVariable(t *testing.T) {
	t.Parallel()

	schema := markedExecutable(t)

	for name, limit := range map[string]any{"a whole number": int64(9), "a JSON number": json.Number("9")} {
		got := costWith(t, schema, `query($n: Int) { things(limit: $n) { id } }`, map[string]any{"n": limit})

		if got != 9 {
			t.Errorf("%s: costs %d, want the 9 rows the variable asks for", name, got)
		}
	}
}

func TestAMarkedListSaturatesInsteadOfOverflowing(t *testing.T) {
	t.Parallel()

	got := costWith(t, markedExecutable(t), `query($n: Int) { things(limit: $n) { id name } }`,
		map[string]any{"n": int64(math.MaxInt64)})

	if got != math.MaxInt {
		t.Errorf("costs %d, want the largest price instead of an overflow", got)
	}
}

func TestTheImportScreenChunkFitsUnderTheCap(t *testing.T) {
	t.Parallel()

	rows := slices.Max(graphres.DefaultListPageSizes())
	chunk := fmt.Sprintf(`query ImportRows($id: UUID!) {
		importJob(id: $id) {
			__typename id
			rows(limit: %d, offset: 0) {
				__typename id position cells outcome reason { __typename code meta }
			}
		}
	}`, rows)

	cost := operationCost(t, chunk)

	t.Logf("a chunk of %d rows costs %d of the %d cap", rows, cost, graphres.ComplexityLimit)
	if cost > graphres.ComplexityLimit {
		t.Errorf("a chunk of %d rows costs %d, want within the cap %d", rows, cost, graphres.ComplexityLimit)
	}
	if cost < rows {
		t.Errorf("a chunk of %d rows costs %d, want the rows priced", rows, cost)
	}
}
