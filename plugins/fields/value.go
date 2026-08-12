// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Value errors.
var (
	errWrongKind         = errors.New("fields: the value does not match the kind its definition declares")
	errNoField           = errors.New("fields: no live definition holds that name")
	errValuesNotAnObject = errors.New("fields: values is an object of field names to values")
)

// coerce returns the storable form of a value, refusing one of another kind.
func coerce(held kind, given any) (any, error) {
	if given == nil {
		return nil, nil
	}
	switch held {
	case kindText, kindLongText, kindSelect:
		return coerceString(given)
	case kindNumber:
		return coerceNumber(given)
	case kindBoolean:
		return coerceBoolean(given)
	case kindDate:
		return coerceDate(given)
	}
	return nil, errWrongKind
}

// coerceString returns the value as text.
func coerceString(given any) (any, error) {
	text, ok := given.(string)
	if !ok {
		return nil, errWrongKind
	}
	return text, nil
}

// coerceNumber returns the value as a whole number.
func coerceNumber(given any) (any, error) {
	number, ok := given.(float64)
	if !ok || number != math.Trunc(number) {
		return nil, errWrongKind
	}
	return int64(number), nil
}

// coerceBoolean returns the value as a boolean.
func coerceBoolean(given any) (any, error) {
	flag, ok := given.(bool)
	if !ok {
		return nil, errWrongKind
	}
	return flag, nil
}

// coerceDate returns the value as a calendar day written YYYY-MM-DD.
func coerceDate(given any) (any, error) {
	text, ok := given.(string)
	if !ok {
		return nil, errWrongKind
	}
	if _, err := time.Parse(time.DateOnly, text); err != nil {
		return nil, errWrongKind
	}
	return text, nil
}

// checkValues returns the storable values, refusing unknown keys and wrong kinds.
func checkValues(live map[string]kind, given map[string]any) (map[string]any, error) {
	var unknown []string
	checked := make(map[string]any, len(given))
	for name, value := range given {
		held, defined := live[name]
		if !defined {
			unknown = append(unknown, name)
			continue
		}
		coerced, err := coerce(held, value)
		if err != nil {
			return nil, fmt.Errorf("%w: %s expects %s", errWrongKind, name, held)
		}
		checked[name] = coerced
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("%w: %s", errNoField, strings.Join(unknown, ", "))
	}
	return checked, nil
}
