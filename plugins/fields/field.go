// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Definition errors.
var (
	errMalformedName = errors.New("fields: a name is camelCase, starting with a lowercase letter")
	errUnknownKind   = errors.New("fields: unknown kind")
	errBlankLabel    = errors.New("fields: a label carries text")
	errLabelTooLong  = fmt.Errorf("fields: a label runs to %d characters", labelMax)
	errReservedName  = errors.New("fields: the name is already a field of the type")

	errSubFieldsRequired   = errors.New("fields: a repeater holds at least one sub field")
	errSubFieldsUnexpected = errors.New("fields: only a repeater holds sub fields")
	errSubFieldNested      = errors.New("fields: a sub field cannot be a repeater")
	errSubFieldNameInvalid = errors.New("fields: a sub field name is camelCase, starting with a lowercase letter")
	errSubFieldNameTaken   = errors.New("fields: two sub fields of one repeater hold the same name")
)

// labelMax caps how long a human label runs.
const labelMax = 120

// kind names the shape of the values a field holds.
type kind string

// The kinds a definition may declare.
const (
	kindText     kind = "TEXT"
	kindLongText kind = "LONGTEXT"
	kindNumber   kind = "NUMBER"
	kindBoolean  kind = "BOOLEAN"
	kindDate     kind = "DATE"
	kindSelect   kind = "SELECT"
	kindRepeater kind = "REPEATER"
)

// kinds maps every declarable kind to the GraphQL scalar it answers with.
var kinds = map[kind]string{
	kindText:     "String",
	kindLongText: "String",
	kindNumber:   "Int",
	kindBoolean:  "Boolean",
	kindDate:     "Date",
	kindSelect:   "String",
	kindRepeater: "JSON",
}

// namePattern matches the camelCase names a definition accepts.
var namePattern = regexp.MustCompile(`^[a-z][a-zA-Z0-9]*$`)

// scalar reports the GraphQL scalar the kind answers with.
func (k kind) scalar() string {
	return kinds[k]
}

// SubField is one column of the rows a repeater holds.
type SubField struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Kind  kind   `json:"kind"`
}

// Definition is one runtime defined field as the catalogue holds it.
type Definition struct {
	ID         uuid.UUID
	Name       string
	Label      string
	Kind       kind
	SubFields  []SubField
	ArchivedAt *time.Time
	CreatedAt  time.Time
}

// newDefinition builds a validated catalogue entry holding the given sub fields.
func newDefinition(
	name, label, declared string, reserved map[string]bool, subFields ...SubField,
) (Definition, error) {
	if !namePattern.MatchString(name) {
		return Definition{}, errMalformedName
	}
	if reserved[name] {
		return Definition{}, errReservedName
	}
	held := kind(declared)
	if _, known := kinds[held]; !known {
		return Definition{}, errUnknownKind
	}
	trimmed, err := checkLabel(label)
	if err != nil {
		return Definition{}, err
	}
	checked, err := checkSubFields(held, subFields)
	if err != nil {
		return Definition{}, err
	}
	return Definition{
		ID:        uuid.Must(uuid.NewV7()),
		Name:      name,
		Label:     trimmed,
		Kind:      held,
		SubFields: checked,
		CreatedAt: time.Now().UTC(),
	}, nil
}

// checkLabel trims a label, refusing one left blank or running past the cap.
func checkLabel(label string) (string, error) {
	trimmed := strings.TrimSpace(label)
	if trimmed == "" {
		return "", errBlankLabel
	}
	if utf8.RuneCountInString(trimmed) > labelMax {
		return "", errLabelTooLong
	}
	return trimmed, nil
}

// checkSubFields validates the sub fields a definition of the given kind holds.
func checkSubFields(held kind, subFields []SubField) ([]SubField, error) {
	if held != kindRepeater {
		if len(subFields) > 0 {
			return nil, errSubFieldsUnexpected
		}
		return nil, nil
	}
	if len(subFields) == 0 {
		return nil, errSubFieldsRequired
	}
	checked := make([]SubField, 0, len(subFields))
	for _, column := range subFields {
		sub, err := checkSubField(column, checked)
		if err != nil {
			return nil, err
		}
		checked = append(checked, sub)
	}
	return checked, nil
}

// checkSubField validates one sub field against the siblings checked before it.
func checkSubField(column SubField, siblings []SubField) (SubField, error) {
	if !namePattern.MatchString(column.Name) {
		return SubField{}, errSubFieldNameInvalid
	}
	if slices.ContainsFunc(siblings, func(sibling SubField) bool { return sibling.Name == column.Name }) {
		return SubField{}, errSubFieldNameTaken
	}
	if column.Kind == kindRepeater {
		return SubField{}, errSubFieldNested
	}
	if _, known := kinds[column.Kind]; !known {
		return SubField{}, errUnknownKind
	}
	label, err := checkLabel(column.Label)
	if err != nil {
		return SubField{}, err
	}
	return SubField{Name: column.Name, Label: label, Kind: column.Kind}, nil
}
