// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"reflect"

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
	arguments, _ := json.Marshal(resolvedArguments(field, variables))
	return qualifiedName(field) + string(arguments)
}

// resolvedArguments returns the arguments field resolves to, each read by its declared type, a null one left out.
func resolvedArguments(field *ast.Field, variables map[string]any) map[string]any {
	given := field.ArgumentMap(variables)
	resolved := make(map[string]any, len(given))
	for _, argument := range field.Definition.Arguments {
		if value := given[argument.Name]; value != nil {
			resolved[argument.Name] = resolvedValue(argument.Type, value)
		}
	}
	return resolved
}

// resolvedValue returns value read as typ, an upload as its key.
func resolvedValue(typ *ast.Type, value any) any {
	if upload, isUpload := value.(graphql.Upload); isUpload {
		return uploadKey(upload)
	}
	if value == nil || typ.Elem == nil {
		return value
	}
	return resolvedList(typ.Elem, value)
}

// uploadKey keys an upload by its name, size, type and bytes, or by the address of its reader when it cannot be reread.
func uploadKey(upload graphql.Upload) string {
	digest := sha256.New()
	_, readErr := io.Copy(digest, upload.File)
	_, seekErr := upload.File.Seek(0, io.SeekStart)
	if readErr != nil || seekErr != nil {
		return fmt.Sprintf("%p", upload.File)
	}
	return fmt.Sprintf("%q %d %q %x", upload.Filename, upload.Size, upload.ContentType, digest.Sum(nil))
}

// resolvedList returns value read as a list of elem, a single value as a list of one.
func resolvedList(elem *ast.Type, value any) []any {
	list := reflect.ValueOf(value)
	if list.Kind() != reflect.Slice {
		return []any{resolvedValue(elem, value)}
	}
	items := make([]any, list.Len())
	for index := range items {
		items[index] = resolvedValue(elem, list.Index(index).Interface())
	}
	return items
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
