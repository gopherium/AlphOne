// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"errors"
	"fmt"
	"regexp"
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
)

// kinds maps every declarable kind to the GraphQL scalar it answers with.
var kinds = map[kind]string{
	kindText:     "String",
	kindLongText: "String",
	kindNumber:   "Int",
	kindBoolean:  "Boolean",
	kindDate:     "Date",
	kindSelect:   "String",
}

// namePattern matches the camelCase names a definition accepts.
var namePattern = regexp.MustCompile(`^[a-z][a-zA-Z0-9]*$`)

// scalar reports the GraphQL scalar the kind answers with.
func (k kind) scalar() string {
	return kinds[k]
}

// Definition is one runtime defined field as the catalogue holds it.
type Definition struct {
	ID         uuid.UUID
	Name       string
	Label      string
	Kind       kind
	ArchivedAt *time.Time
	CreatedAt  time.Time
}

// newDefinition builds a validated catalogue entry.
func newDefinition(name, label, declared string, reserved map[string]bool) (Definition, error) {
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
	trimmed := strings.TrimSpace(label)
	if trimmed == "" {
		return Definition{}, errBlankLabel
	}
	if utf8.RuneCountInString(trimmed) > labelMax {
		return Definition{}, errLabelTooLong
	}
	return Definition{
		ID:        uuid.Must(uuid.NewV7()),
		Name:      name,
		Label:     trimmed,
		Kind:      held,
		CreatedAt: time.Now().UTC(),
	}, nil
}
