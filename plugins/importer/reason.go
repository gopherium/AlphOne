// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"encoding/csv"
	"errors"

	"github.com/gopherium/alphone/sdk"
)

// The codes a row reason names beside the two it shares with [sdk.FieldTextError].
const (
	reasonCellCountMismatch = "row_cell_count_mismatch"
	reasonQuoteMisplaced    = "row_quote_misplaced"
	reasonMalformed         = "row_malformed"
	reasonIncomplete        = "row_incomplete"
	reasonContactInvalid    = "contact_details_invalid"
	reasonIdentityTaken     = "identity_taken"
	reasonIdentityTakenBy   = "identity_taken_by"
	reasonFieldTextRefused  = "field_text_refused"
)

// rowReasonCodes lists every code the importer writes on a row.
var rowReasonCodes = []string{
	reasonCellCountMismatch, reasonQuoteMisplaced, reasonMalformed, reasonIncomplete, reasonContactInvalid,
	reasonIdentityTaken, reasonIdentityTakenBy, sdk.FieldTextKindMismatch, sdk.FieldTextFieldUnknown,
	reasonFieldTextRefused,
}

// rowReason is why a staged row settled as it did, a stable code beside the values its sentence names.
type rowReason struct {
	Code string         `json:"code"`
	Meta map[string]any `json:"meta"`
}

// newReason returns the reason a code names, its meta never nil.
func newReason(code string, meta map[string]any) rowReason {
	if meta == nil {
		meta = map[string]any{}
	}
	return rowReason{Code: code, Meta: meta}
}

// cellCountReason returns the note of a row whose cell count differs from the header.
func cellCountReason(cells, columns int) rowReason {
	return newReason(reasonCellCountMismatch, map[string]any{"cells": cells, "columns": columns})
}

// malformedReason returns the note of a row the reader could not read, naming the line of a misplaced quote.
func malformedReason(err error, linesBefore int) rowReason {
	var parsed *csv.ParseError
	if errors.As(err, &parsed) && (errors.Is(err, csv.ErrBareQuote) || errors.Is(err, csv.ErrQuote)) {
		return newReason(reasonQuoteMisplaced, map[string]any{"line": parsed.Line + linesBefore})
	}
	return newReason(reasonMalformed, nil)
}

// fieldTextReason returns the reason of a row carrying a value no field accepts.
func fieldTextReason(err error) rowReason {
	var named sdk.FieldTextError
	if !errors.As(err, &named) {
		return newReason(reasonFieldTextRefused, nil)
	}
	switch {
	case named.Reason == sdk.FieldTextKindMismatch && len(named.Fields) == 1 && named.Kind != "":
		return newReason(sdk.FieldTextKindMismatch, map[string]any{"field": named.Fields[0], "kind": named.Kind})
	case named.Reason == sdk.FieldTextFieldUnknown && len(named.Fields) > 0:
		return newReason(sdk.FieldTextFieldUnknown, map[string]any{"fields": named.Fields})
	}
	return newReason(reasonFieldTextRefused, nil)
}

// identityTakenReason returns the reason of a row whose address the named contact already holds.
func identityTakenReason(owner string) rowReason {
	return newReason(reasonIdentityTakenBy, map[string]any{"ownerName": owner})
}

// unreadable reports whether the note marks a row the reader could not read.
func (r *rowReason) unreadable() bool {
	return r != nil && (r.Code == reasonMalformed || r.Code == reasonQuoteMisplaced)
}

// optionalReason returns the reason a row stores, or nil when it carries none.
func optionalReason(reason rowReason) *rowReason {
	if reason.Code == "" {
		return nil
	}
	return &reason
}
