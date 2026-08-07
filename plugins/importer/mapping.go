// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
)

// errRequiredFieldUnmapped reports assignments leaving a required field unclaimed.
var errRequiredFieldUnmapped = fmt.Errorf("no assignment claims the required field %q", fieldContactName)

// errMappingLocked reports a mapping change on an import past its ready state.
var errMappingLocked = errors.New("the import no longer accepts a mapping")

type assignment struct {
	Column int       `json:"column"`
	Field  fieldName `json:"field"`
}

type mappingRequest struct {
	Assignments []assignment `json:"assignments"`
}

// handleMappingPut returns a handler storing the column assignments of an import.
func (p *Plugin) handleMappingPut() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req mappingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "the request body is malformed")
			return
		}
		stored, ok := p.loadImport(w, r)
		if !ok {
			return
		}
		assigned, ok := usableMapping(w, req.Assignments, stored)
		if !ok {
			return
		}
		if err := p.store.updateMapping(r.Context(), stored.ID, assigned); err != nil {
			respondError(w, http.StatusInternalServerError, "the mapping could not be stored")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// usableMapping returns the mapping assignments stand for, answering the caller itself when they do not.
func usableMapping(w http.ResponseWriter, assignments []assignment, stored importRow) (mapping, bool) {
	if stored.State != stateReady {
		respondError(w, http.StatusConflict, errMappingLocked.Error())
		return nil, false
	}
	assigned, err := buildMapping(assignments, len(stored.Columns))
	if err != nil {
		respondError(w, http.StatusUnprocessableEntity, err.Error())
		return nil, false
	}
	return assigned, true
}

// buildMapping turns assignments into the stored mapping, refusing an unusable set.
func buildMapping(assignments []assignment, width int) (mapping, error) {
	assigned := make(mapping, len(assignments))
	claimed := make(map[fieldName]bool, len(assignments))
	for _, one := range assignments {
		if err := checkAssignment(one, width, assigned, claimed); err != nil {
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

// withinColumns reports whether the column index names a column the import carries.
func withinColumns(column, width int) bool {
	return column >= 0 && column < width
}

// checkAssignment reports why one assignment cannot join the mapping being built.
func checkAssignment(one assignment, width int, assigned mapping, claimed map[fieldName]bool) error {
	switch {
	case !withinColumns(one.Column, width):
		return fmt.Errorf("the import carries no column %d", one.Column)
	case !mappable(one.Field):
		return fmt.Errorf("the registry carries no field %q", one.Field)
	case assigned[strconv.Itoa(one.Column)] != "":
		return fmt.Errorf("two assignments claim column %d", one.Column)
	case claimed[one.Field]:
		return fmt.Errorf("two assignments claim the field %q", one.Field)
	}
	return nil
}
