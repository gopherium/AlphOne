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

// reasonsFor names the stable reason and the data each refused sentinel answers with.
var reasonsFor = []struct {
	sentinel error
	reason   string
	meta     map[string]any
}{
	{contact.ErrEmptyName, "contact_name_required", nil},
	{contact.ErrEmptyChannel, "identity_channel_required", nil},
	{contact.ErrEmptyIdentifier, "identity_identifier_required", nil},
	{contact.ErrChannelNotWritable, "channel_not_writable", nil},
	{task.ErrEmptyTitle, "task_title_required", nil},
	{task.ErrInvalidPriority, "task_priority_unknown", nil},
	{task.ErrInvalidStatus, "task_status_unknown", nil},
	{task.ErrUnattributedOrigin, "origin_source_required", nil},
	{webhook.ErrInvalidURL, "webhook_url_invalid", nil},
	{webhook.ErrNoEvents, "webhook_events_required", nil},
	{event.ErrUnknownName, "event_unknown", nil},
	{scalar.ErrInvalid, "value_malformed", nil},
	{cursor.ErrMalformed, "cursor_malformed", nil},
	{errExactlyOneTaskFilter, "task_filter_choice_required", nil},
	{errInvalidFirst, "first_out_of_range", map[string]any{"min": 1, "max": maxPageSize}},
	{authkit.ErrSelfDisable, "self_disable_refused", nil},
	{authkit.ErrSelfRole, "self_role_refused", nil},
	{gouncer.ErrLastPrivileged, "last_privileged_refused", nil},
	{role.ErrLastAdmin, "last_privileged_refused", nil},
	{role.ErrBeyondReach, "role_beyond_reach", nil},
	{role.ErrUnknownTier, "role_unknown", nil},
	{apitoken.ErrEmptyName, "token_name_required", nil},
	{apitoken.ErrMalformedScope, "scope_malformed", nil},
	{apitoken.ErrNoScopes, "scopes_required", nil},
	{apitoken.ErrUnknownArea, "area_unknown", nil},
	{apitoken.ErrNegativeLifetime, "lifetime_negative", nil},
	{apitoken.ErrLifetimeTooLong, "lifetime_too_long", map[string]any{"maxDays": apitoken.MaxLifetimeDays}},
	{contact.ErrNotFound, "contact_not_found", nil},
	{contact.ErrIdentityNotFound, "identity_not_found", nil},
	{task.ErrNotFound, "task_not_found", nil},
	{webhook.ErrNotFound, "webhook_not_found", nil},
	{apitoken.ErrNotFound, "token_not_found", nil},
}

// withReason records the stable reason and its data on a presented error.
func withReason(presented *gqlerror.Error, reason string, meta map[string]any) {
	if reason == "" {
		return
	}
	presented.Extensions["reason"] = reason
	if len(meta) > 0 {
		presented.Extensions["meta"] = meta
	}
}

// reasonOf returns the table entry a refused sentinel answers with.
func reasonOf(err error) (string, map[string]any) {
	for _, held := range reasonsFor {
		if errors.Is(err, held.sentinel) {
			return held.reason, held.meta
		}
	}
	return "", nil
}

// spokenAs names every brick error in the deployment's own voice.
var spokenAs = []struct {
	sentinel error
	message  string
}{
	{authkit.ErrSelfRole, "you cannot change your own role"},
	{authkit.ErrSelfDisable, "you cannot disable your own account"},
	{gouncer.ErrLastPrivileged, role.ErrLastAdmin.Error()},
	{role.ErrBeyondReach, "that role is beyond your own"},
}

// speak rewrites a brick error so no package name reaches a caller.
func speak(presented *gqlerror.Error, err error) {
	for _, held := range spokenAs {
		if errors.Is(err, held.sentinel) {
			presented.Message = held.message
			return
		}
	}
}

// PresentError maps resolver errors to client-facing GraphQL errors.
func PresentError(ctx context.Context, err error) *gqlerror.Error {
	presented := graphql.DefaultErrorPresenter(ctx, err)
	speak(presented, err)
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
		withReason(presented, "identity_taken", map[string]any{"ownerContactId": conflict.OwnerID.String()})
		return true
	}
	var limited rateLimitedError
	if errors.As(err, &limited) {
		presented.Message = limited.Error()
		withCode(presented, "RATE_LIMITED")
		presented.Extensions["retryAfter"] = int(limited.retryAfter.Seconds())
		withReason(presented, "rate_limited", map[string]any{"retryAfter": int(limited.retryAfter.Seconds())})
		return true
	}
	if errors.Is(err, authkit.ErrInvalidCredentials) {
		withCode(presented, "UNAUTHENTICATED")
		withReason(presented, "credentials_invalid", nil)
		return true
	}
	return false
}

// applyListCode classifies domain errors by the shared sentinel lists.
func applyListCode(presented *gqlerror.Error, err error) bool {
	if anyIs(err, validationErrors) {
		withCode(presented, "VALIDATION")
		named, meta := reasonOf(err)
		withReason(presented, named, meta)
		return true
	}
	if anyIs(err, notFoundErrors) {
		withCode(presented, "NOT_FOUND")
		named, meta := reasonOf(err)
		withReason(presented, named, meta)
		return true
	}
	if status, response, ok := authkit.ErrorResponseForAuthError(err); ok {
		presented.Message = response.Message
		withCode(presented, codeForStatus(status))
		withReason(presented, response.Code, response.Meta)
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
