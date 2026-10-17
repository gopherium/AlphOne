// SPDX-License-Identifier: Elastic-2.0

package scalar_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/graph/scalar"
)

func TestUUIDRoundTrip(t *testing.T) {
	t.Parallel()

	id := uuid.MustParse("6f1c2e6a-8f2b-4c1d-9e3f-5a7b9c1d2e3f")

	var buf bytes.Buffer
	scalar.MarshalUUID(id).MarshalGQL(&buf)
	if got, want := buf.String(), `"6f1c2e6a-8f2b-4c1d-9e3f-5a7b9c1d2e3f"`; got != want {
		t.Errorf("marshaled uuid = %s, want %s", got, want)
	}

	parsed, err := scalar.UnmarshalUUID("6f1c2e6a-8f2b-4c1d-9e3f-5a7b9c1d2e3f")
	if err != nil {
		t.Fatalf("UnmarshalUUID() error = %v, want nil", err)
	}
	if parsed != id {
		t.Errorf("unmarshaled uuid = %s, want %s", parsed, id)
	}
}

// refusalOf returns the error unmarshal answers for a value.
func refusalOf[T any](unmarshal func(any) (T, error)) func(any) error {
	return func(value any) error {
		_, err := unmarshal(value)
		return err
	}
}

func TestUUIDAndDateScalarsRejectInvalidInput(t *testing.T) {
	t.Parallel()

	wrongTypeInput := 42
	for _, tt := range []struct {
		name  string
		input any
		read  func(any) error
	}{
		{"UnmarshalUUID", "not-a-uuid", refusalOf(scalar.UnmarshalUUID)},
		{"UnmarshalUUID", wrongTypeInput, refusalOf(scalar.UnmarshalUUID)},
		{"UnmarshalDateTime", wrongTypeInput, refusalOf(scalar.UnmarshalDateTime)},
		{"UnmarshalDate", wrongTypeInput, refusalOf(scalar.UnmarshalDate)},
	} {
		if err := tt.read(tt.input); !errors.Is(err, scalar.ErrInvalid) {
			t.Errorf("%s(%#v) error = %v, want ErrInvalid", tt.name, tt.input, err)
		}
	}
}

func TestDateTimeMatchesEncodingJSON(t *testing.T) {
	t.Parallel()

	withMicros := time.Date(2026, 8, 5, 10, 30, 15, 123456000, time.FixedZone("offset", 2*3600))

	jsonBytes, err := json.Marshal(withMicros.UTC())
	if err != nil {
		t.Fatalf("json.Marshal() error = %v, want nil", err)
	}
	var buf bytes.Buffer
	scalar.MarshalDateTime(withMicros).MarshalGQL(&buf)

	if got, want := buf.String(), string(jsonBytes); got != want {
		t.Errorf("marshaled datetime = %s, want the encoding/json bytes %s", got, want)
	}
	if !bytes.HasSuffix(buf.Bytes(), []byte(`Z"`)) {
		t.Errorf("marshaled datetime = %s, want a UTC Z suffix", buf.String())
	}
}

func TestDateTimeRoundTrip(t *testing.T) {
	t.Parallel()

	withMicros := time.Date(2026, 8, 5, 10, 30, 15, 123456000, time.UTC)

	parsed, err := scalar.UnmarshalDateTime("2026-08-05T10:30:15.123456Z")
	if err != nil {
		t.Fatalf("UnmarshalDateTime() error = %v, want nil", err)
	}
	if !parsed.Equal(withMicros) {
		t.Errorf("unmarshaled datetime = %v, want %v", parsed, withMicros)
	}

	if _, err := scalar.UnmarshalDateTime("yesterday"); !errors.Is(err, scalar.ErrInvalid) {
		t.Errorf("UnmarshalDateTime(yesterday) error = %v, want ErrInvalid", err)
	}
}

func TestDateRoundTrip(t *testing.T) {
	t.Parallel()

	dueOn := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)

	var buf bytes.Buffer
	scalar.MarshalDate(dueOn).MarshalGQL(&buf)
	if got, want := buf.String(), `"2026-08-05"`; got != want {
		t.Errorf("marshaled date = %s, want %s", got, want)
	}

	parsed, err := scalar.UnmarshalDate("2026-08-05")
	if err != nil {
		t.Fatalf("UnmarshalDate() error = %v, want nil", err)
	}
	if !parsed.Equal(dueOn) {
		t.Errorf("unmarshaled date = %v, want %v", parsed, dueOn)
	}

	europeanFormat := "05/08/2026"
	if _, err := scalar.UnmarshalDate(europeanFormat); !errors.Is(err, scalar.ErrInvalid) {
		t.Errorf("UnmarshalDate(%s) error = %v, want ErrInvalid", europeanFormat, err)
	}
}

func TestLocalTimeRoundTripsEveryEdgeOfTheDay(t *testing.T) {
	t.Parallel()

	for _, clock := range []string{"00:00", "09:05", "19:59", "20:00", "23:59"} {
		parsed, err := scalar.UnmarshalLocalTime(clock)
		if err != nil {
			t.Fatalf("UnmarshalLocalTime(%q) error = %v, want nil", clock, err)
		}
		if parsed != clock {
			t.Errorf("unmarshaled time = %q, want %q", parsed, clock)
		}

		var buf bytes.Buffer
		scalar.MarshalLocalTime(parsed).MarshalGQL(&buf)
		if got, want := buf.String(), strconv.Quote(clock); got != want {
			t.Errorf("marshaled time = %s, want %s", got, want)
		}
	}
}

func TestLocalTimeRefusesAnythingButAWallClockMinute(t *testing.T) {
	t.Parallel()

	wrongTypeInput := 930
	for _, malformed := range []any{
		"9:00", "24:00", "09:00:30", "11:60", "", "0900", " 09:00", "09:00\n", wrongTypeInput,
	} {
		if _, err := scalar.UnmarshalLocalTime(malformed); !errors.Is(err, scalar.ErrInvalid) {
			t.Errorf("UnmarshalLocalTime(%#v) error = %v, want ErrInvalid", malformed, err)
		}
	}
}
