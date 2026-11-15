// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/gopherium/alphone/sdk"
)

// RepeatedFields rejects an operation that asks one field with the same arguments under two response names.
type RepeatedFields struct{}

// ExtensionName names the rule among the handler extensions.
func (RepeatedFields) ExtensionName() string {
	return "RepeatedFields"
}

// Validate accepts any schema.
func (RepeatedFields) Validate(graphql.ExecutableSchema) error {
	return nil
}

// MutateOperationContext rejects the operation when one of its selection sets asks a field under two names.
func (RepeatedFields) MutateOperationContext(_ context.Context, rc *graphql.OperationContext) *gqlerror.Error {
	return repeatedIn(rc, rc.Operation.SelectionSet)
}

// repeatedIn returns the rejection of the first field set or a set below it asks twice, nil when none is.
func repeatedIn(rc *graphql.OperationContext, set ast.SelectionSet) *gqlerror.Error {
	fields := graphql.CollectFields(rc, set, nil)
	if rejected := repeatedAmong(fields, rc.Variables); rejected != nil {
		return rejected
	}
	for _, field := range fields {
		if rejected := repeatedIn(rc, field.Selections); rejected != nil {
			return rejected
		}
	}
	return nil
}

// repeatedAmong returns the rejection of the first collected field asked again under another name, nil when none is.
func repeatedAmong(fields []graphql.CollectedField, variables map[string]any) *gqlerror.Error {
	named := make(map[string]string, len(fields))
	for _, field := range fields {
		asked := askedAs(field.Field, variables)
		if first, seen := named[asked]; seen {
			return repeatedField(field.Field, first)
		}
		named[asked] = field.Alias
	}
	return nil
}

// askedAs keys a field by its type condition, its name and its arguments once variables and defaults apply.
func askedAs(field *ast.Field, variables map[string]any) string {
	arguments, _ := json.Marshal(field.ArgumentMap(variables))
	return qualifiedName(field) + string(arguments)
}

// qualifiedName names a field after the type it is asked on, as Type.field.
func qualifiedName(field *ast.Field) string {
	return field.ObjectDefinition.Name + "." + field.Name
}

// repeatedField returns the rejection of field asked under its alias after it was asked as first.
func repeatedField(field *ast.Field, first string) *gqlerror.Error {
	name := qualifiedName(field)
	return gqlerror.Wrap(sdk.GraphError{
		Code: "VALIDATION", Reason: "repeated_field", Meta: map[string]any{"field": name},
		Err: fmt.Errorf("graph: the field %s is asked under the names %s and %s", name, first, field.Alias),
	})
}
