// SPDX-License-Identifier: Elastic-2.0

package dyngraph_test

import (
	"context"
	"errors"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	gqlparser "github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/validator/rules"

	"github.com/gopherium/alphone/internal/dyngraph"
	"github.com/gopherium/alphone/sdk"
)

// carriedSDL is a compiled schema shape holding the carrier field.
const carriedSDL = `
scalar JSON
type Query { contact: Contact }
type Contact {
	id: ID!
	name: String!
	field(name: String!): JSON
}`

// bareSDL is a compiled schema shape holding no carrier field.
const bareSDL = `
type Query { contact: Contact }
type Contact {
	id: ID!
	name: String!
}`

// stubInner is a generated executor stand in recording what reaches it.
type stubInner struct {
	schema *ast.Schema
	seen   **ast.OperationDefinition
}

// Schema reports the schema the stub was built over.
func (s *stubInner) Schema() *ast.Schema { return s.schema }

// Complexity prices nothing.
func (s *stubInner) Complexity(context.Context, string, string, int, map[string]any) (int, bool) {
	return 0, false
}

// Exec records the operation it was asked to run.
func (s *stubInner) Exec(ctx context.Context) graphql.ResponseHandler {
	if s.seen != nil {
		*s.seen = graphql.GetOperationContext(ctx).Operation
	}
	return func(context.Context) *graphql.Response { return &graphql.Response{} }
}

// stubBuild returns a Build serving sdl, remembering every widened schema it got.
func stubBuild(t *testing.T, sdl string, seen **ast.OperationDefinition, calls *int) (dyngraph.Build, *ast.Schema) {
	t.Helper()
	compiled := gqlparser.MustLoadSchema(&ast.Source{Name: "test.graphqls", Input: sdl})
	return func(widened *ast.Schema) graphql.ExecutableSchema {
		if calls != nil {
			*calls++
		}
		served := widened
		if served == nil {
			served = compiled
		}
		return &stubInner{schema: served, seen: seen}
	}, compiled
}

// fakeSource serves a settable snapshot.
type fakeSource struct {
	version uint64
	fields  []sdk.GraphField
	err     error
}

// FieldsSnapshot reports the settable snapshot.
func (f *fakeSource) FieldsSnapshot(context.Context) (uint64, []sdk.GraphField, error) {
	return f.version, f.fields, f.err
}

// birthDate is the field every test defines.
var birthDate = sdk.GraphField{Entity: "Contact", Name: "birthDate", Type: "JSON"}

// operationFor parses source against schema and wraps it in an operation context.
func operationFor(t *testing.T, schema *ast.Schema, source string) context.Context {
	t.Helper()
	doc, err := gqlparser.LoadQueryWithRules(schema, source, rules.NewDefaultRules())
	if err != nil {
		t.Fatalf("parsing %q: %v", source, err)
	}
	opCtx := &graphql.OperationContext{Operation: doc.Operations[0], Doc: doc}
	return graphql.WithOperationContext(t.Context(), opCtx)
}

func TestPassesThroughWithoutSources(t *testing.T) {
	t.Parallel()

	build, compiled := stubBuild(t, carriedSDL, nil, nil)

	dyn := dyngraph.New(build)

	if dyn.Schema() != compiled {
		t.Error("Schema() = a copy, want the compiled schema untouched")
	}
}

func TestWidensADefinedField(t *testing.T) {
	t.Parallel()

	build, _ := stubBuild(t, carriedSDL, nil, nil)

	dyn := dyngraph.New(build, &fakeSource{version: 1, fields: []sdk.GraphField{birthDate}})

	widened := dyn.Schema().Types["Contact"].Fields.ForName("birthDate")
	if widened == nil {
		t.Fatal("birthDate missing, want it declared on Contact")
	}
	if widened.Type.Name() != "JSON" {
		t.Errorf("type = %q, want JSON", widened.Type.Name())
	}
}

func TestSkipsWideningWithoutACarrier(t *testing.T) {
	t.Parallel()

	build, _ := stubBuild(t, bareSDL, nil, nil)

	dyn := dyngraph.New(build, &fakeSource{version: 1, fields: []sdk.GraphField{birthDate}})

	if dyn.Schema().Types["Contact"].Fields.ForName("birthDate") != nil {
		t.Error("birthDate declared, want no widening when the carrier is absent")
	}
}

func TestNeverShadowsACompiledField(t *testing.T) {
	t.Parallel()

	shadow := sdk.GraphField{Entity: "Contact", Name: "name", Type: "JSON"}
	build, _ := stubBuild(t, carriedSDL, nil, nil)

	dyn := dyngraph.New(build, &fakeSource{version: 1, fields: []sdk.GraphField{shadow}})

	if got := dyn.Schema().Types["Contact"].Fields.ForName("name").Type.Name(); got != "String" {
		t.Errorf("name answers %q, want the compiled String untouched", got)
	}
}

func TestRewritesOntoTheCarrier(t *testing.T) {
	t.Parallel()

	var seen *ast.OperationDefinition
	build, _ := stubBuild(t, carriedSDL, &seen, nil)
	dyn := dyngraph.New(build, &fakeSource{version: 1, fields: []sdk.GraphField{birthDate}})
	ctx := operationFor(t, dyn.Schema(), `{ contact { name birthDate bd: birthDate } }`)

	dyn.Exec(ctx)

	fields := seen.SelectionSet[0].(*ast.Field).SelectionSet
	name, plain, aliased := fields[0].(*ast.Field), fields[1].(*ast.Field), fields[2].(*ast.Field)
	if name.Name != "name" || name.Alias != "name" {
		t.Errorf("name = %q as %q, want it untouched", name.Name, name.Alias)
	}
	if plain.Name != "field" || plain.Alias != "birthDate" {
		t.Errorf("selection = %q as %q, want the carrier under the birthDate alias", plain.Name, plain.Alias)
	}
	if got := plain.Arguments.ForName("name").Value.Raw; got != "birthDate" {
		t.Errorf("carrier argument = %q, want birthDate", got)
	}
	if aliased.Name != "field" || aliased.Alias != "bd" {
		t.Errorf("selection = %q as %q, want the caller's alias kept", aliased.Name, aliased.Alias)
	}
}

