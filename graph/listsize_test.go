// SPDX-License-Identifier: Elastic-2.0

package graph_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

// listSizeDirective names the directive a plugin list prices itself with.
const listSizeDirective = "listSize"

// listSizeProblemsIn returns how every field of one SDL source falls short of pricing its list.
func listSizeProblemsIn(t *testing.T, name, source string) []string {
	t.Helper()
	doc, err := parser.ParseSchema(&ast.Source{Name: name, Input: source})
	if err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	var problems []string
	for _, def := range append(doc.Definitions, doc.Extensions...) {
		for _, field := range def.Fields {
			problems = append(problems, listSizeProblems(def.Name, field)...)
		}
	}
	return problems
}

// listSizeProblems reports how one field falls short of pricing its list.
func listSizeProblems(owner string, field *ast.FieldDefinition) []string {
	sized := field.Directives.ForName(listSizeDirective)
	if sized == nil {
		if field.Type.Elem != nil && field.Arguments.ForName("limit") != nil {
			return []string{fmt.Sprintf("%s.%s takes a limit without @%s", owner, field.Name, listSizeDirective)}
		}
		return nil
	}
	return slicingProblems(owner, field, sized)
}

// slicingProblems reports a @listSize that names no argument list or an argument the field does not take.
func slicingProblems(owner string, field *ast.FieldDefinition, sized *ast.Directive) []string {
	slicing := slicingList(sized)
	if len(slicing) == 0 {
		return []string{fmt.Sprintf("%s.%s names no list of slicing arguments", owner, field.Name)}
	}
	var problems []string
	sizedByLimit := false
	for _, named := range slicing {
		sizedByLimit = sizedByLimit || named.Value.Raw == "limit"
		if field.Arguments.ForName(named.Value.Raw) == nil {
			problems = append(problems,
				fmt.Sprintf("%s.%s is sized by %q, which it does not take", owner, field.Name, named.Value.Raw))
		}
	}
	if !sizedByLimit && field.Arguments.ForName("limit") != nil {
		problems = append(problems,
			fmt.Sprintf("%s.%s takes a limit its @%s does not size it by", owner, field.Name, listSizeDirective))
	}
	return problems
}

// slicingList returns the values a @listSize names as its slicing arguments, none unless they form a list.
func slicingList(sized *ast.Directive) ast.ChildValueList {
	slicing := sized.Arguments.ForName("slicingArguments")
	if slicing == nil || slicing.Value.Kind != ast.ListValue {
		return nil
	}
	return slicing.Value.Children
}

// checkListSizesIn reports every list pricing problem of the SDL files under one plugin graph dir, counting the files.
func checkListSizesIn(t *testing.T, dir string) int {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.graphqls"))
	if err != nil {
		t.Fatalf("globbing %s: %v", dir, err)
	}
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		for _, problem := range listSizeProblemsIn(t, path, string(raw)) {
			t.Errorf("%s: %s", path, problem)
		}
	}
	return len(files)
}

func TestEveryPluginListTakingALimitPricesItself(t *testing.T) {
	t.Parallel()

	checked := 0
	for _, dir := range pluginGraphDirs(t) {
		checked += checkListSizesIn(t, dir)
	}

	if checked == 0 {
		t.Fatal("checked 0 plugin schema files, the globs are broken")
	}
}

func TestListSizeCheckingFlagsEveryShortfall(t *testing.T) {
	t.Parallel()

	synthetic := `
type Query { unpriced(limit: Int): [Thing!]! }
extend type Query { typo(limit: Int): [Thing!]! @listSize(slicingArguments: ["limt"]) }
type Thing { single(limit: Int): [Thing!]! @listSize(slicingArguments: "limit") }
type Other { empty(limit: Int): [Thing!]! @listSize(slicingArguments: []) }
type Bare { bare(limit: Int): [Thing!]! @listSize }
type Offsetted { wrong(limit: Int, offset: Int): [Thing!]! @listSize(slicingArguments: ["offset"]) }
`
	want := []string{
		"Query.unpriced takes a limit without @listSize",
		`Query.typo is sized by "limt", which it does not take`,
		`Query.typo takes a limit its @listSize does not size it by`,
		"Thing.single names no list of slicing arguments",
		"Other.empty names no list of slicing arguments",
		"Bare.bare names no list of slicing arguments",
		"Offsetted.wrong takes a limit its @listSize does not size it by",
	}

	got := listSizeProblemsIn(t, "synthetic", synthetic)

	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("problems = %q, want %q", got, want)
	}
}

func TestListSizeCheckingAcceptsAPricedListAndIgnoresTheRest(t *testing.T) {
	t.Parallel()

	synthetic := `
type Query { priced(limit: Int, offset: Int): [Thing!]! @listSize(slicingArguments: ["limit"]) }
type Thing { unbounded: [Thing!]! one(limit: Int): Thing scalar(limit: Int): String }
`

	if got := listSizeProblemsIn(t, "synthetic", synthetic); len(got) != 0 {
		t.Errorf("problems = %q, want none for a priced list and fields that are no list or take no limit", got)
	}
}
