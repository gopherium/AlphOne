// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/gopherium/alphone/graph/scalar"
	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/task"
	"github.com/gopherium/alphone/internal/webhook"
)

// code extracts the extensions code of a presented error.
func code(t *testing.T, presented *gqlerror.Error) string {
	t.Helper()
	raw, ok := presented.Extensions["code"].(string)
	if !ok {
		t.Fatalf("presented error carries no code: %+v", presented)
	}
	return raw
}

func TestPresentErrorMapsDomainErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want string
	}{
		{"empty title", task.ErrEmptyTitle, "VALIDATION"},
		{"invalid priority", task.ErrInvalidPriority, "VALIDATION"},
		{"empty contact name", contact.ErrEmptyName, "VALIDATION"},
		{"invalid webhook url", webhook.ErrInvalidURL, "VALIDATION"},
		{"unknown event name", event.ErrUnknownName, "VALIDATION"},
		{"invalid scalar input", fmt.Errorf("%w: bad date", scalar.ErrInvalid), "VALIDATION"},
		{"contact not found", contact.ErrNotFound, "NOT_FOUND"},
		{"identity not found", contact.ErrIdentityNotFound, "NOT_FOUND"},
		{"task not found", task.ErrNotFound, "NOT_FOUND"},
		{"webhook not found", webhook.ErrNotFound, "NOT_FOUND"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			presented := graphres.PresentError(context.Background(), testCase.err)
			if got := code(t, presented); got != testCase.want {
				t.Errorf("code = %q, want %q", got, testCase.want)
			}
			if presented.Message != testCase.err.Error() {
				t.Errorf("message = %q, want %q", presented.Message, testCase.err.Error())
			}
		})
	}
}

func TestPresentErrorCarriesTheConflictOwner(t *testing.T) {
	t.Parallel()

	ownerID := uuid.MustParse("3b1f4d5e-6a7b-4c8d-9e0f-1a2b3c4d5e6f")

	presented := graphres.PresentError(context.Background(), contact.IdentityExistsError{OwnerID: ownerID})

	if got := code(t, presented); got != "CONFLICT" {
		t.Errorf("code = %q, want CONFLICT", got)
	}
	if got := presented.Extensions["ownerContactId"]; got != ownerID.String() {
		t.Errorf("ownerContactId = %v, want %s", got, ownerID)
	}
	if presented.Message != contact.ErrIdentityExists.Error() {
		t.Errorf("message = %q, want %q", presented.Message, contact.ErrIdentityExists.Error())
	}
}

func TestPresentErrorMasksUnrecognizedErrors(t *testing.T) {
	t.Parallel()

	leakyStoreError := errors.New("pgx: connection refused during scan")

	presented := graphres.PresentError(context.Background(), leakyStoreError)

	if got := code(t, presented); got != "INTERNAL" {
		t.Errorf("code = %q, want INTERNAL", got)
	}
	if presented.Message != "internal error" {
		t.Errorf("message = %q, want the masked internal error", presented.Message)
	}
	if strings.Contains(presented.Message, "pgx") {
		t.Errorf("message %q leaks the store error", presented.Message)
	}
}

func TestPresentErrorPassesGraphQLValidationThrough(t *testing.T) {
	t.Parallel()

	parseFailure := gqlerror.Errorf("Cannot query field %q on type %q.", "nope", "Query")

	presented := graphres.PresentError(context.Background(), parseFailure)

	if presented.Message != parseFailure.Message {
		t.Errorf("message = %q, want %q untouched", presented.Message, parseFailure.Message)
	}
	if got := presented.Extensions["code"]; got == "INTERNAL" {
		t.Error("a gqlgen validation error was masked as INTERNAL")
	}
}
