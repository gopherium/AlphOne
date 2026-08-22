// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/credential"
	"github.com/gopherium/alphone/internal/role"
)

// scopeDirectiveName names the directive a root field declares its area with.
const scopeDirectiveName = "scope"

// authArea names the fields every authenticated caller reaches, whatever its token holds.
const authArea = "auth"

// tokensArea names the fields only a login session reaches.
const tokensArea = "tokens"

// introspectionPrefix marks the meta fields no schema declares.
const introspectionPrefix = "__"

// scopeKey addresses one root field of one operation.
type scopeKey struct {
	operation ast.Operation
	field     string
}

// adminArgument names the scope argument reserving a field to the admin tier.
const adminArgument = "admin"

// fieldScope is the area, access and tier one root field needs.
type fieldScope struct {
	area  string
	write bool
	admin bool
}

// ScopeMap answers what every root field of a schema needs of its caller.
type ScopeMap map[scopeKey]fieldScope

// NewScopeMap reads the scope every root field of schema declares.
func NewScopeMap(schema *ast.Schema) ScopeMap {
	scopes := ScopeMap{}
	roots := map[ast.Operation]*ast.Definition{
		ast.Query:        schema.Query,
		ast.Mutation:     schema.Mutation,
		ast.Subscription: schema.Subscription,
	}
	for operation, root := range roots {
		if root == nil {
			continue
		}
		for _, field := range root.Fields {
			if declared := field.Directives.ForName(scopeDirectiveName); declared != nil {
				scopes[scopeKey{operation, field.Name}] = scopeOf(declared)
			}
		}
	}
	return scopes
}

// scopeOf reads the area and access one scope directive declares.
func scopeOf(declared *ast.Directive) fieldScope {
	scope := fieldScope{}
	if area := declared.Arguments.ForName("area"); area != nil {
		scope.area = area.Value.Raw
	}
	if write := declared.Arguments.ForName("write"); write != nil {
		scope.write = write.Value.Raw == "true"
	}
	if adminOnly := declared.Arguments.ForName(adminArgument); adminOnly != nil {
		scope.admin = adminOnly.Value.Raw == "true"
	}
	return scope
}

// AdminOnly reports whether one root field is reserved to the admin tier.
func (m ScopeMap) AdminOnly(operation ast.Operation, field string) bool {
	return m[scopeKey{operation, field}].admin
}

// Allows reports whether held scopes reach one root field, refusing anything the schema does not scope.
func (m ScopeMap) Allows(operation ast.Operation, field string, held apitoken.Scopes) bool {
	if isIntrospection(field) {
		return true
	}
	needed, declared := m[scopeKey{operation, field}]
	if !declared {
		return false
	}
	switch needed.area {
	case authArea:
		return true
	case tokensArea:
		return false
	}
	return held.Allows(needed.area, needed.write)
}

// Needed names the scope one root field asks of its caller, or the field itself when unscoped.
func (m ScopeMap) Needed(operation ast.Operation, field string) string {
	needed, declared := m[scopeKey{operation, field}]
	if !declared {
		return field
	}
	access := "read"
	if needed.write {
		access = "write"
	}
	return needed.area + ":" + access
}

// ScopeGate returns the operation gate refusing every root field the caller's role and token do not reach.
func ScopeGate(scopes ScopeMap) graphql.OperationMiddleware {
	return func(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
		token, carried := credential.TokenOf(ctx)
		tier := role.Of(authkit.IdentityFromContext(ctx).Role)
		operation := graphql.GetOperationContext(ctx)
		if operation.Operation == nil {
			return scopeRefusal("the operation")
		}
		kind := operation.Operation.Operation
		for _, selected := range graphql.CollectFields(operation, operation.Operation.SelectionSet, nil) {
			if carried && !scopes.Allows(kind, selected.Name, token.Scopes) {
				return scopeRefusal(scopes.Needed(kind, selected.Name))
			}
			if !tier.Allows(scopes.AdminOnly(kind, selected.Name)) {
				return refusal("admin required", scopes.Needed(kind, selected.Name))
			}
		}
		return next(ctx)
	}
}

// scopeRefusal answers one operation with the scope its token lacks.
func scopeRefusal(needed string) graphql.ResponseHandler {
	return refusal("scope required: "+needed, needed)
}

// refusal answers one operation with the message and the scope the refused field wanted.
func refusal(message, needed string) graphql.ResponseHandler {
	return graphql.OneShot(&graphql.Response{Errors: gqlerror.List{&gqlerror.Error{
		Message:    message,
		Extensions: map[string]any{"code": "UNAUTHORIZED", "scope": needed},
	}}})
}

// isIntrospection reports whether the field is a meta field no schema declares.
func isIntrospection(field string) bool {
	return len(field) >= len(introspectionPrefix) && field[:len(introspectionPrefix)] == introspectionPrefix
}
