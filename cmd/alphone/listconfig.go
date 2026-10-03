// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/language"

	"github.com/gopherium/framework/gonsole"

	"github.com/gopherium/alphone/internal/graphres"
)

// listSettings bounds the pages the graph answers and the lists the admin screens draw.
type listSettings struct {
	paging  graphres.Paging
	screens graphres.Screens
}

// loadListSettings reads the graph page bounds and the admin screen settings from the environment.
func loadListSettings(env gonsole.Env) (listSettings, error) {
	paging, err := loadPaging(env)
	if err != nil {
		return listSettings{}, err
	}
	screens, err := loadScreens(env, paging.Cap)
	if err != nil {
		return listSettings{}, err
	}
	return listSettings{paging: paging, screens: screens}, nil
}

// loadPaging reads the page the graph answers by default and the largest one it answers.
func loadPaging(env gonsole.Env) (graphres.Paging, error) {
	size, err := env.Count("GRAPH_PAGE_SIZE", graphres.DefaultPageSize)
	if err != nil {
		return graphres.Paging{}, err
	}
	ceiling, err := env.Count("GRAPH_PAGE_CAP", graphres.DefaultPageCap)
	if err != nil {
		return graphres.Paging{}, err
	}
	if largest := graphres.LargestPricedPage(); ceiling > largest {
		return graphres.Paging{}, fmt.Errorf("%s: must not exceed %d, the most rows a read of one field a row fits "+
			"under the query cost limit %d, got %d", env.Key("GRAPH_PAGE_CAP"), largest, graphres.ComplexityLimit, ceiling)
	}
	if size > ceiling {
		return graphres.Paging{}, fmt.Errorf("%s: must not exceed %s %d, got %d",
			env.Key("GRAPH_PAGE_SIZE"), env.Key("GRAPH_PAGE_CAP"), ceiling, size)
	}
	return graphres.Paging{Size: size, Cap: ceiling}, nil
}

// loadScreens reads the toast time, the pages a list offers, none past ceiling, and the format locale.
func loadScreens(env gonsole.Env, ceiling int) (graphres.Screens, error) {
	toast, err := gonsole.Parse(env, "TOAST_DURATION", graphres.DefaultToastDuration, parseToastDuration)
	if err != nil {
		return graphres.Screens{}, err
	}
	sizes, err := loadPageSizes(env, ceiling)
	if err != nil {
		return graphres.Screens{}, err
	}
	size, err := env.Count("LIST_PAGE_SIZE", graphres.DefaultListPageSize)
	if err != nil {
		return graphres.Screens{}, err
	}
	if !slices.Contains(sizes, size) {
		return graphres.Screens{}, fmt.Errorf("%s: must be one of %s %v, got %d",
			env.Key("LIST_PAGE_SIZE"), env.Key("LIST_PAGE_SIZES"), sizes, size)
	}
	locale, err := gonsole.Parse(env, "FORMAT_LOCALE", graphres.DefaultFormatLocale, parseFormatLocale)
	if err != nil {
		return graphres.Screens{}, err
	}
	return graphres.Screens{ToastDuration: toast, PageSizes: sizes, PageSize: size, FormatLocale: locale}, nil
}

// parseFormatLocale reads the locale dates and numbers are written in as a canonical BCP 47 tag that names a language.
func parseFormatLocale(value string) (string, error) {
	tag, err := language.Parse(value)
	base, _, _ := tag.Raw()
	if err != nil || base.String() == "und" {
		return "", fmt.Errorf("must be a BCP 47 language tag such as es-ES or en-GB, got %q", value)
	}
	return tag.String(), nil
}

// longestToast is the longest toast a browser timer and a graph Int both hold.
const longestToast = math.MaxInt32 * time.Millisecond

// parseToastDuration reads how long a toast stays in whole milliseconds.
func parseToastDuration(value string) (time.Duration, error) {
	held, err := time.ParseDuration(value)
	if err != nil || held < time.Millisecond || held > longestToast || held%time.Millisecond != 0 {
		return 0, fmt.Errorf("must be a duration such as 6s or 1500ms, "+
			"in whole milliseconds from 1ms to %dms, got %q", longestToast.Milliseconds(), value)
	}
	return held, nil
}

// loadPageSizes reads the page sizes a list offers, the defaults when unset, none past ceiling.
func loadPageSizes(env gonsole.Env, ceiling int) ([]int, error) {
	sizes, err := gonsole.Parse(env, "LIST_PAGE_SIZES", graphres.DefaultListPageSizes(), readPageSizes)
	if err != nil {
		return nil, err
	}
	for _, size := range sizes {
		if size > ceiling {
			return nil, fmt.Errorf("%s: must hold no size past %s %d, got %d",
				env.Key("LIST_PAGE_SIZES"), env.Key("GRAPH_PAGE_CAP"), ceiling, size)
		}
	}
	return sizes, nil
}

// readPageSizes reads a comma separated list of positive page sizes, each once from the smallest up.
func readPageSizes(value string) ([]int, error) {
	entries := strings.Split(value, ",")
	sizes := make([]int, 0, len(entries))
	for _, entry := range entries {
		size, err := strconv.Atoi(strings.TrimSpace(entry))
		if err != nil || size <= 0 {
			return nil, fmt.Errorf("must list positive whole numbers, got %q", value)
		}
		if len(sizes) > 0 && size <= sizes[len(sizes)-1] {
			return nil, fmt.Errorf("must list each size once from the smallest up, got %q", value)
		}
		sizes = append(sizes, size)
	}
	return sizes, nil
}
