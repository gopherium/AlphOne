// SPDX-License-Identifier: Elastic-2.0

// Package dyngraph serves runtime defined fields as real GraphQL fields over
// the generated executable schema.
package dyngraph

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/99designs/gqlgen/graphql"
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

// snapshot is one immutable view of the catalogue, its schema and its executor.
type snapshot struct {
	inner    graphql.ExecutableSchema
	schema   *ast.Schema
	byEntity map[string]map[string]sdk.GraphField
	versions []uint64
}

// Schema serves the generated schema widened by every source's field catalogue.
type Schema struct {
	build   Build
	base    *ast.Schema
	sources []sdk.FieldSource
	reload  sync.Mutex
	live    atomic.Pointer[snapshot]
}

// New returns a schema serving build widened by the given sources.
func New(build Build, sources ...sdk.FieldSource) *Schema {
	s := &Schema{build: build, sources: sources}
	compiled := build(nil)
	s.base = compiled.Schema()
	s.live.Store(&snapshot{inner: compiled, schema: s.base})
	s.refresh(context.Background())
	return s
}

// refresh swaps the served snapshot when any source moved past it.
func (s *Schema) refresh(ctx context.Context) {
	if len(s.sources) == 0 {
		return
	}
	versions, fields, err := s.collect(ctx)
	if err != nil || versionsEqual(versions, s.live.Load().versions) {
		return
	}
	s.reload.Lock()
	defer s.reload.Unlock()
	byEntity := groupByEntity(s.base, fields)
	widened := widen(s.base, byEntity)
	served := widened
	if widened == s.base {
		served = nil
	}
	s.live.Store(&snapshot{inner: s.build(served), schema: widened, byEntity: byEntity, versions: versions})
}

// collect gathers every source's snapshot.
func (s *Schema) collect(ctx context.Context) ([]uint64, []sdk.GraphField, error) {
	versions := make([]uint64, len(s.sources))
	var fields []sdk.GraphField
	for i, source := range s.sources {
		version, held, err := source.FieldsSnapshot(ctx)
		if err != nil {
			return nil, nil, err
		}
		versions[i] = version
		fields = append(fields, held...)
	}
	return versions, fields, nil
}

// versionsEqual reports whether two version vectors match.
func versionsEqual(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
	if len(byEntity) == 0 {
		return base
	}
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

// Schema reports the widened schema every request validates and introspects against.
func (s *Schema) Schema() *ast.Schema {
	return s.live.Load().schema
}

// Complexity prices a field.
func (s *Schema) Complexity(
	ctx context.Context, typeName, field string, childComplexity int, args map[string]any,
) (int, bool) {
	return s.live.Load().inner.Complexity(ctx, typeName, field, childComplexity, args)
}

// Exec runs the generated executor over the rewritten operation.
func (s *Schema) Exec(ctx context.Context) graphql.ResponseHandler {
	s.refresh(ctx)
	live := s.live.Load()
	if len(live.byEntity) > 0 {
		opCtx := graphql.GetOperationContext(ctx)
		s.rewrite(live, opCtx.Operation.SelectionSet)
		for _, fragment := range opCtx.Doc.Fragments {
			s.rewrite(live, fragment.SelectionSet)
		}
	}
	return live.inner.Exec(ctx)
}

// rewrite points every runtime defined field in a selection set at the carrier.
func (s *Schema) rewrite(live *snapshot, selections ast.SelectionSet) {
	for _, selection := range selections {
		switch node := selection.(type) {
		case *ast.Field:
			if !s.rewriteField(live, node) {
				s.rewrite(live, node.SelectionSet)
			}
		case *ast.InlineFragment:
			s.rewrite(live, node.SelectionSet)
		case *ast.FragmentSpread:
		}
	}
}

// rewriteField points one field at the carrier, reporting whether it did.
func (s *Schema) rewriteField(live *snapshot, field *ast.Field) bool {
	if field.ObjectDefinition == nil || strings.HasPrefix(field.Name, "__") {
		return false
	}
	entity := live.byEntity[field.ObjectDefinition.Name]
	if _, defined := entity[field.Name]; !defined {
		return false
	}
	carrier := s.base.Types[field.ObjectDefinition.Name].Fields.ForName(carrierField)
	field.Arguments = ast.ArgumentList{{
		Name:  carrierArg,
		Value: &ast.Value{Raw: field.Name, Kind: ast.StringValue},
	}}
	field.Name = carrierField
	field.Definition = carrier
	return true
}
