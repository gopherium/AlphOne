// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// errRequiredFieldUnmapped reports assignments leaving a required field unclaimed.
var errRequiredFieldUnmapped = fmt.Errorf("no assignment claims the required field %q", fieldContactName)

// errMappingLocked reports a mapping change on an import past its ready state.
var errMappingLocked = errors.New("the import no longer accepts a mapping")

// assignment binds one column of an import to a field of the registry.
type assignment struct {
	Column int
	Field  fieldName
}

// buildMapping turns assignments into the stored mapping, refusing an unusable set.
func buildMapping(assignments []assignment, width int, known registry) (mapping, error) {
	assigned := make(mapping, len(assignments))
	claimed := make(map[fieldName]bool, len(assignments))
	for _, one := range assignments {
		if err := checkAssignment(one, width, assigned, claimed, known); err != nil {
			return nil, err
		}
		assigned[strconv.Itoa(one.Column)] = one.Field
		claimed[one.Field] = true
	}
	if !claimed[fieldContactName] {
		return nil, errRequiredFieldUnmapped
	}
	return assigned, nil
}

// checkEntry refuses a ready import whose mapping names a field no registry holds.
func checkEntry(stored importRow, known registry) error {
	if stored.State != stateReady {
		return nil
	}
	var vanished []string
	for _, field := range stored.Mapping {
		if !known.holds(field) {
			vanished = append(vanished, string(field))
		}
	}
	if len(vanished) == 0 {
		return nil
	}
	slices.Sort(vanished)
	return fmt.Errorf("the mapping names %s, which the registry no longer carries",
		strings.Join(vanished, ", "))
}

// withinColumns reports whether the column index names a column the import carries.
func withinColumns(column, width int) bool {
	return column >= 0 && column < width
}

// checkAssignment reports why one assignment cannot join the mapping being built.
func checkAssignment(
	one assignment, width int, assigned mapping, claimed map[fieldName]bool, known registry,
) error {
	switch {
	case !withinColumns(one.Column, width):
		return fmt.Errorf("the import carries no column %d", one.Column)
	case !known.holds(one.Field):
		return fmt.Errorf("the registry carries no field %q", one.Field)
	case assigned[strconv.Itoa(one.Column)] != "":
		return fmt.Errorf("two assignments claim column %d", one.Column)
	case claimed[one.Field]:
		return fmt.Errorf("two assignments claim the field %q", one.Field)
	}
	return nil
}
