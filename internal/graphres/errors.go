// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"context"
	"errors"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/gopherium/alphone/graph/scalar"
	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/cursor"
	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/task"
	"github.com/gopherium/alphone/internal/webhook"
)

// validationErrors lists the domain errors presented as VALIDATION.
var validationErrors = []error{
	contact.ErrEmptyName,
	contact.ErrEmptyChannel,
	contact.ErrEmptyIdentifier,
	contact.ErrChannelNotWritable,
	task.ErrEmptyTitle,
	task.ErrInvalidPriority,
	task.ErrInvalidStatus,
	task.ErrUnattributedOrigin,
	webhook.ErrInvalidURL,
	webhook.ErrNoEvents,
	event.ErrUnknownName,
	scalar.ErrInvalid,
	cursor.ErrMalformed,
	errExactlyOneTaskFilter,
	errInvalidFirst,
}

// notFoundErrors lists the domain errors presented as NOT_FOUND.
var notFoundErrors = []error{
	contact.ErrNotFound,
	contact.ErrIdentityNotFound,
	task.ErrNotFound,
	webhook.ErrNotFound,
}

// PresentError maps resolver errors to client-facing GraphQL errors.
func PresentError(ctx context.Context, err error) *gqlerror.Error {
	presented := graphql.DefaultErrorPresenter(ctx, err)
	var conflict contact.IdentityExistsError
	if errors.As(err, &conflict) {
		presented.Message = conflict.Error()
		withCode(presented, "CONFLICT")
		presented.Extensions["ownerContactId"] = conflict.OwnerID.String()
		return presented
	}
	if anyIs(err, validationErrors) {
		withCode(presented, "VALIDATION")
		return presented
	}
	if anyIs(err, notFoundErrors) {
		withCode(presented, "NOT_FOUND")
		return presented
	}
	if errors.Unwrap(presented) == nil {
		return presented
	}
	presented.Message = "internal error"
	withCode(presented, "INTERNAL")
	return presented
}

// anyIs reports whether err matches any of the targets.
func anyIs(err error, targets []error) bool {
	for _, target := range targets {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// withCode records the extensions code of a presented error.
func withCode(presented *gqlerror.Error, code string) {
	if presented.Extensions == nil {
		presented.Extensions = map[string]any{}
	}
	presented.Extensions["code"] = code
}
