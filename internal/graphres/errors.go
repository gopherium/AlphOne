// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"context"
	"errors"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/graph/scalar"
	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/cursor"
	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/internal/task"
	"github.com/gopherium/alphone/internal/webhook"
	"github.com/gopherium/alphone/sdk"
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
	authkit.ErrSelfDisable,
	authkit.ErrSelfRole,
	gouncer.ErrLastPrivileged,
	role.ErrLastAdmin,
	role.ErrBeyondReach,
	role.ErrUnknownTier,
	apitoken.ErrEmptyName,
	apitoken.ErrMalformedScope,
	apitoken.ErrNoScopes,
	apitoken.ErrUnknownArea,
	apitoken.ErrNegativeLifetime,
	apitoken.ErrLifetimeTooLong,
}

// notFoundErrors lists the domain errors presented as NOT_FOUND.
var notFoundErrors = []error{
	contact.ErrNotFound,
	contact.ErrIdentityNotFound,
	task.ErrNotFound,
	webhook.ErrNotFound,
	apitoken.ErrNotFound,
}

// PresentError maps resolver errors to client-facing GraphQL errors.
func PresentError(ctx context.Context, err error) *gqlerror.Error {
	presented := graphql.DefaultErrorPresenter(ctx, err)
	if applySpecialCode(presented, err) || applyListCode(presented, err) {
		return presented
	}
	if errors.Unwrap(presented) == nil {
		return presented
	}
	presented.Message = "internal error"
	withCode(presented, "INTERNAL")
	return presented
}

// applySpecialCode handles the errors carrying extra extension fields.
func applySpecialCode(presented *gqlerror.Error, err error) bool {
	var coded sdk.GraphError
	if errors.As(err, &coded) {
		withCode(presented, coded.Code)
		for key, value := range coded.Extensions {
			presented.Extensions[key] = value
		}
		return true
	}
	var conflict contact.IdentityExistsError
	if errors.As(err, &conflict) {
		presented.Message = conflict.Error()
		withCode(presented, "CONFLICT")
		presented.Extensions["ownerContactId"] = conflict.OwnerID.String()
		return true
	}
	var limited rateLimitedError
	if errors.As(err, &limited) {
		presented.Message = limited.Error()
		withCode(presented, "RATE_LIMITED")
		presented.Extensions["retryAfter"] = int(limited.retryAfter.Seconds())
		return true
	}
	if errors.Is(err, authkit.ErrInvalidCredentials) {
		withCode(presented, "UNAUTHENTICATED")
		return true
	}
	return false
}

// applyListCode classifies domain errors by the shared sentinel lists.
func applyListCode(presented *gqlerror.Error, err error) bool {
	if anyIs(err, validationErrors) {
		withCode(presented, "VALIDATION")
		return true
	}
	if anyIs(err, notFoundErrors) {
		withCode(presented, "NOT_FOUND")
		return true
	}
	if status, message, ok := authkit.StatusForAuthError(err); ok {
		presented.Message = message
		withCode(presented, codeForStatus(status))
		return true
	}
	return false
}

// codeForStatus maps an authkit HTTP status onto an extensions code.
func codeForStatus(status int) string {
	switch status {
	case 404:
		return "NOT_FOUND"
	case 409:
		return "CONFLICT"
	default:
		return "VALIDATION"
	}
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
