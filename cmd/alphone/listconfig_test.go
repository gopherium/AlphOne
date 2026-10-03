// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gopherium/alphone/internal/graphres"
)

func TestTheScreenSettingsFallBackToTheirDefaults(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL": "postgres://localhost/x",
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	screens := held.lists.screens
	if screens.ToastDuration != graphres.DefaultToastDuration || screens.PageSize != graphres.DefaultListPageSize {
		t.Errorf("screens = %+v, want the default toast time and list page", screens)
	}
	if !slices.Equal(screens.PageSizes, graphres.DefaultListPageSizes()) {
		t.Errorf("page sizes = %v, want the defaults %v", screens.PageSizes, graphres.DefaultListPageSizes())
	}
}

func TestTheScreenSettingsAreReadFromTheEnvironment(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL":    "postgres://localhost/x",
		"ALPHONE_TOAST_DURATION":  "9s",
		"ALPHONE_LIST_PAGE_SIZES": "5, 15,30",
		"ALPHONE_LIST_PAGE_SIZE":  "15",
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	screens := held.lists.screens
	if screens.ToastDuration != 9*time.Second || screens.PageSize != 15 {
		t.Errorf("screens = %+v, want toasts for 9s and lists opening on 15", screens)
	}
	if !slices.Equal(screens.PageSizes, []int{5, 15, 30}) {
		t.Errorf("page sizes = %v, want [5 15 30]", screens.PageSizes)
	}
}

func TestTheScreenSettingsRefuseAnUnreadableValue(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		named string
		sizes string
		size  string
		toast string
	}{
		"no toast at all":              {named: "ALPHONE_TOAST_DURATION", toast: "0s"},
		"a toast in words":             {named: "ALPHONE_TOAST_DURATION", toast: "a while"},
		"a toast under a millisecond":  {named: "ALPHONE_TOAST_DURATION", toast: "500us"},
		"a toast in part milliseconds": {named: "ALPHONE_TOAST_DURATION", toast: "1500us"},
		"a toast past the timer range": {named: "ALPHONE_TOAST_DURATION", toast: "2147483648ms"},
		"a toast without a unit":       {named: "ALPHONE_TOAST_DURATION", toast: "6000"},
		"an empty page size":           {named: "ALPHONE_LIST_PAGE_SIZES", sizes: "10,,20", size: "10"},
		"a page size in words":         {named: "ALPHONE_LIST_PAGE_SIZES", sizes: "10,twenty", size: "10"},
		"a page of no rows":            {named: "ALPHONE_LIST_PAGE_SIZES", sizes: "0,10", size: "10"},
		"a page size twice":            {named: "ALPHONE_LIST_PAGE_SIZES", sizes: "10,20,20", size: "10"},
		"page sizes out of order":      {named: "ALPHONE_LIST_PAGE_SIZES", sizes: "20,10,50", size: "10"},
		"a page past the graph cap":    {named: "ALPHONE_GRAPH_PAGE_CAP", sizes: "10,300", size: "10"},
		"an opening page in words":     {named: "ALPHONE_LIST_PAGE_SIZE", size: "twenty"},
		"an opening page not held":     {named: "ALPHONE_LIST_PAGE_SIZE", size: "25"},
	}
	for testName, tt := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			_, err := loadRunConfig(testGetenv(map[string]string{
				"ALPHONE_DATABASE_URL":    "postgres://localhost/x",
				"ALPHONE_TOAST_DURATION":  tt.toast,
				"ALPHONE_LIST_PAGE_SIZES": tt.sizes,
				"ALPHONE_LIST_PAGE_SIZE":  tt.size,
			}))

			if err == nil || !strings.Contains(err.Error(), tt.named) {
				t.Errorf("loadRunConfig() error = %v, want the value refused naming %s", err, tt.named)
			}
		})
	}
}

func TestTheFormatLocaleFallsBackToTheEuropeanDefault(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL": "postgres://localhost/x",
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	if got := held.lists.screens.FormatLocale; got != "es-ES" {
		t.Errorf("format locale = %q, want the default es-ES", got)
	}
}

