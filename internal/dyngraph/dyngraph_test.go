// SPDX-License-Identifier: Elastic-2.0

package dyngraph_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/google/uuid"
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
}
type Task { title: String! }`

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

// stubBuild returns a Build serving sdl, counting every build it makes.
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

// fakeSource serves each tenant a settable stamp and settable fields.
type fakeSource struct {
	mu      sync.Mutex
	stamps  map[uuid.UUID]uint64
	fields  map[uuid.UUID][]sdk.GraphField
	err     error
	entered chan struct{}
	release chan struct{}
}

// FieldsSnapshot reports the calling tenant's settable stamp and fields, held until released when gated.
func (f *fakeSource) FieldsSnapshot(ctx context.Context) (uint64, []sdk.GraphField, error) {
	f.mu.Lock()
	tenant := sdk.TenantOrDefault(ctx)
	stamp, fields, err := f.stamps[tenant], f.fields[tenant], f.err
	f.mu.Unlock()
	if f.entered != nil {
		f.entered <- struct{}{}
		<-f.release
	}
	return stamp, fields, err
}

// set gives a tenant a stamp and the fields it serves.
func (f *fakeSource) set(tenant uuid.UUID, stamp uint64, fields ...sdk.GraphField) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stamps[tenant] = stamp
	f.fields[tenant] = fields
}

// sourceOf returns a source serving the default tenant the given fields at stamp one.
func sourceOf(fields ...sdk.GraphField) *fakeSource {
	source := &fakeSource{stamps: map[uuid.UUID]uint64{}, fields: map[uuid.UUID][]sdk.GraphField{}}
	source.set(sdk.DefaultTenantID, 1, fields...)
	return source
}

// birthDate is the field most tests define.
var birthDate = sdk.GraphField{Entity: "Contact", Name: "birthDate", Type: "JSON"}

// shoeSize is the field another tenant defines.
var shoeSize = sdk.GraphField{Entity: "Contact", Name: "shoeSize", Type: "JSON"}

// served returns the executable schema itself, the way a test inspects what a tenant is served.
func served(schema graphql.ExecutableSchema) graphql.ExecutableSchema { return schema }

// graphsOf returns graphs serving build widened by the sources, holding up to held tenants.
func graphsOf(build dyngraph.Build, held int, sources ...sdk.FieldSource) *dyngraph.Graphs[graphql.ExecutableSchema] {
	return dyngraph.New(build, served, held, sources...)
}

// inTenantOf returns a context serving a fresh tenant and that tenant.
func inTenantOf(t *testing.T) (context.Context, uuid.UUID) {
	t.Helper()
	tenant := uuid.Must(uuid.NewV7())
	return sdk.WithTenant(t.Context(), tenant), tenant
}

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

func TestServesTheCompiledSchemaWithoutSources(t *testing.T) {
	t.Parallel()

	build, compiled := stubBuild(t, carriedSDL, nil, nil)

	graphs := graphsOf(build, 0)

	if graphs.For(t.Context()).Schema() != compiled || graphs.Plain().Schema() != compiled {
		t.Error("the served schema is a copy, want the compiled schema untouched")
	}
}

func TestServesTheCompiledSchemaAsThePlainGraph(t *testing.T) {
	t.Parallel()

	build, compiled := stubBuild(t, carriedSDL, nil, nil)

	graphs := graphsOf(build, 0, sourceOf(birthDate))

	if graphs.Plain().Schema() != compiled {
		t.Error("Plain() serves a widened schema, want the compiled one")
	}
}

func TestWidensADefinedField(t *testing.T) {
	t.Parallel()

	build, _ := stubBuild(t, carriedSDL, nil, nil)

	graphs := graphsOf(build, 0, sourceOf(birthDate))

	widened := graphs.For(t.Context()).Schema().Types["Contact"].Fields.ForName("birthDate")
	if widened == nil {
		t.Fatal("birthDate missing, want it declared on Contact")
	}
	if widened.Type.Name() != "JSON" {
		t.Errorf("type = %q, want JSON", widened.Type.Name())
	}
}

func TestServesEachTenantItsOwnFields(t *testing.T) {
	t.Parallel()

	build, _ := stubBuild(t, carriedSDL, nil, nil)
	source := sourceOf(birthDate)
	inAcme, acme := inTenantOf(t)
	source.set(acme, 1, shoeSize)
	graphs := graphsOf(build, 0, source)

	owned := graphs.For(t.Context()).Schema().Types["Contact"].Fields
	acmes := graphs.For(inAcme).Schema().Types["Contact"].Fields

	if owned.ForName("birthDate") == nil || owned.ForName("shoeSize") != nil {
		t.Errorf("the default tenant's Contact = %v, want birthDate and no shoeSize", owned)
	}
	if acmes.ForName("shoeSize") == nil || acmes.ForName("birthDate") != nil {
		t.Errorf("Acme's Contact = %v, want shoeSize and no birthDate", acmes)
	}
}

func TestServesThePlainGraphToATenantWithNoFields(t *testing.T) {
	t.Parallel()

	build, _ := stubBuild(t, carriedSDL, nil, nil)
	inAcme, _ := inTenantOf(t)
	graphs := graphsOf(build, 0, sourceOf(birthDate))

	if graphs.For(inAcme) != graphs.Plain() {
		t.Error("a tenant with no fields got a graph of its own, want the plain one shared")
	}
}

func TestSkipsWideningWithoutACarrier(t *testing.T) {
	t.Parallel()

	build, _ := stubBuild(t, bareSDL, nil, nil)

	graphs := graphsOf(build, 0, sourceOf(birthDate))

	if graphs.For(t.Context()).Schema().Types["Contact"].Fields.ForName("birthDate") != nil {
		t.Error("birthDate declared, want no widening when the carrier is absent")
	}
}

func TestNeverShadowsACompiledField(t *testing.T) {
	t.Parallel()

	shadow := sdk.GraphField{Entity: "Contact", Name: "name", Type: "JSON"}
	build, _ := stubBuild(t, carriedSDL, nil, nil)

	graphs := graphsOf(build, 0, sourceOf(shadow))

	if got := graphs.For(t.Context()).Schema().Types["Contact"].Fields.ForName("name").Type.Name(); got != "String" {
		t.Errorf("name answers %q, want the compiled String untouched", got)
	}
}

func TestRewritesOntoTheCarrier(t *testing.T) {
	t.Parallel()

	var seen *ast.OperationDefinition
	build, _ := stubBuild(t, carriedSDL, &seen, nil)
	graph := graphsOf(build, 0, sourceOf(birthDate)).For(t.Context())
	ctx := operationFor(t, graph.Schema(), `{ contact { name birthDate bd: birthDate } }`)

	graph.Exec(ctx)

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
	graph := graphsOf(build, 0, sourceOf(birthDate)).For(t.Context())
	source := `query { contact { ...person } } fragment person on Contact { birthDate }`
	doc, err := gqlparser.LoadQueryWithRules(graph.Schema(), source, rules.NewDefaultRules())
	if err != nil {
		t.Fatalf("parsing the fragment query: %v", err)
	}
	opCtx := &graphql.OperationContext{Operation: doc.Operations[0], Doc: doc}

	graph.Exec(graphql.WithOperationContext(t.Context(), opCtx))

	rewritten := doc.Fragments[0].SelectionSet[0].(*ast.Field)
	if rewritten.Name != "field" || rewritten.Alias != "birthDate" {
		t.Errorf("fragment selection = %q as %q, want the carrier under the alias", rewritten.Name, rewritten.Alias)
	}
}

func TestBuildsATenantsGraphAgainOnlyWhenItsStampMoves(t *testing.T) {
	t.Parallel()

	calls := 0
	source := sourceOf(birthDate)
	build, _ := stubBuild(t, carriedSDL, nil, &calls)
	graphs := graphsOf(build, 0, source)
	graphs.For(t.Context())
	built := calls

	graphs.For(t.Context())
	if calls != built {
		t.Errorf("builds = %d, want %d with the stamp unchanged", calls, built)
	}
	extra := sdk.GraphField{Entity: "Contact", Name: "loyaltyPoints", Type: "JSON"}
	source.set(sdk.DefaultTenantID, 2, birthDate, extra)
	schema := graphs.For(t.Context()).Schema()

	if calls != built+1 {
		t.Errorf("builds = %d, want %d after the stamp moved", calls, built+1)
	}
	if schema.Types["Contact"].Fields.ForName("loyaltyPoints") == nil {
		t.Error("loyaltyPoints missing, want the moved catalogue served")
	}
}

func TestBuildsOneGraphForCallersMissingATenantTogether(t *testing.T) {
	t.Parallel()

	var builds atomic.Int32
	stub, _ := stubBuild(t, carriedSDL, nil, nil)
	counted := func(widened *ast.Schema) graphql.ExecutableSchema {
		builds.Add(1)
		return stub(widened)
	}
	source := sourceOf(birthDate)
	source.entered = make(chan struct{})
	source.release = make(chan struct{})
	graphs := graphsOf(counted, 0, source)
	answers := make(chan graphql.ExecutableSchema, 3)
	for range 3 {
		go func() { answers <- graphs.For(t.Context()) }()
	}
	for range 3 {
		<-source.entered
	}

	close(source.release)

	first := <-answers
	for range 2 {
		if <-answers != first {
			t.Error("callers missing the tenant together got different graphs, want the one built for them all")
		}
	}
	if got := builds.Load(); got != 2 {
		t.Errorf("builds = %d, want the compiled schema and one widened graph", got)
	}
}

func TestBuildsATenantsGraphWhileAnotherTenantsBuildRuns(t *testing.T) {
	t.Parallel()

	stub, _ := stubBuild(t, carriedSDL, nil, nil)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	gated := func(widened *ast.Schema) graphql.ExecutableSchema {
		if widened != nil && widened.Types["Contact"].Fields.ForName("birthDate") != nil {
			entered <- struct{}{}
			<-release
		}
		return stub(widened)
	}
	source := sourceOf()
	inSlow, slow := inTenantOf(t)
	inQuick, quick := inTenantOf(t)
	source.set(slow, 1, birthDate)
	source.set(quick, 1, shoeSize)
	graphs := graphsOf(gated, 0, source)
	go graphs.For(inSlow)
	<-entered

	built := make(chan graphql.ExecutableSchema, 1)
	go func() { built <- graphs.For(inQuick) }()

	select {
	case graph := <-built:
		if graph.Schema().Types["Contact"].Fields.ForName("shoeSize") == nil {
			t.Error("shoeSize missing, want the quick tenant's own graph")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a tenant's graph waited on another tenant's build, want builds of different tenants apart")
	}
}

func TestLetsGoOfTheBuildLockOfATenantTheCacheLetsGo(t *testing.T) {
	t.Parallel()

	source := sourceOf(birthDate)
	inAcme, acme := inTenantOf(t)
	source.set(acme, 1, shoeSize)
	build, _ := stubBuild(t, carriedSDL, nil, nil)
	graphs := graphsOf(build, 1, source)
	graphs.For(t.Context())

	graphs.For(inAcme)

	if got := graphs.BuildLocks(); got != 1 {
		t.Errorf("build locks = %d, want only the held tenant's", got)
	}
}

func TestRewritesAnyFieldTheCompiledSchemaLacks(t *testing.T) {
	t.Parallel()

	var seen *ast.OperationDefinition
	source := sourceOf(birthDate)
	build, _ := stubBuild(t, carriedSDL, &seen, nil)
	graphs := graphsOf(build, 0, source)
	graph := graphs.For(t.Context())
	ctx := operationFor(t, graph.Schema(), `{ contact { birthDate } }`)
	source.set(sdk.DefaultTenantID, 2)
	graphs.For(t.Context())

	graph.Exec(ctx)

	rewritten := seen.SelectionSet[0].(*ast.Field).SelectionSet[0].(*ast.Field)
	if rewritten.Name != "field" {
		t.Errorf("selection = %q, want the carrier so the executor never meets an unknown field", rewritten.Name)
	}
}

func TestLeavesAFieldAloneOnATypeCarryingNoCarrier(t *testing.T) {
	t.Parallel()

	var seen *ast.OperationDefinition
	build, compiled := stubBuild(t, carriedSDL, &seen, nil)
	graph := graphsOf(build, 0, sourceOf(birthDate)).For(t.Context())
	selection := &ast.Field{Name: "estimatedHours", ObjectDefinition: compiled.Types["Task"]}
	operation := &ast.OperationDefinition{
		Operation:    ast.Query,
		SelectionSet: ast.SelectionSet{selection},
	}
	ctx := graphql.WithOperationContext(t.Context(),
		&graphql.OperationContext{Operation: operation, Doc: &ast.QueryDocument{}})

	graph.Exec(ctx)

	if selection.Name != "estimatedHours" {
		t.Errorf("selection = %q, want it untouched when the type carries no carrier", selection.Name)
	}
}

func TestLeavesAFieldAloneOnATypeTheCompiledSchemaLacks(t *testing.T) {
	t.Parallel()

	var seen *ast.OperationDefinition
	build, _ := stubBuild(t, carriedSDL, &seen, nil)
	graph := graphsOf(build, 0, sourceOf(birthDate)).For(t.Context())
	selection := &ast.Field{Name: "birthDate", ObjectDefinition: &ast.Definition{Name: "Unknown"}}
	operation := &ast.OperationDefinition{
		Operation:    ast.Query,
		SelectionSet: ast.SelectionSet{selection},
	}
	ctx := graphql.WithOperationContext(t.Context(),
		&graphql.OperationContext{Operation: operation, Doc: &ast.QueryDocument{}})

	graph.Exec(ctx)

	if selection.Name != "birthDate" {
		t.Errorf("selection = %q, want it untouched on a type the compiled schema lacks", selection.Name)
	}
}

func TestDelegatesComplexityToTheInnerExecutor(t *testing.T) {
	t.Parallel()

	build, _ := stubBuild(t, carriedSDL, nil, nil)
	graph := graphsOf(build, 0, sourceOf(birthDate)).For(t.Context())

	if _, priced := graph.Complexity(t.Context(), "Contact", "name", 1, nil); priced {
		t.Error("Complexity() priced a field the stub never prices, want the inner answer")
	}
}

func TestRewritesInsideInlineFragmentsAndSkipsIntrospection(t *testing.T) {
	t.Parallel()

	var seen *ast.OperationDefinition
	build, _ := stubBuild(t, carriedSDL, &seen, nil)
	graph := graphsOf(build, 0, sourceOf(birthDate)).For(t.Context())
	source := `{ contact { __typename ... on Contact { birthDate } } }`
	ctx := operationFor(t, graph.Schema(), source)

	graph.Exec(ctx)

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

func TestServesTheLastGraphOnASourceError(t *testing.T) {
	t.Parallel()

	source := sourceOf(birthDate)
	build, _ := stubBuild(t, carriedSDL, nil, nil)
	graphs := graphsOf(build, 0, source)
	graphs.For(t.Context())

	source.mu.Lock()
	source.err = errors.New("catalogue unavailable")
	source.mu.Unlock()
	source.set(sdk.DefaultTenantID, 9)

	if graphs.For(t.Context()).Schema().Types["Contact"].Fields.ForName("birthDate") == nil {
		t.Error("birthDate missing, want the last good graph served on an error")
	}
}

func TestServesThePlainGraphOnASourceErrorBeforeAnyGraph(t *testing.T) {
	t.Parallel()

	source := sourceOf(birthDate)
	source.err = errors.New("catalogue unavailable")
	build, _ := stubBuild(t, carriedSDL, nil, nil)
	graphs := graphsOf(build, 0, source)

	if graphs.For(t.Context()) != graphs.Plain() {
		t.Error("a failed first read served a graph of its own, want the plain one")
	}
}

func TestLetsTheTenantServedLeastRecentlyGo(t *testing.T) {
	t.Parallel()

	source := sourceOf(birthDate)
	inAcme, acme := inTenantOf(t)
	source.set(acme, 1, shoeSize)
	build, _ := stubBuild(t, carriedSDL, nil, nil)
	graphs := graphsOf(build, 1, source)
	first := graphs.For(t.Context())
	graphs.For(inAcme)

	if graphs.For(t.Context()) == first {
		t.Error("the tenant kept its graph past the one held, want it let go and built again")
	}
}

// visitFreshTenants serves count fresh tenants a field each.
func visitFreshTenants(t *testing.T, graphs *dyngraph.Graphs[graphql.ExecutableSchema], source *fakeSource, count int) {
	t.Helper()
	for range count {
		ctx, tenant := inTenantOf(t)
		source.set(tenant, 1, shoeSize)
		graphs.For(ctx)
	}
}

func TestHoldsTheDefaultNumberOfTenantsWhenGivenNone(t *testing.T) {
	t.Parallel()

	for _, given := range []int{0, -1} {
		t.Run(fmt.Sprint(given), func(t *testing.T) {
			t.Parallel()

			source := sourceOf(birthDate)
			build, _ := stubBuild(t, carriedSDL, nil, nil)
			graphs := graphsOf(build, given, source)
			first := graphs.For(t.Context())
			visitFreshTenants(t, graphs, source, sdk.DefaultTenantsHeld-1)
			if graphs.For(t.Context()) != first {
				t.Fatal("the tenant lost its graph before the default number was passed, want it held")
			}

			visitFreshTenants(t, graphs, source, sdk.DefaultTenantsHeld)

			if graphs.For(t.Context()) == first {
				t.Error("the tenant kept its graph past the default number, want it let go and built again")
			}
		})
	}
}

func TestBuildsATenantsGraphAgainWhenAnySourcesStampMoves(t *testing.T) {
	t.Parallel()

	first := sourceOf(birthDate)
	second := sourceOf()
	build, _ := stubBuild(t, carriedSDL, nil, nil)
	graphs := graphsOf(build, 0, first, second)
	graphs.For(t.Context())

	second.set(sdk.DefaultTenantID, 2, shoeSize)
	schema := graphs.For(t.Context()).Schema()

	if schema.Types["Contact"].Fields.ForName("shoeSize") == nil {
		t.Error("shoeSize missing, want the graph built again when the second source's stamp moved")
	}
	if schema.Types["Contact"].Fields.ForName("birthDate") == nil {
		t.Error("birthDate missing, want the first source's field still served")
	}
}
