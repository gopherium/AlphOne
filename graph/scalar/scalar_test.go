// SPDX-License-Identifier: Elastic-2.0

package scalar_test

import (
	"bytes"
	"encoding/json"
	"errors"
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

func TestUUIDRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	if _, err := scalar.UnmarshalUUID("not-a-uuid"); !errors.Is(err, scalar.ErrInvalid) {
		t.Errorf("UnmarshalUUID(not-a-uuid) error = %v, want ErrInvalid", err)
	}
	wrongTypeInput := 42
	if _, err := scalar.UnmarshalUUID(wrongTypeInput); !errors.Is(err, scalar.ErrInvalid) {
		t.Errorf("UnmarshalUUID(42) error = %v, want ErrInvalid", err)
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
