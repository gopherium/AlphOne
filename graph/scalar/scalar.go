// SPDX-License-Identifier: Elastic-2.0

// Package scalar implements the wire codecs of the custom GraphQL scalars.
package scalar

import (
	"errors"
	"fmt"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/google/uuid"
)

// ErrInvalid reports a scalar input the codec cannot parse.
var ErrInvalid = errors.New("scalar: invalid value")

// dateLayout is the wire format of the Date scalar.
const dateLayout = "2006-01-02"

// MarshalUUID renders id as a quoted UUID string.
func MarshalUUID(id uuid.UUID) graphql.Marshaler {
	return graphql.MarshalString(id.String())
}

// UnmarshalUUID parses value as a UUID string.
func UnmarshalUUID(value any) (uuid.UUID, error) {
	raw, ok := value.(string)
	if !ok {
		return uuid.Nil, fmt.Errorf("%w: a UUID must be a string", ErrInvalid)
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: %q is not a UUID", ErrInvalid, raw)
	}
	return id, nil
}

// MarshalDateTime renders t as a quoted RFC3339 UTC timestamp with nanoseconds.
func MarshalDateTime(t time.Time) graphql.Marshaler {
	return graphql.MarshalString(t.UTC().Format(time.RFC3339Nano))
}

// UnmarshalDateTime parses value as an RFC3339 timestamp.
func UnmarshalDateTime(value any) (time.Time, error) {
	raw, ok := value.(string)
	if !ok {
		return time.Time{}, fmt.Errorf("%w: a DateTime must be a string", ErrInvalid)
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %q is not an RFC3339 timestamp", ErrInvalid, raw)
	}
	return t, nil
}

// MarshalDate renders t as a quoted YYYY-MM-DD string.
func MarshalDate(t time.Time) graphql.Marshaler {
	return graphql.MarshalString(t.Format(dateLayout))
}

// UnmarshalDate parses value as a YYYY-MM-DD string.
func UnmarshalDate(value any) (time.Time, error) {
	raw, ok := value.(string)
	if !ok {
		return time.Time{}, fmt.Errorf("%w: a Date must be a string", ErrInvalid)
	}
	t, err := time.Parse(dateLayout, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %q is not a YYYY-MM-DD date", ErrInvalid, raw)
	}
	return t, nil
}