func TestTheFormatLocaleIsReadFromTheEnvironmentInItsCanonicalForm(t *testing.T) {
	t.Parallel()

	for raw, want := range map[string]string{
		"de-DE":           "de-DE",
		"en-gb":           "en-GB",
		"es_ES":           "es-ES",
		"en-GB-u-nu-latn": "en-GB-u-nu-latn",
		"en-x-foo":        "en-x-foo",
	} {
		held, err := loadRunConfig(testGetenv(map[string]string{
			"ALPHONE_DATABASE_URL":  "postgres://localhost/x",
			"ALPHONE_FORMAT_LOCALE": raw,
		}))

		if err != nil || held.lists.screens.FormatLocale != want {
			t.Errorf("ALPHONE_FORMAT_LOCALE=%s holds %q with error %v, want %q", raw,
				held.lists.screens.FormatLocale, err, want)
		}
	}
}

func TestAFormatLocaleThatIsNoLanguageTagIsRefusedByName(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"not a locale", "xx-YY", "und", "es-ES-", "x-foo", "und-x-i-enochian", "und-ES"} {
		_, err := loadRunConfig(testGetenv(map[string]string{
			"ALPHONE_DATABASE_URL":  "postgres://localhost/x",
			"ALPHONE_FORMAT_LOCALE": raw,
		}))

		want := `ALPHONE_FORMAT_LOCALE: must be a BCP 47 language tag such as es-ES or en-GB, got "` + raw + `"`
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ALPHONE_FORMAT_LOCALE=%q error = %v, want %q", raw, err, want)
		}
	}
}

func TestTheToastDurationHoldsFromOneMillisecondToTheLargestTimer(t *testing.T) {
	t.Parallel()

	for raw, want := range map[string]time.Duration{
		"1ms":          time.Millisecond,
		"2147483647ms": math.MaxInt32 * time.Millisecond,
	} {
		held, err := loadRunConfig(testGetenv(map[string]string{
			"ALPHONE_DATABASE_URL":   "postgres://localhost/x",
			"ALPHONE_TOAST_DURATION": raw,
		}))

		if err != nil || held.lists.screens.ToastDuration != want {
			t.Errorf("ALPHONE_TOAST_DURATION=%s holds %v with error %v, want %v", raw,
				held.lists.screens.ToastDuration, err, want)
		}
	}
}

func TestAnUnreadableToastDurationIsRefusedWithItsBounds(t *testing.T) {
	t.Parallel()

	_, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL":   "postgres://localhost/x",
		"ALPHONE_TOAST_DURATION": "500us",
	}))

	want := `ALPHONE_TOAST_DURATION: must be a duration such as 6s or 1500ms, ` +
		`in whole milliseconds from 1ms to 2147483647ms, got "500us"`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("loadRunConfig() error = %v, want %q", err, want)
	}
}

func TestPageSizesListedTwiceOrOutOfOrderAreRefusedByTheRule(t *testing.T) {
	t.Parallel()

	_, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL":    "postgres://localhost/x",
		"ALPHONE_LIST_PAGE_SIZES": "20,10",
		"ALPHONE_LIST_PAGE_SIZE":  "10",
	}))

	want := `ALPHONE_LIST_PAGE_SIZES: must list each size once from the smallest up, got "20,10"`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("loadRunConfig() error = %v, want %q", err, want)
	}
}

