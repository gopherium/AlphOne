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

// capabilityArgument names the scope argument naming the capability a field needs.
const capabilityArgument = "capability"

// fieldScope is the area, access and capability one root field needs.
type fieldScope struct {
	area       string
	write      bool
	admin      bool
	capability role.Capability
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
	if needed := declared.Arguments.ForName(capabilityArgument); needed != nil &&
		needed.Value.Kind != ast.NullValue {
		scope.capability = role.Capability(needed.Value.Raw)
	}
	return scope
}

// WritesBeyondAuth reports whether one root field changes anything besides the caller's own session.
func (m ScopeMap) WritesBeyondAuth(operation ast.Operation, field string) bool {
	held := m[scopeKey{operation, field}]
	return held.write && held.area != authArea
}

// AdminOnly reports whether one root field is reserved to the admin tier.
func (m ScopeMap) AdminOnly(operation ast.Operation, field string) bool {
	return m[scopeKey{operation, field}].admin
}

// Capability returns the capability one root field needs, none when it declares no gate.
func (m ScopeMap) Capability(operation ast.Operation, field string) role.Capability {
	held := m[scopeKey{operation, field}]
	if held.capability != "" {
		return held.capability
	}
	if held.admin {
		return role.ManageUsers
	}
	return ""
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

// deactivatedKey marks a context whose standing tenant is deactivated.
type deactivatedKey struct{}

// WithDeactivatedTenant returns ctx marked as serving a deactivated tenant.
func WithDeactivatedTenant(ctx context.Context) context.Context {
	return context.WithValue(ctx, deactivatedKey{}, true)
}

// TenantDeactivated reports whether the context serves a deactivated tenant.
func TenantDeactivated(ctx context.Context) bool {
	marked, ok := ctx.Value(deactivatedKey{}).(bool)
	return ok && marked
}

// ScopeGate returns the operation gate refusing every root field the caller's role and token do not reach.
func ScopeGate(scopes ScopeMap) graphql.OperationMiddleware {
	return func(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
		token, carried := credential.TokenOf(ctx)
		tier := role.Role(authkit.IdentityFromContext(ctx).Role)
		operation := graphql.GetOperationContext(ctx)
		if operation.Operation == nil {
			return scopeError("the operation")
		}
		kind := operation.Operation.Operation
		for _, selected := range graphql.CollectFields(operation, operation.Operation.SelectionSet, nil) {
			if refused := fieldRefusal(ctx, scopes, kind, selected.Name, token, carried, tier); refused != nil {
				return refused
			}
		}
		return next(ctx)
	}
}

// fieldRefusal returns the answer refusing one selected root field, nil when the caller reaches it.
func fieldRefusal(
	ctx context.Context, scopes ScopeMap, kind ast.Operation, name string,
	token credential.Token, carried bool, tier role.Role,
) graphql.ResponseHandler {
	if carried && !scopes.Allows(kind, name, token.Scopes) {
		return scopeError(scopes.Needed(kind, name))
	}
	if needed := scopes.Capability(kind, name); needed != "" && !role.Can(tier, needed) {
		return capabilityError(scopes.Needed(kind, name), needed)
	}
	if TenantDeactivated(ctx) && scopes.WritesBeyondAuth(kind, name) {
		return deactivatedError()
	}
	return nil
}

// deactivatedError answers one operation refusing a write from a deactivated tenant.
func deactivatedError() graphql.ResponseHandler {
	return graphql.OneShot(&graphql.Response{Errors: gqlerror.List{&gqlerror.Error{
		Message: "tenant deactivated",
		Extensions: map[string]any{
			"code":   "UNAUTHORIZED",
			"reason": "tenant_deactivated",
		},
	}}})
}

// scopeError answers one operation with the scope its token lacks.
func scopeError(needed string) graphql.ResponseHandler {
	return refusedWith("scope required: "+needed, needed)
}

// capabilityError answers one operation naming the scope and the capability the caller's role lacked.
func capabilityError(needed string, lacked role.Capability) graphql.ResponseHandler {
	return graphql.OneShot(&graphql.Response{Errors: gqlerror.List{&gqlerror.Error{
		Message: "admin required",
		Extensions: map[string]any{
			"code":       "UNAUTHORIZED",
			"scope":      needed,
			"capability": string(lacked),
			"reason":     "capability_missing",
			"meta":       map[string]any{"scope": needed, "capability": string(lacked)},
		},
	}}})
}

// refusedWith answers one operation with the message and the scope the refused field wanted.
func refusedWith(message, needed string) graphql.ResponseHandler {
	return graphql.OneShot(&graphql.Response{Errors: gqlerror.List{&gqlerror.Error{
		Message: message,
		Extensions: map[string]any{
			"code":   "UNAUTHORIZED",
			"scope":  needed,
			"reason": "scope_missing",
			"meta":   map[string]any{"scope": needed},
		},
	}}})
}

// isIntrospection reports whether the field is a meta field no schema declares.
func isIntrospection(field string) bool {
	return len(field) >= len(introspectionPrefix) && field[:len(introspectionPrefix)] == introspectionPrefix
}
