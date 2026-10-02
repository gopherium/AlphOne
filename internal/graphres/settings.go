// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"cmp"
	"context"
	"fmt"
	"time"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/sdk"
)

// Page bounds and screen settings served when the environment names none.
const (
	DefaultPageSize      = 50
	DefaultPageCap       = 200
	DefaultToastDuration = 6 * time.Second
	DefaultListPageSize  = 20
	DefaultFormatLocale  = "es-ES"
)

// DefaultListPageSizes returns the page sizes a list offers when the environment names none.
func DefaultListPageSizes() []int {
	return []int{10, 20, 50, 100}
}

// Screens carries the settings the admin screens read once.
type Screens struct {
	// ToastDuration is how long a confirmation toast stays. Zero serves [DefaultToastDuration].
	ToastDuration time.Duration
	// PageSizes lists the page sizes a list offers. Empty serves [DefaultListPageSizes].
	PageSizes []int
	// PageSize is the page size a list opens on. Zero serves [DefaultListPageSize].
	PageSize int
	// FormatLocale is the locale dates, times, numbers and money are written in. Empty serves [DefaultFormatLocale].
	FormatLocale string
}

// pageSizes returns the page sizes a list offers.
func (s Screens) pageSizes() []int {
	if len(s.PageSizes) == 0 {
		return DefaultListPageSizes()
	}
	return s.PageSizes
}

// AdminSettings answers the settings the admin screens read once.
func (q QueryResolvers) AdminSettings(context.Context) (*model.AdminSettings, error) {
	screens := q.root.Screens
	return &model.AdminSettings{
		ToastMilliseconds: int(cmp.Or(screens.ToastDuration, DefaultToastDuration).Milliseconds()),
		ListPageSizes:     screens.pageSizes(),
		ListPageSize:      cmp.Or(screens.PageSize, DefaultListPageSize),
		ContactPageCap:    q.root.Paging.ceiling(),
		FormatLocale:      cmp.Or(screens.FormatLocale, DefaultFormatLocale),
	}, nil
}

// Paging bounds the pages the core lists answer.
type Paging struct {
	// Size is the page answered when a caller names none. Zero serves [DefaultPageSize].
	Size int
	// Cap is the largest page answered. Zero serves [DefaultPageCap].
	Cap int
}

// size returns the page answered when a caller names none.
func (p Paging) size() int {
	return cmp.Or(p.Size, DefaultPageSize)
}

// ceiling returns the largest page answered.
func (p Paging) ceiling() int {
	return cmp.Or(p.Cap, DefaultPageCap)
}

// pageSize resolves the size argument a list names into a page size.
func (p Paging) pageSize(argument string, asked *int) (int, error) {
	if asked == nil {
		return p.size(), nil
	}
	if *asked < 1 || *asked > p.ceiling() {
		return 0, sdk.GraphError{
			Code: "VALIDATION", Reason: "first_out_of_range",
			Meta: map[string]any{"min": 1, "max": p.ceiling()},
			Err:  fmt.Errorf("graph: %s must be between 1 and %d", argument, p.ceiling()),
		}
	}
	return *asked, nil
}
