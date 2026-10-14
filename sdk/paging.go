// SPDX-License-Identifier: Elastic-2.0

package sdk

import "fmt"

// DefaultPageSize is the page a list answers when neither the caller nor the host names one.
const DefaultPageSize = 50

// DefaultPageCap is the largest page a list answers when the host names none.
const DefaultPageCap = 200

// PageBounds bounds the pages a plugin list answers.
type PageBounds struct {
	// Size is the page answered when a caller names none, zero or below for [DefaultPageSize].
	Size int
	// Cap is the largest page answered, zero or below for [DefaultPageCap].
	Cap int
}

// size returns the page answered when a caller names none.
func (b PageBounds) size() int {
	if b.Size < 1 {
		return DefaultPageSize
	}
	return b.Size
}

// ceiling returns the largest page answered.
func (b PageBounds) ceiling() int {
	if b.Cap < 1 {
		return DefaultPageCap
	}
	return b.Cap
}

// Limit resolves the limit a list call names into a page size, refusing one outside one and the cap.
func (b PageBounds) Limit(limit *int) (int, error) {
	if limit == nil {
		return b.size(), nil
	}
	if *limit < 1 || *limit > b.ceiling() {
		return 0, GraphError{
			Code: "VALIDATION", Reason: "first_out_of_range",
			Meta: map[string]any{"min": 1, "max": b.ceiling()},
			Err:  fmt.Errorf("limit must be between 1 and %d", b.ceiling()),
		}
	}
	return *limit, nil
}

// Offset resolves the offset a list call names, a missing or negative one starting at the first row.
func Offset(offset *int) int {
	if offset == nil {
		return 0
	}
	return max(0, *offset)
}
