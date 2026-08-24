// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"testing"

	"github.com/vektah/gqlparser/v2/ast"
)

func TestOnlyLoginFieldsRejectsAnEmptySelection(t *testing.T) {
	t.Parallel()

	if onlyLoginFields(nil) {
		t.Error("onlyLoginFields(nil) = true, want false")
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

func TestOnlyLocaleFieldsRejectsAFragmentSelection(t *testing.T) {
	t.Parallel()

	if onlyLocaleFields(ast.SelectionSet{&ast.FragmentSpread{Name: "sneaky"}}) {
		t.Error("onlyLocaleFields(fragment) = true, want only the plain field admitted")
	}
}

func TestOnlyLocaleFieldsRejectsAnEmptySelection(t *testing.T) {
	t.Parallel()

	if onlyLocaleFields(nil) {
		t.Error("onlyLocaleFields(nil) = true, want false")
	}
}
