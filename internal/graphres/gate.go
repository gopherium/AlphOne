// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/google/uuid"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/gopherium/gouncer/authkit"
)

// loginFieldName is the one root field an anonymous caller may select.
const loginFieldName = "login"

// AnonymousGate rejects anonymous operations reaching beyond login and the locale.
func AnonymousGate(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
	if authkit.IdentityFromContext(ctx).ID != uuid.Nil {
		return next(ctx)
	}
	operation := graphql.GetOperationContext(ctx).Operation
	if operation == nil || !anonymousOperation(operation) {
		return graphql.OneShot(&graphql.Response{Errors: gqlerror.List{unauthenticatedError()}})
	}
	return next(ctx)
}

// anonymousOperation reports whether every selection is one an anonymous caller may reach.
func anonymousOperation(operation *ast.OperationDefinition) bool {
	switch operation.Operation {
	case ast.Mutation:
		return onlyLoginFields(operation.SelectionSet)
	case ast.Query:
		return onlyLocaleFields(operation.SelectionSet)
	default:
		return false
	}
}

// localeFieldName is the one root query an anonymous caller may select.
const localeFieldName = "locale"

// onlyLocaleFields reports whether every root selection is the plain locale field.
func onlyLocaleFields(selections ast.SelectionSet) bool {
	if len(selections) == 0 {
		return false
	}
	for _, selection := range selections {
		field, ok := selection.(*ast.Field)
		if !ok || field.Name != localeFieldName {
			return false
		}
	}
	return true
}

// onlyLoginFields reports whether every root selection is the plain login field.
func onlyLoginFields(selections ast.SelectionSet) bool {
	if len(selections) == 0 {
		return false
	}
	for _, selection := range selections {
		field, ok := selection.(*ast.Field)
		if !ok || field.Name != loginFieldName {
			return false
		}
	}
	return true
}

// unauthenticatedError builds the gate's rejection error.
func unauthenticatedError() *gqlerror.Error {
	return &gqlerror.Error{
		Message:    "authentication required",
		Extensions: map[string]any{"code": "UNAUTHENTICATED", "reason": "authentication_required"},
	}
}
