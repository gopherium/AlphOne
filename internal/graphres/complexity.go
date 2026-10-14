// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/99designs/gqlgen/complexity"
	"github.com/99designs/gqlgen/graphql"
	"github.com/google/uuid"
	gqlparser "github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/validator/rules"

	"github.com/gopherium/alphone/graph"
	"github.com/gopherium/alphone/graph/model"
)

// ComplexityLimit caps the priced cost of one operation.
const ComplexityLimit = 2500

// oneFieldRow reads one field of one row through the contact page, the cheapest row a core list prices.
const oneFieldRow = `{ contactPage(limit: 1) { items { id } } }`

// LargestPricedPage returns the most rows a core list page holds while one field a row stays within ComplexityLimit.
func LargestPricedPage() int {
	schema := ExecutableSchema(nil)
	query := gqlparser.MustLoadQueryWithRules(schema.Schema(), oneFieldRow, rules.NewDefaultRules())
	return ComplexityLimit / complexity.Calculate(context.Background(), schema, query.Operations[0], nil)
}

// pageCost resolves a page size argument into a row multiplier, a missing or empty one priced as the unsized page.
func pageCost(first *int, unsized int) int {
	if first == nil || *first < 1 {
		return unsized
	}
	return *first
}

// boundedQueries is a query resolver set naming the page bounds it answers.
type boundedQueries interface {
	pageBounds() Paging
}

// pageBounds returns the page bounds the core lists answer.
func (q QueryResolvers) pageBounds() Paging {
	return q.root.Paging
}

// pagingOf returns the page bounds the root's core queries answer, the defaults for a root naming none.
func pagingOf(root graph.ResolverRoot) Paging {
	if root == nil {
		return Paging{}
	}
	if queries, ok := root.Query().(boundedQueries); ok {
		return queries.pageBounds()
	}
	return Paging{}
}

// ExecutableSchema builds the priced executable schema over the resolver root.
func ExecutableSchema(root graph.ResolverRoot) graphql.ExecutableSchema {
	return ExecutableSchemaOver(root, nil)
}

// ExecutableSchemaOver builds the priced executable schema serving schema in place of the compiled in one.
func ExecutableSchemaOver(root graph.ResolverRoot, schema *ast.Schema) graphql.ExecutableSchema {
	cfg := graph.Config{Resolvers: root, Schema: schema}
	unsized := pagingOf(root).size()
	cfg.Complexity.Query.Contacts = func(child int, _ *string, first *int, _ *string) int {
		return pageCost(first, unsized) * child
	}
	cfg.Complexity.Query.ContactPage = func(
		child int, _ *string, _ []string, _ model.ContactOrderBy, _ model.SortOrder, limit, _ *int,
	) int {
		return pageCost(limit, unsized) * child
	}
	cfg.Complexity.Query.Tasks = func(child int, _, _ *time.Time, _ *uuid.UUID, _ *string, first *int, _ *string) int {
		return pageCost(first, unsized) * child
	}
	cfg.Complexity.Contact.Tasks = func(child int, _ *string, first *int, _ *string) int {
		return pageCost(first, unsized) * child
	}
	inner := graph.NewExecutableSchema(cfg)
	return pricedSchema{ExecutableSchema: inner, sizes: listSizesOf(inner.Schema()), unsized: unsized}
}

// listSizeDirective names the directive a list field prices itself with.
const listSizeDirective = "listSize"

// pricedSchema prices every list field the schema marks with @listSize by the rows it asks for.
type pricedSchema struct {
	graphql.ExecutableSchema
	sizes   map[string][]string
	unsized int
}

// Complexity prices a marked list as its page times its row, and any other field as the generated schema does.
func (s pricedSchema) Complexity(
	ctx context.Context, typeName, field string, child int, args map[string]any,
) (int, bool) {
	slicing, marked := s.sizes[typeName+"."+field]
	if !marked {
		return s.ExecutableSchema.Complexity(ctx, typeName, field, child, args)
	}
	return saturatedProduct(pageOf(slicing, args, s.unsized), child), true
}

// listSizesOf maps every field the schema marks with @listSize to the arguments that size its page.
func listSizesOf(schema *ast.Schema) map[string][]string {
	sizes := map[string][]string{}
	for _, def := range schema.Types {
		for _, field := range def.Fields {
			if sized := field.Directives.ForName(listSizeDirective); sized != nil {
				sizes[def.Name+"."+field.Name] = slicingArguments(sized)
			}
		}
	}
	return sizes
}

// slicingArguments returns the argument names a @listSize directive sizes its page by.
func slicingArguments(sized *ast.Directive) []string {
	listed, _ := sized.ArgumentMap(nil)["slicingArguments"].([]any)
	names := make([]string, len(listed))
	for i, name := range listed {
		names[i] = fmt.Sprint(name)
	}
	return names
}

// pageOf returns the largest page the slicing arguments ask for, the unsized page when they ask for none.
func pageOf(slicing []string, args map[string]any, unsized int) int {
	page := 0
	for _, name := range slicing {
		asked, _ := graphql.UnmarshalInt(args[name])
		page = max(page, asked)
	}
	if page < 1 {
		return unsized
	}
	return page
}

// saturatedProduct multiplies a page by its row cost, answering the largest price instead of overflowing.
func saturatedProduct(page, row int) int {
	if row > 0 && page > math.MaxInt/row {
		return math.MaxInt
	}
	return page * row
}