func TestABoundSetByAnotherSettingIsRefusedNamingBoth(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		set  map[string]string
		want string
	}{
		"a graph page past the graph cap": {
			set:  map[string]string{"ALPHONE_GRAPH_PAGE_SIZE": "60", "ALPHONE_GRAPH_PAGE_CAP": "40"},
			want: "ALPHONE_GRAPH_PAGE_SIZE: must not exceed ALPHONE_GRAPH_PAGE_CAP 40, got 60",
		},
		"a list page past the graph cap": {
			set: map[string]string{
				"ALPHONE_GRAPH_PAGE_CAP": "250", "ALPHONE_LIST_PAGE_SIZES": "10,300", "ALPHONE_LIST_PAGE_SIZE": "10",
			},
			want: "ALPHONE_LIST_PAGE_SIZES: must hold no size past ALPHONE_GRAPH_PAGE_CAP 250, got 300",
		},
		"an opening page the list does not offer": {
			set:  map[string]string{"ALPHONE_LIST_PAGE_SIZES": "5,15,30", "ALPHONE_LIST_PAGE_SIZE": "25"},
			want: "ALPHONE_LIST_PAGE_SIZE: must be one of ALPHONE_LIST_PAGE_SIZES [5 15 30], got 25",
		},
	}
	for testName, tt := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			env := map[string]string{"ALPHONE_DATABASE_URL": "postgres://localhost/x"}
			maps.Copy(env, tt.set)

			_, err := loadRunConfig(testGetenv(env))

			if err == nil || err.Error() != tt.want {
				t.Errorf("loadRunConfig() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestTheGraphPageBoundsFallBackToTheirDefaults(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL": "postgres://localhost/x",
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	want := graphres.Paging{Size: graphres.DefaultPageSize, Cap: graphres.DefaultPageCap}
	if held.lists.paging != want {
		t.Errorf("paging = %+v, want the graph defaults %+v", held.lists.paging, want)
	}
}

func TestTheGraphPageBoundsAreReadFromTheEnvironment(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL":    "postgres://localhost/x",
		"ALPHONE_GRAPH_PAGE_SIZE": "25",
		"ALPHONE_GRAPH_PAGE_CAP":  "400",
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	if want := (graphres.Paging{Size: 25, Cap: 400}); held.lists.paging != want {
		t.Errorf("paging = %+v, want %+v", held.lists.paging, want)
	}
}

func TestTheGraphPageCapHoldsExactlyAtTheLargestPricedPage(t *testing.T) {
	t.Parallel()

	largest := graphres.LargestPricedPage()
	held, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL":   "postgres://localhost/x",
		"ALPHONE_GRAPH_PAGE_CAP": strconv.Itoa(largest),
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want the cap at the largest priced page accepted", err)
	}
	if held.lists.paging.Cap != largest {
		t.Errorf("paging cap = %d, want the largest priced page %d", held.lists.paging.Cap, largest)
	}
}

func TestAGraphPageCapPastTheLargestPricedPageIsRefusedWithItsBound(t *testing.T) {
	t.Parallel()

	largest := graphres.LargestPricedPage()
	_, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL":   "postgres://localhost/x",
		"ALPHONE_GRAPH_PAGE_CAP": strconv.Itoa(largest + 1),
	}))

	want := "ALPHONE_GRAPH_PAGE_CAP: must not exceed " + strconv.Itoa(largest) +
		", the most rows a read of one field a row fits under the query cost limit " +
		strconv.Itoa(graphres.ComplexityLimit) + ", got " + strconv.Itoa(largest+1)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("loadRunConfig() error = %v, want %q", err, want)
	}
}

func TestTheGraphPageBoundsRefuseAnUnreadableValue(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		name string
		raw  string
	}{
		"no rows a page":       {"ALPHONE_GRAPH_PAGE_SIZE", "0"},
		"a page size in words": {"ALPHONE_GRAPH_PAGE_SIZE", "fifty"},
		"a page past the cap":  {"ALPHONE_GRAPH_PAGE_SIZE", "201"},
		"a negative cap":       {"ALPHONE_GRAPH_PAGE_CAP", "-1"},
		"a cap in words":       {"ALPHONE_GRAPH_PAGE_CAP", "many"},
		"a cap under the page": {"ALPHONE_GRAPH_PAGE_CAP", "49"},
		"a cap past the largest priced page": {
			"ALPHONE_GRAPH_PAGE_CAP", strconv.Itoa(graphres.LargestPricedPage() + 1),
		},
	}
	for testName, tt := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			_, err := loadRunConfig(testGetenv(map[string]string{
				"ALPHONE_DATABASE_URL": "postgres://localhost/x",
				tt.name:                tt.raw,
			}))

			if err == nil || !strings.Contains(err.Error(), tt.name) {
				t.Errorf("loadRunConfig() with %s=%q error = %v, want the value refused by name", tt.name, tt.raw, err)
			}
		})
	}
}
