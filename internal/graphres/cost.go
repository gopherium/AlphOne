// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/errcode"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// CostLimit rejects an operation whose priced cost passes ComplexityLimit.
type CostLimit struct {
	schema graphql.ExecutableSchema
}

// ExtensionName names the limit among the handler extensions.
func (*CostLimit) ExtensionName() string {
	return "CostLimit"
}

// Validate keeps the executable schema the limit prices operations by.
func (c *CostLimit) Validate(schema graphql.ExecutableSchema) error {
	c.schema = schema
	return nil
}

// MutateOperationContext rejects the operation when its priced cost passes ComplexityLimit.
func (c *CostLimit) MutateOperationContext(ctx context.Context, rc *graphql.OperationContext) *gqlerror.Error {
	cost := OperationCost(ctx, c.schema, rc.Operation, rc.Variables)
	if cost <= ComplexityLimit {
		return nil
	}
	rejected := gqlerror.Errorf("operation has complexity %d, which exceeds the limit of %d", cost, ComplexityLimit)
	errcode.Set(rejected, "COMPLEXITY_LIMIT_EXCEEDED")
	return rejected
}

// OperationCost returns the priced cost of the operation, each named fragment priced once.
func OperationCost(
	ctx context.Context, schema graphql.ExecutableSchema, op *ast.OperationDefinition, variables map[string]any,
) int {
	walk := costWalk{ctx: ctx, schema: schema, variables: variables, fragments: map[string]int{}}
	return walk.setCost(op.SelectionSet)
}

// costWalk prices the selections of one operation, keeping the cost of each named fragment it priced.
type costWalk struct {
	ctx       context.Context
	schema    graphql.ExecutableSchema
	variables map[string]any
	fragments map[string]int
}

// setCost returns the summed cost of the fields, inline fragments and fragment spreads of a selection set.
func (w costWalk) setCost(set ast.SelectionSet) int {
	total := 0
	for _, selection := range set {
		switch s := selection.(type) {
		case *ast.Field:
			total = saturatedSum(total, w.fieldCost(s))
		case *ast.InlineFragment:
			total = saturatedSum(total, w.setCost(s.SelectionSet))
		case *ast.FragmentSpread:
			total = saturatedSum(total, w.fragmentCost(s))
		}
	}
	return total
}

// fieldCost returns the price the schema gives a field over its own selections, a schema read priced at nothing.
func (w costWalk) fieldCost(field *ast.Field) int {
	if field.Definition.Type.Name() == "__Schema" {
		return 0
	}
	child := w.setCost(field.SelectionSet)
	arguments := field.ArgumentMap(w.variables)
	price, priced := w.schema.Complexity(w.ctx, field.ObjectDefinition.Name, field.Name, child, arguments)
	if priced && price >= 1 {
		return price
	}
	return saturatedSum(1, child)
}

// fragmentCost returns the cost of the named fragment a spread reads, priced once per operation.
func (w costWalk) fragmentCost(spread *ast.FragmentSpread) int {
	if cost, priced := w.fragments[spread.Name]; priced {
		return cost
	}
	cost := w.setCost(spread.Definition.SelectionSet)
	w.fragments[spread.Name] = cost
	return cost
}
