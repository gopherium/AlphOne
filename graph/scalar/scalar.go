// SPDX-License-Identifier: Elastic-2.0

// Package scalar implements the wire codecs of the custom GraphQL scalars.
package scalar

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/google/uuid"
)

// ErrInvalid reports a scalar input the codec cannot parse.
var ErrInvalid = errors.New("scalar: invalid value")

// dateLayout is the wire format of the Date scalar.
const dateLayout = "2006-01-02"

// localTimePattern matches the wire format of the LocalTime scalar.
var localTimePattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// parsedString reads value as a string that parse accepts, or reports [ErrInvalid] naming the scalar and its form.
func parsedString[T any](value any, name, form string, parse func(string) (T, error)) (T, error) {
	var zero T
	raw, ok := value.(string)
	if !ok {
		return zero, fmt.Errorf("%w: a %s must be a string", ErrInvalid, name)
	}
	parsed, err := parse(raw)
	if err != nil {
		return zero, fmt.Errorf("%w: %q is not %s", ErrInvalid, raw, form)
	}
	return parsed, nil
}

// timeIn returns a parser reading a time written in layout.
func timeIn(layout string) func(string) (time.Time, error) {
	return func(raw string) (time.Time, error) {
		return time.Parse(layout, raw)
	}
}

// wallClock returns raw when it spells an HH:MM time of day, or reports [ErrInvalid].
func wallClock(raw string) (string, error) {
	if !localTimePattern.MatchString(raw) {
		return "", ErrInvalid
	}
	return raw, nil
}

// MarshalUUID renders id as a quoted UUID string.
func MarshalUUID(id uuid.UUID) graphql.Marshaler {
	return graphql.MarshalString(id.String())
}

// UnmarshalUUID parses value as a UUID string.
func UnmarshalUUID(value any) (uuid.UUID, error) {
	return parsedString(value, "UUID", "a UUID", uuid.Parse)
}

// MarshalDateTime renders t as a quoted RFC3339 UTC timestamp with nanoseconds.
func MarshalDateTime(t time.Time) graphql.Marshaler {
	return graphql.MarshalString(t.UTC().Format(time.RFC3339Nano))
}

// UnmarshalDateTime parses value as an RFC3339 timestamp.
func UnmarshalDateTime(value any) (time.Time, error) {
	return parsedString(value, "DateTime", "an RFC3339 timestamp", timeIn(time.RFC3339Nano))
}

// MarshalDate renders t as a quoted YYYY-MM-DD string.
func MarshalDate(t time.Time) graphql.Marshaler {
	return graphql.MarshalString(t.Format(dateLayout))
}

// UnmarshalDate parses value as a YYYY-MM-DD string.
func UnmarshalDate(value any) (time.Time, error) {
	return parsedString(value, "Date", "a YYYY-MM-DD date", timeIn(dateLayout))
}

// MarshalLocalTime renders clock as a quoted HH:MM string.
func MarshalLocalTime(clock string) graphql.Marshaler {
	return graphql.MarshalString(clock)
}

// UnmarshalLocalTime parses value as an HH:MM time of day.
func UnmarshalLocalTime(value any) (string, error) {
	return parsedString(value, "LocalTime", "an HH:MM time of day", wallClock)
}
