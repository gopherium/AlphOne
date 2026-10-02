// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"sort"
	"strings"
	"time"
)

// Value errors.
var (
	errWrongKind         = errors.New("fields: the value does not match the kind its definition declares")
	errNoField           = errors.New("fields: no live definition holds that name")
	errValuesNotAnObject = errors.New("fields: values is an object of field names to values")
	errEntryEmpty        = errors.New("fields: an entry holds at least one filled cell")

	errRepeaterEntriesOnly = errors.New("fields: a repeater takes its entries one at a time")
)

// checkEntry returns the storable cells of one repeater entry, refusing unknown keys, wrong kinds and a blank entry.
func checkEntry(name string, columns []SubField, given any, own string) (map[string]any, error) {
	cells, ok := given.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: %s expects an entry", errWrongKind, name)
	}
	entry, unknown, err := checkRow(name, columnKinds(columns), withoutOwnID(cells, own))
	if err != nil {
		return nil, err
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("%w: %s", errNoField, strings.Join(unknown, ", "))
	}
	dropBlankText(entry)
	if len(entry) == 0 {
		return nil, fmt.Errorf("%w: %s", errEntryEmpty, name)
	}
	return entry, nil
}

// withoutOwnID returns the cells with an id cell naming the entry's own id left out.
func withoutOwnID(cells map[string]any, own string) map[string]any {
	if own == "" || cells[entryIDKey] != own {
		return cells
	}
	kept := maps.Clone(cells)
	delete(kept, entryIDKey)
	return kept
}

// dropBlankText removes every text cell holding nothing but white space.
func dropBlankText(entry map[string]any) {
	for key, value := range entry {
		if text, isText := value.(string); isText && strings.TrimSpace(text) == "" {
			delete(entry, key)
		}
	}
}

// columnKinds maps each sub field name to the kind it declares.
func columnKinds(columns []SubField) map[string]kind {
	kindsOf := make(map[string]kind, len(columns))
	for _, column := range columns {
		kindsOf[column.Name] = column.Kind
	}
	return kindsOf
}

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

// coerceNumber returns the value as a whole number the Int scalar holds.
func coerceNumber(given any) (any, error) {
	number, ok := decimalOf(given)
	if !ok || number != math.Trunc(number) {
		return nil, errWrongKind
	}
	if number < math.MinInt32 || number > math.MaxInt32 {
		return nil, errWrongKind
	}
	return int64(number), nil
}

// decimalOf reads a number from a variable or an inline literal, which arrive typed differently.
func decimalOf(given any) (float64, bool) {
	switch held := given.(type) {
	case float64:
		return held, true
	case int64:
		return float64(held), true
	case json.Number:
		number, err := held.Float64()
		return number, err == nil
	}
	return 0, false
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

// valueError is a value check error beside the fields it names and the kind of the one named field.
type valueError struct {
	err   error
	names []string
	kind  kind
}

// Error returns the check error text.
func (e valueError) Error() string {
	return e.err.Error()
}

// Unwrap returns the check error.
func (e valueError) Unwrap() error {
	return e.err
}

// checkValues returns the storable values the view allows, refusing repeaters, unknown keys and wrong kinds.
func checkValues(live *view, given map[string]any) (map[string]any, error) {
	if repeaters := repeatersIn(live, given); len(repeaters) > 0 {
		return nil, valueError{
			err:   fmt.Errorf("%w: %s", errRepeaterEntriesOnly, strings.Join(repeaters, ", ")),
			names: repeaters,
		}
	}
	var unknown []string
	checked := make(map[string]any, len(given))
	for _, name := range slices.Sorted(maps.Keys(given)) {
		held, defined := live.kinds[name]
		if !defined {
			unknown = append(unknown, name)
			continue
		}
		coerced, err := coerce(held, given[name])
		if err != nil {
			return nil, valueError{
				err:   fmt.Errorf("%w: %s expects %s", errWrongKind, name, held),
				names: []string{name},
				kind:  held,
			}
		}
		checked[name] = coerced
	}
	if len(unknown) > 0 {
		return nil, valueError{err: fmt.Errorf("%w: %s", errNoField, strings.Join(unknown, ", ")), names: unknown}
	}
	return checked, nil
}

// repeatersIn returns the given names the view holds as repeaters, sorted.
func repeatersIn(live *view, given map[string]any) []string {
	var named []string
	for name := range given {
		if live.kinds[name] == kindRepeater {
			named = append(named, name)
		}
	}
	sort.Strings(named)
	return named
}

// checkRow returns the storable cells of one entry and the paths of the cells no column holds.
func checkRow(path string, kindsOf map[string]kind, cells map[string]any) (map[string]any, []string, error) {
	var unknown []string
	row := make(map[string]any, len(cells))
	for key, value := range cells {
		held, known := kindsOf[key]
		if !known {
			unknown = append(unknown, path+"."+key)
			continue
		}
		coerced, err := coerce(held, value)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %s.%s expects %s", errWrongKind, path, key, held)
		}
		if coerced != nil {
			row[key] = coerced
		}
	}
	return row, unknown, nil
}
