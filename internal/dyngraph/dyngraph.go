// SPDX-License-Identifier: Elastic-2.0

// Package dyngraph serves runtime defined fields as real GraphQL fields over
// the generated executable schema, each tenant only its own.
package dyngraph

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/99designs/gqlgen/graphql"
	"github.com/google/uuid"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/vektah/gqlparser/v2/ast"

	"github.com/gopherium/alphone/sdk"
)

// carrierField is the compiled field every runtime field rides on.
const carrierField = "field"

// carrierArg names the carrier argument holding the runtime field name.
const carrierArg = "name"

// Build returns the generated executable schema serving the given widened
// schema. A nil schema means the compiled in one.
type Build func(*ast.Schema) graphql.ExecutableSchema

// Serve wraps one executable schema in what the host answers a request with.
type Serve[T any] func(graphql.ExecutableSchema) T

// graphEntry is one tenant's served graph and the source stamps it was built from.
type graphEntry[T any] struct {
	stamps []uint64
	served T
}

// Graphs serves each tenant the generated schema widened by its own field catalogue.
type Graphs[T any] struct {
	build    Build
	serve    Serve[T]
	base     *ast.Schema
	plain    T
	sources  []sdk.FieldSource
	held     *lru.Cache[uuid.UUID, graphEntry[T]]
	building sync.Mutex
}

// New returns the graphs serving build widened by each tenant's fields from the sources, holding up to held tenants.
func New[T any](build Build, serve Serve[T], held int, sources ...sdk.FieldSource) *Graphs[T] {
	compiled := build(nil)
	cache, _ := lru.New[uuid.UUID, graphEntry[T]](cmp.Or(max(held, 0), sdk.DefaultTenantsHeld))
	return &Graphs[T]{
		build:   build,
		serve:   serve,
		base:    compiled.Schema(),
		plain:   serve(compiled),
		sources: sources,
		held:    cache,
	}
}

// Plain returns the compiled graph, the one served outside any tenant's fields.
func (g *Graphs[T]) Plain() T {
	return g.plain
}

// For returns the calling tenant's graph, built again only when a source's stamp moved.
func (g *Graphs[T]) For(ctx context.Context) T {
	if len(g.sources) == 0 {
		return g.plain
	}
	tenant := sdk.TenantOrDefault(ctx)
	cached, found := g.held.Get(tenant)
	stamps, fields, err := g.collect(ctx)
	if err != nil {
		if found {
			return cached.served
		}
		return g.plain
	}
	if found && slices.Equal(stamps, cached.stamps) {
		return cached.served
	}
	return g.builtFor(tenant, stamps, fields)
}

// builtFor returns the tenant's graph for the given stamps, building it unless a caller missing it too already did.
func (g *Graphs[T]) builtFor(tenant uuid.UUID, stamps []uint64, fields []sdk.GraphField) T {
	g.building.Lock()
	defer g.building.Unlock()
	if cached, found := g.held.Get(tenant); found && slices.Equal(stamps, cached.stamps) {
		return cached.served
	}
	served := g.graphOf(fields)
	g.held.Add(tenant, graphEntry[T]{stamps: stamps, served: served})
	return served
}

// collect gathers every source's stamp and fields for the calling tenant.
func (g *Graphs[T]) collect(ctx context.Context) ([]uint64, []sdk.GraphField, error) {
	stamps := make([]uint64, len(g.sources))
	var fields []sdk.GraphField
	for i, source := range g.sources {
		stamp, held, err := source.FieldsSnapshot(ctx)
		if err != nil {
			return nil, nil, err
		}
		stamps[i] = stamp
		fields = append(fields, held...)
	}
	return stamps, fields, nil
}

// graphOf returns the graph serving the given fields, the plain one when none can be served.
func (g *Graphs[T]) graphOf(fields []sdk.GraphField) T {
	byEntity := groupByEntity(g.base, fields)
	if len(byEntity) == 0 {
		return g.plain
	}
	schema := widen(g.base, byEntity)
	return g.serve(&widened{inner: g.build(schema), schema: schema, base: g.base})
}

