// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/graphres"
)

// adminSettingsResult is the shape the admin settings read answers.
type adminSettingsResult struct {
	AdminSettings struct {
		ToastMilliseconds int    `json:"toastMilliseconds"`
		ListPageSizes     []int  `json:"listPageSizes"`
		ListPageSize      int    `json:"listPageSize"`
		ContactPageCap    int    `json:"contactPageCap"`
		FormatLocale      string `json:"formatLocale"`
	} `json:"adminSettings"`
}

// adminSettingsDocument reads every admin setting.
const adminSettingsDocument = `{ adminSettings { toastMilliseconds listPageSizes listPageSize contactPageCap ` +
	`formatLocale } }`

func TestAdminSettingsAnswerTheConfiguredScreens(t *testing.T) {
	t.Parallel()

	resolver := newStubResolver(&stubContactStore{}, &stubTaskStore{})
	resolver.Paging = graphres.Paging{Cap: 150}
	resolver.Screens = graphres.Screens{
		ToastDuration: 9 * time.Second, PageSizes: []int{5, 15}, PageSize: 15, FormatLocale: "de-DE",
	}
	client := newGraphClient(t, resolver, uuid.Must(uuid.NewV7()))

	var answer adminSettingsResult
	client.MustPost(adminSettingsDocument, &answer)

	held := answer.AdminSettings
	if held.ToastMilliseconds != 9000 {
		t.Errorf("toastMilliseconds = %d, want the configured 9000", held.ToastMilliseconds)
	}
	if !slices.Equal(held.ListPageSizes, []int{5, 15}) || held.ListPageSize != 15 {
		t.Errorf("list pages = %v opening on %d, want [5 15] opening on 15", held.ListPageSizes, held.ListPageSize)
	}
	if held.ContactPageCap != 150 {
		t.Errorf("contactPageCap = %d, want the configured graph cap 150", held.ContactPageCap)
	}
	if held.FormatLocale != "de-DE" {
		t.Errorf("formatLocale = %q, want the configured de-DE", held.FormatLocale)
	}
}

func TestAdminSettingsFallBackToTheDefaults(t *testing.T) {
	t.Parallel()

	client := newGraphClient(t, newStubResolver(&stubContactStore{}, &stubTaskStore{}), uuid.Must(uuid.NewV7()))

	var answer adminSettingsResult
	client.MustPost(adminSettingsDocument, &answer)

	held := answer.AdminSettings
	if held.ToastMilliseconds != int(graphres.DefaultToastDuration.Milliseconds()) {
		t.Errorf("toastMilliseconds = %d, want the default %v", held.ToastMilliseconds, graphres.DefaultToastDuration)
	}
	if !slices.Equal(held.ListPageSizes, graphres.DefaultListPageSizes()) {
		t.Errorf("listPageSizes = %v, want the defaults %v", held.ListPageSizes, graphres.DefaultListPageSizes())
	}
	if held.ListPageSize != graphres.DefaultListPageSize || held.ContactPageCap != graphres.DefaultPageCap {
		t.Errorf("list opens on %d under a cap of %d, want the defaults %d and %d",
			held.ListPageSize, held.ContactPageCap, graphres.DefaultListPageSize, graphres.DefaultPageCap)
	}
	if held.FormatLocale != "es-ES" {
		t.Errorf("formatLocale = %q, want the default es-ES", held.FormatLocale)
	}
}