func TestRewritesThroughFragments(t *testing.T) {
	t.Parallel()

	var seen *ast.OperationDefinition
	build, _ := stubBuild(t, carriedSDL, &seen, nil)
	dyn := dyngraph.New(build, &fakeSource{version: 1, fields: []sdk.GraphField{birthDate}})
	source := `query { contact { ...person } } fragment person on Contact { birthDate }`
	doc, err := gqlparser.LoadQueryWithRules(dyn.Schema(), source, rules.NewDefaultRules())
	if err != nil {
		t.Fatalf("parsing the fragment query: %v", err)
	}
	opCtx := &graphql.OperationContext{Operation: doc.Operations[0], Doc: doc}

	dyn.Exec(graphql.WithOperationContext(t.Context(), opCtx))

	rewritten := doc.Fragments[0].SelectionSet[0].(*ast.Field)
	if rewritten.Name != "field" || rewritten.Alias != "birthDate" {
		t.Errorf("fragment selection = %q as %q, want the carrier under the alias", rewritten.Name, rewritten.Alias)
	}
}

func TestReloadsOnlyOnAVersionBump(t *testing.T) {
	t.Parallel()

	calls := 0
	source := &fakeSource{version: 1, fields: []sdk.GraphField{birthDate}}
	build, _ := stubBuild(t, carriedSDL, nil, &calls)
	dyn := dyngraph.New(build, source)
	built := calls

	dyn.Exec(operationFor(t, dyn.Schema(), `{ contact { name } }`))
	dyn.Exec(operationFor(t, dyn.Schema(), `{ contact { name } }`))
	if calls != built {
		t.Errorf("builds = %d, want %d with the version unchanged", calls, built)
	}

	extra := sdk.GraphField{Entity: "Contact", Name: "loyaltyPoints", Type: "JSON"}
	source.version, source.fields = 2, []sdk.GraphField{birthDate, extra}
	dyn.Exec(operationFor(t, dyn.Schema(), `{ contact { name } }`))
	if calls != built+1 {
		t.Errorf("builds = %d, want %d after the bump", calls, built+1)
	}
	if dyn.Schema().Types["Contact"].Fields.ForName("loyaltyPoints") == nil {
		t.Error("loyaltyPoints missing, want the bumped catalogue served")
	}
}

func TestServesASourceStartingAtVersionZero(t *testing.T) {
	t.Parallel()

	build, _ := stubBuild(t, carriedSDL, nil, nil)

	dyn := dyngraph.New(build, &fakeSource{version: 0, fields: []sdk.GraphField{birthDate}})

	if dyn.Schema().Types["Contact"].Fields.ForName("birthDate") == nil {
		t.Error("birthDate missing, want a zero version catalogue served too")
	}
}

func TestDelegatesComplexityToTheInnerExecutor(t *testing.T) {
	t.Parallel()

	build, _ := stubBuild(t, carriedSDL, nil, nil)
	dyn := dyngraph.New(build, &fakeSource{version: 1, fields: []sdk.GraphField{birthDate}})

	if _, priced := dyn.Complexity(t.Context(), "Contact", "name", 1, nil); priced {
		t.Error("Complexity() priced a field the stub never prices, want the inner answer")
	}
}

func TestRewritesInsideInlineFragmentsAndSkipsIntrospection(t *testing.T) {
	t.Parallel()

	var seen *ast.OperationDefinition
	build, _ := stubBuild(t, carriedSDL, &seen, nil)
	dyn := dyngraph.New(build, &fakeSource{version: 1, fields: []sdk.GraphField{birthDate}})
	source := `{ contact { __typename ... on Contact { birthDate } } }`
	ctx := operationFor(t, dyn.Schema(), source)

	dyn.Exec(ctx)

	contact := seen.SelectionSet[0].(*ast.Field)
	typename := contact.SelectionSet[0].(*ast.Field)
	if typename.Name != "__typename" {
		t.Errorf("selection = %q, want __typename untouched", typename.Name)
	}
	inlined := contact.SelectionSet[1].(*ast.InlineFragment).SelectionSet[0].(*ast.Field)
	if inlined.Name != "field" || inlined.Alias != "birthDate" {
		t.Errorf("inline selection = %q as %q, want the carrier under the alias", inlined.Name, inlined.Alias)
	}
}

func TestServesTheLastSnapshotOnASourceError(t *testing.T) {
	t.Parallel()

	source := &fakeSource{version: 1, fields: []sdk.GraphField{birthDate}}
	build, _ := stubBuild(t, carriedSDL, nil, nil)
	dyn := dyngraph.New(build, source)

	source.err = errors.New("catalogue unavailable")
	source.version = 9
	dyn.Exec(operationFor(t, dyn.Schema(), `{ contact { name } }`))

	if dyn.Schema().Types["Contact"].Fields.ForName("birthDate") == nil {
		t.Error("birthDate missing, want the last good snapshot served on an error")
	}
}