// groupByEntity indexes fields by entity and name, dropping unservable ones.
func groupByEntity(base *ast.Schema, fields []sdk.GraphField) map[string]map[string]sdk.GraphField {
	byEntity := make(map[string]map[string]sdk.GraphField)
	for _, field := range fields {
		owner := base.Types[field.Entity]
		if owner == nil || owner.Fields.ForName(carrierField) == nil {
			continue
		}
		if owner.Fields.ForName(field.Name) != nil {
			continue
		}
		if byEntity[field.Entity] == nil {
			byEntity[field.Entity] = make(map[string]sdk.GraphField)
		}
		byEntity[field.Entity][field.Name] = field
	}
	return byEntity
}

// widen returns a copy of base whose entities carry the catalogue fields.
func widen(base *ast.Schema, byEntity map[string]map[string]sdk.GraphField) *ast.Schema {
	widened := *base
	widened.Types = make(map[string]*ast.Definition, len(base.Types))
	for name, def := range base.Types {
		widened.Types[name] = def
	}
	for entity, fields := range byEntity {
		source := base.Types[entity]
		copied := *source
		copied.Fields = append(ast.FieldList(nil), source.Fields...)
		for _, field := range fields {
			copied.Fields = append(copied.Fields, &ast.FieldDefinition{
				Name:        field.Name,
				Type:        ast.NamedType(field.Type, nil),
				Description: "operator defined field",
			})
		}
		widened.Types[entity] = &copied
	}
	widened.Query = widened.Types["Query"]
	widened.Mutation = widened.Types["Mutation"]
	widened.Subscription = widened.Types["Subscription"]
	return &widened
}

// widened serves one tenant's widened schema over the generated executor.
type widened struct {
	inner  graphql.ExecutableSchema
	schema *ast.Schema
	base   *ast.Schema
}

// Schema reports the widened schema the tenant's requests validate and introspect against.
func (w *widened) Schema() *ast.Schema {
	return w.schema
}

// Complexity prices a field.
func (w *widened) Complexity(
	ctx context.Context, typeName, field string, childComplexity int, args map[string]any,
) (int, bool) {
	return w.inner.Complexity(ctx, typeName, field, childComplexity, args)
}

// Exec runs the generated executor over the rewritten operation.
func (w *widened) Exec(ctx context.Context) graphql.ResponseHandler {
	opCtx := graphql.GetOperationContext(ctx)
	rewrite(w.base, opCtx.Operation.SelectionSet)
	for _, fragment := range opCtx.Doc.Fragments {
		rewrite(w.base, fragment.SelectionSet)
	}
	return w.inner.Exec(ctx)
}

// rewrite points every field the compiled schema lacks at the carrier.
func rewrite(base *ast.Schema, selections ast.SelectionSet) {
	for _, selection := range selections {
		switch node := selection.(type) {
		case *ast.Field:
			if !rewriteField(base, node) {
				rewrite(base, node.SelectionSet)
			}
		case *ast.InlineFragment:
			rewrite(base, node.SelectionSet)
		case *ast.FragmentSpread:
		}
	}
}

// rewriteField points one field at the carrier, reporting whether it did.
func rewriteField(base *ast.Schema, field *ast.Field) bool {
	if field.ObjectDefinition == nil || strings.HasPrefix(field.Name, "__") {
		return false
	}
	owner := base.Types[field.ObjectDefinition.Name]
	if owner == nil || owner.Fields.ForName(field.Name) != nil {
		return false
	}
	carrier := owner.Fields.ForName(carrierField)
	if carrier == nil {
		return false
	}
	field.Arguments = ast.ArgumentList{{
		Name:  carrierArg,
		Value: &ast.Value{Raw: field.Name, Kind: ast.StringValue},
	}}
	field.Name = carrierField
	field.Definition = carrier
	return true
}
