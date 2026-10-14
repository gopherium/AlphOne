// SPDX-License-Identifier: Elastic-2.0

package sdk

import (
	"errors"
	"testing"
)

func TestPageBoundsAnswerTheHostPageWhenTheCallerNamesNone(t *testing.T) {
	t.Parallel()

	got, err := PageBounds{Size: 7, Cap: 9}.Limit(nil)

	if err != nil || got != 7 {
		t.Errorf("Limit(nil) = %d, %v, want the host page of 7", got, err)
	}
}

func TestPageBoundsAcceptALimitUpToTheCap(t *testing.T) {
	t.Parallel()

	for _, asked := range []int{1, 9} {
		got, err := PageBounds{Size: 7, Cap: 9}.Limit(&asked)

		if err != nil || got != asked {
			t.Errorf("Limit(%d) = %d, %v, want it accepted", asked, got, err)
		}
	}
}

func TestPageBoundsRefuseALimitOutsideOneAndTheCap(t *testing.T) {
	t.Parallel()

	for _, asked := range []int{0, -1, 10} {
		t.Run("", func(t *testing.T) {
			t.Parallel()

			_, err := PageBounds{Size: 7, Cap: 9}.Limit(&asked)

			var graphErr GraphError
			if !errors.As(err, &graphErr) {
				t.Fatalf("Limit(%d) error = %v, want a graph error", asked, err)
			}
			if graphErr.Code != "VALIDATION" || graphErr.Reason != "first_out_of_range" {
				t.Errorf("Limit(%d) = %s %s, want VALIDATION first_out_of_range", asked, graphErr.Code, graphErr.Reason)
			}
			if graphErr.Meta["min"] != 1 || graphErr.Meta["max"] != 9 {
				t.Errorf("Limit(%d) meta = %v, want min 1 and max 9", asked, graphErr.Meta)
			}
		})
	}
}

func TestPageBoundsTheHostLeftUnsetServeTheDefaults(t *testing.T) {
	t.Parallel()

	for name, bounds := range map[string]PageBounds{
		"zero":  {},
		"below": {Size: -1, Cap: -1},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			unsized, err := bounds.Limit(nil)
			if err != nil || unsized != DefaultPageSize {
				t.Errorf("Limit(nil) = %d, %v, want the default page %d", unsized, err, DefaultPageSize)
			}
			top := DefaultPageCap
			if got, err := bounds.Limit(&top); err != nil || got != top {
				t.Errorf("Limit(%d) = %d, %v, want the default cap accepted", top, got, err)
			}
			past := DefaultPageCap + 1
			if _, err := bounds.Limit(&past); err == nil {
				t.Errorf("Limit(%d) error = nil, want a page past the default cap refused", past)
			}
		})
	}
}

func TestTheDefaultPageBoundsMatchTheCoreLists(t *testing.T) {
	t.Parallel()

	if DefaultPageSize != 50 || DefaultPageCap != 200 {
		t.Errorf("defaults = %d and %d, want 50 and 200", DefaultPageSize, DefaultPageCap)
	}
}

func TestOffsetStartsAtTheFirstRowForAMissingOrNegativeOffset(t *testing.T) {
	t.Parallel()

	negative, positive := -3, 4

	if got := Offset(nil); got != 0 {
		t.Errorf("Offset(nil) = %d, want 0", got)
	}
	if got := Offset(&negative); got != 0 {
		t.Errorf("Offset(-3) = %d, want 0", got)
	}
	if got := Offset(&positive); got != 4 {
		t.Errorf("Offset(4) = %d, want 4", got)
	}
}
