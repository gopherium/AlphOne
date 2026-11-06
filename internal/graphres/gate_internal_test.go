// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"maps"
	"testing"

	"github.com/vektah/gqlparser/v2/ast"
)

func TestSelectsOnlyRejectsAnEmptySelection(t *testing.T) {
	t.Parallel()

	for name, allowed := range map[string]map[string]bool{
		"queries":   anonymousQueryFields,
		"mutations": anonymousMutationFields,
	} {
		if selectsOnly(nil, allowed) {
			t.Errorf("selectsOnly(nil, %s) = true, want false", name)
		}
	}
}

func TestAnonymousOperationRefusesASubscription(t *testing.T) {
	t.Parallel()

	operation := &ast.OperationDefinition{
		Operation:    ast.Subscription,
		SelectionSet: ast.SelectionSet{&ast.Field{Name: "coreEvent"}},
	}

	if anonymousOperation(operation) {
		t.Error("anonymousOperation(subscription) = true, want streams behind login")
	}
}

func TestSelectsOnlyRejectsAFragmentSelection(t *testing.T) {
	t.Parallel()

	for name, selection := range map[string]ast.Selection{
		"a spread":          &ast.FragmentSpread{Name: "sneaky"},
		"an inline section": &ast.InlineFragment{TypeCondition: "Query"},
	} {
		if selectsOnly(ast.SelectionSet{selection}, anonymousQueryFields) {
			t.Errorf("selectsOnly(%s) = true, want only a plain field admitted", name)
		}
	}
}

func TestTheAnonymousSetsStaySplitByOperationKind(t *testing.T) {
	t.Parallel()

	query := &ast.OperationDefinition{
		Operation:    ast.Query,
		SelectionSet: ast.SelectionSet{&ast.Field{Name: "login"}},
	}
	mutation := &ast.OperationDefinition{
		Operation:    ast.Mutation,
		SelectionSet: ast.SelectionSet{&ast.Field{Name: "legalLinks"}},
	}

	if anonymousOperation(query) {
		t.Error("anonymousOperation(a query named login) = true, want the mutation set kept apart")
	}
	if anonymousOperation(mutation) {
		t.Error("anonymousOperation(a mutation named legalLinks) = true, want the query set kept apart")
	}
}

func TestTheAnonymousQueriesAreExactlyTheLocaleAndTheLegalLinks(t *testing.T) {
	t.Parallel()

	want := map[string]bool{"locale": true, "legalLinks": true}

	if !maps.Equal(anonymousQueryFields, want) {
		t.Errorf("anonymousQueryFields = %v, want %v", anonymousQueryFields, want)
	}
}
