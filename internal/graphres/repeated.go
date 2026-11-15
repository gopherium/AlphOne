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

	"github.com/gopherium/alphone/graph/scalar"
	"github.com/gopherium/alphone/sdk"
)

// RepeatedFields rejects an operation that asks one field with the same arguments under two response names.
type RepeatedFields struct {
	schema *ast.Schema
}

// ExtensionName names the rule among the handler extensions.
func (*RepeatedFields) ExtensionName() string {
	return "RepeatedFields"
}

// Validate keeps the schema the rule reads input objects by.
func (r *RepeatedFields) Validate(schema graphql.ExecutableSchema) error {
	r.schema = schema.Schema()
	return nil
}

// MutateOperationContext rejects the operation when one of its selection sets asks a field under two names.
func (r *RepeatedFields) MutateOperationContext(_ context.Context, rc *graphql.OperationContext) *gqlerror.Error {
	return askedOperation{operation: rc, schema: r.schema}.repeatedIn(rc.Operation.SelectionSet)
}

// canonicalReaders maps a scalar name to the function that reads a value of it to its canonical value.
var canonicalReaders = map[string]func(any) (any, error){
	"Int":    canonicalInt,
	"String": canonicalString,
	"UUID":   canonicalUUID,
}

// askedOperation is one operation read against the schema it is asked over.
type askedOperation struct {
	operation *graphql.OperationContext
	schema    *ast.Schema
}

// repeatedIn returns the rejection of the first field set or a set below it asks twice, nil when none is.
func (a askedOperation) repeatedIn(set ast.SelectionSet) *gqlerror.Error {
	fields := graphql.CollectFields(a.operation, set, nil)
	if rejected := a.repeatedAmong(fields); rejected != nil {
		return rejected
	}
	for _, field := range fields {
		if rejected := a.repeatedIn(field.Selections); rejected != nil {
			return rejected
		}
	}
	return nil
}

// repeatedAmong returns the rejection of the first collected field asked again under another name, nil when none is.
func (a askedOperation) repeatedAmong(fields []graphql.CollectedField) *gqlerror.Error {
	named := make(map[string]string, len(fields))
	for _, field := range fields {
		asked := a.askedAs(field.Field)
		if first, seen := named[asked]; seen {
			return repeatedField(field.Field, first)
		}
		named[asked] = field.Alias
	}
	return nil
}

// askedAs keys a field by its type condition, its name and its arguments once variables and defaults apply.
func (a askedOperation) askedAs(field *ast.Field) string {
	arguments, _ := json.Marshal(a.resolvedArguments(field))
	return qualifiedName(field) + string(arguments)
}

// resolvedArguments returns the arguments field resolves to, each read by its declared type, a null one left out.
func (a askedOperation) resolvedArguments(field *ast.Field) map[string]any {
	given := field.ArgumentMap(a.operation.Variables)
	resolved := make(map[string]any, len(given))
	for _, argument := range field.Definition.Arguments {
		if value := given[argument.Name]; value != nil {
			resolved[argument.Name] = a.resolvedValue(argument.Type, value)
		}
	}
	return resolved
}

// resolvedValue returns value read as typ, a list item by item and a named type by its kind.
func (a askedOperation) resolvedValue(typ *ast.Type, value any) any {
	if value == nil {
		return nil
	}
	if typ.Elem != nil {
		return a.resolvedList(typ.Elem, value)
	}
	return a.resolvedNamed(typ.NamedType, value)
}

// resolvedList returns value read as a list of elem, a single value as a list of one.
func (a askedOperation) resolvedList(elem *ast.Type, value any) []any {
	list := reflect.ValueOf(value)
	if list.Kind() != reflect.Slice {
		return []any{a.resolvedValue(elem, value)}
	}
	items := make([]any, list.Len())
	for index := range items {
		items[index] = a.resolvedValue(elem, list.Index(index).Interface())
	}
	return items
}

// resolvedNamed reads value as the named type, an upload by key, an input by its members, a listed scalar canonically.
func (a askedOperation) resolvedNamed(name string, value any) any {
	if upload, isUpload := value.(graphql.Upload); isUpload {
		return uploadKey(upload)
	}
	if definition := a.schema.Types[name]; definition.Kind == ast.InputObject {
		members, _ := value.(map[string]any)
		return a.resolvedObject(definition, members)
	}
	if read, listed := canonicalReaders[name]; listed {
		return canonicalOrWritten(read, value)
	}
	return value
}

// resolvedObject reads the declared members of an input by their types, leaving out a null one unless omittable.
func (a askedOperation) resolvedObject(definition *ast.Definition, members map[string]any) map[string]any {
	resolved := make(map[string]any, len(members))
	for _, member := range definition.Fields {
		value, given := members[member.Name]
		switch {
		case value != nil:
			resolved[member.Name] = a.resolvedValue(member.Type, value)
		case given && omittable(member):
			resolved[member.Name] = nil
		}
	}
	return resolved
}

// omittable reports whether an input member keeps a null apart from the member left out.
func omittable(member *ast.FieldDefinition) bool {
	directive := member.Directives.ForName("goField")
	if directive == nil {
		return false
	}
	marked := directive.Arguments.ForName("omittable")
	return marked != nil && marked.Value.Raw == "true"
}

// canonicalOrWritten returns value read to its canonical value, or value as written when it does not decode.
func canonicalOrWritten(read func(any) (any, error), value any) any {
	canonical, err := read(value)
	if err != nil {
		return value
	}
	return canonical
}

// canonicalInt returns the number an Int value decodes to.
func canonicalInt(value any) (any, error) {
	return graphql.UnmarshalInt(value)
}

// canonicalString returns the text a String value decodes to.
func canonicalString(value any) (any, error) {
	return graphql.UnmarshalString(value)
}

// canonicalUUID returns the ID a UUID value names in its canonical spelling.
func canonicalUUID(value any) (any, error) {
	id, err := scalar.UnmarshalUUID(value)
	return id.String(), err
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
