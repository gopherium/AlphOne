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

	"github.com/gopherium/alphone/internal/graphres"
)

// listSettings bounds the pages the graph answers and the lists the admin screens draw.
type listSettings struct {
	paging  graphres.Paging
	screens graphres.Screens
}

// loadListSettings reads the graph page bounds and the admin screen settings from the environment.
func loadListSettings(getenv func(string) string) (listSettings, error) {
	paging, err := loadPaging(getenv)
	if err != nil {
		return listSettings{}, err
	}
	screens, err := loadScreens(getenv, paging.Cap)
	if err != nil {
		return listSettings{}, err
	}
	return listSettings{paging: paging, screens: screens}, nil
}

// loadPaging reads the page the graph answers by default and the largest one it answers.
func loadPaging(getenv func(string) string) (graphres.Paging, error) {
	size, err := parsePositiveCount("ALPHONE_GRAPH_PAGE_SIZE", getenv("ALPHONE_GRAPH_PAGE_SIZE"),
		graphres.DefaultPageSize)
	if err != nil {
		return graphres.Paging{}, err
	}
	ceiling, err := parsePositiveCount("ALPHONE_GRAPH_PAGE_CAP", getenv("ALPHONE_GRAPH_PAGE_CAP"),
		graphres.DefaultPageCap)
	if err != nil {
		return graphres.Paging{}, err
	}
	if largest := graphres.LargestPricedPage(); ceiling > largest {
		return graphres.Paging{}, fmt.Errorf(
			"ALPHONE_GRAPH_PAGE_CAP %d must not exceed %d, the most rows a read of one field a row fits "+
				"under the query cost limit %d", ceiling, largest, graphres.ComplexityLimit)
	}
	if size > ceiling {
		return graphres.Paging{}, fmt.Errorf(
			"ALPHONE_GRAPH_PAGE_SIZE %d must not exceed ALPHONE_GRAPH_PAGE_CAP %d", size, ceiling)
	}
	return graphres.Paging{Size: size, Cap: ceiling}, nil
}

// loadScreens reads the toast time, the pages a list offers, none past ceiling, and the format locale.
func loadScreens(getenv func(string) string, ceiling int) (graphres.Screens, error) {
	toast, err := parseToastDuration(getenv("ALPHONE_TOAST_DURATION"))
	if err != nil {
		return graphres.Screens{}, err
	}
	sizes, err := parsePageSizes(getenv("ALPHONE_LIST_PAGE_SIZES"), ceiling)
	if err != nil {
		return graphres.Screens{}, err
	}
	size, err := parsePositiveCount("ALPHONE_LIST_PAGE_SIZE", getenv("ALPHONE_LIST_PAGE_SIZE"),
		graphres.DefaultListPageSize)
	if err != nil {
		return graphres.Screens{}, err
	}
	if !slices.Contains(sizes, size) {
		return graphres.Screens{}, fmt.Errorf(
			"ALPHONE_LIST_PAGE_SIZE %d must be one of ALPHONE_LIST_PAGE_SIZES %v", size, sizes)
	}
	locale, err := parseFormatLocale(getenv("ALPHONE_FORMAT_LOCALE"))
	if err != nil {
		return graphres.Screens{}, err
	}
	return graphres.Screens{ToastDuration: toast, PageSizes: sizes, PageSize: size, FormatLocale: locale}, nil
}

// parseFormatLocale reads the locale dates and numbers are written in as a canonical BCP 47 tag that names a language.
func parseFormatLocale(raw string) (string, error) {
	if raw == "" {
		return graphres.DefaultFormatLocale, nil
	}
	tag, err := language.Parse(raw)
	base, _, _ := tag.Raw()
	if err != nil || base.String() == "und" {
		return "", fmt.Errorf("ALPHONE_FORMAT_LOCALE must be a BCP 47 language tag such as es-ES or en-GB, got %q", raw)
	}
	return tag.String(), nil
}

// longestToast is the longest toast a browser timer and a graph Int both hold.
const longestToast = math.MaxInt32 * time.Millisecond

// parseToastDuration reads how long a toast stays in whole milliseconds, empty applying the default.
func parseToastDuration(raw string) (time.Duration, error) {
	if raw == "" {
		return graphres.DefaultToastDuration, nil
	}
	held, err := time.ParseDuration(raw)
	if err != nil || held < time.Millisecond || held > longestToast || held%time.Millisecond != 0 {
		return 0, fmt.Errorf("ALPHONE_TOAST_DURATION must be a duration such as 6s or 1500ms, "+
			"in whole milliseconds from 1ms to %dms, got %q", longestToast.Milliseconds(), raw)
	}
	return held, nil
}

// parsePageSizes reads the page sizes a list offers, empty applying the defaults, none past ceiling.
func parsePageSizes(raw string, ceiling int) ([]int, error) {
	sizes := graphres.DefaultListPageSizes()
	if raw != "" {
		read, err := readPageSizes(raw)
		if err != nil {
			return nil, err
		}
		sizes = read
	}
	for _, size := range sizes {
		if size > ceiling {
			return nil, fmt.Errorf(
				"ALPHONE_LIST_PAGE_SIZES holds %d, past ALPHONE_GRAPH_PAGE_CAP %d", size, ceiling)
		}
	}
	return sizes, nil
}

// readPageSizes reads a comma separated list of positive page sizes, each once from the smallest up.
func readPageSizes(raw string) ([]int, error) {
	entries := strings.Split(raw, ",")
	sizes := make([]int, 0, len(entries))
	for _, entry := range entries {
		size, err := strconv.Atoi(strings.TrimSpace(entry))
		if err != nil || size <= 0 {
			return nil, fmt.Errorf("ALPHONE_LIST_PAGE_SIZES must list positive whole numbers, got %q", raw)
		}
		if len(sizes) > 0 && size <= sizes[len(sizes)-1] {
			return nil, fmt.Errorf("ALPHONE_LIST_PAGE_SIZES must list each size once from the smallest up, got %q", raw)
		}
		sizes = append(sizes, size)
	}
	return sizes, nil
}
