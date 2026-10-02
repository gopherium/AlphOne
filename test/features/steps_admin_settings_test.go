// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

// adminSettingsQuery reads the settings the admin screens read once.
const adminSettingsQuery = `{"query":"{ adminSettings {` +
	` toastMilliseconds listPageSizes listPageSize contactPageCap formatLocale } }"}`

// adminSettingsAnswer is the envelope every admin settings step reads.
type adminSettingsAnswer struct {
	Data struct {
		AdminSettings *struct {
			ToastMilliseconds int    `json:"toastMilliseconds"`
			ListPageSizes     []int  `json:"listPageSizes"`
			ListPageSize      int    `json:"listPageSize"`
			ContactPageCap    int    `json:"contactPageCap"`
			FormatLocale      string `json:"formatLocale"`
		} `json:"adminSettings"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// adminSettings decodes the admin settings the last read answered.
func (w *world) adminSettings() (adminSettingsAnswer, error) {
	var answer adminSettingsAnswer
	if err := json.Unmarshal(w.answered, &answer); err != nil {
		return answer, fmt.Errorf("decoding %s: %w", w.answered, err)
	}
	if len(answer.Errors) > 0 || answer.Data.AdminSettings == nil {
		return answer, fmt.Errorf("the graph answered no admin settings, answered %s", w.answered)
	}
	return answer, nil
}

// spokenSizes reads a list of sizes written as "10, 20, 50 and 100".
func spokenSizes(spoken string) ([]int, error) {
	parts := strings.FieldsFunc(strings.ReplaceAll(spoken, " and ", ","), func(r rune) bool {
		return r == ',' || r == ' '
	})
	sizes := make([]int, 0, len(parts))
	for _, part := range parts {
		size, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("reading %q: %w", spoken, err)
		}
		sizes = append(sizes, size)
	}
	return sizes, nil
}

// registerAdminSettingsSteps binds the admin settings steps and the world lifecycle.
func registerAdminSettingsSteps(sc *godog.ScenarioContext, t *testing.T) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, worldKey{}, newWorld(t)), nil
	})

	sc.Given(`^a running AlphOne holding a user with an API token$`, func(ctx context.Context) error {
		if worldFrom(ctx).secret == "" {
			return fmt.Errorf("the scenario holds no token")
		}
		return nil
	})
	bindTenantSteps(sc)
	bindAdminSettingsSteps(sc)
}

// bindAdminSettingsSteps binds the steps reading and checking the admin settings.
func bindAdminSettingsSteps(sc *godog.ScenarioContext) {
	sc.When(`^the caller reads the admin settings$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		_, err := w.postGraphWith(ctx, w.secret, adminSettingsQuery)
		return err
	})

	sc.When(`^an anonymous caller reads the admin settings$`, func(ctx context.Context) error {
		_, err := worldFrom(ctx).postGraphWith(ctx, "", adminSettingsQuery)
		return err
	})

	sc.Then(`^a toast stays (\d+) milliseconds$`, func(ctx context.Context, want int) error {
		answer, err := worldFrom(ctx).adminSettings()
		if err != nil {
			return err
		}
		if got := answer.Data.AdminSettings.ToastMilliseconds; got != want {
			return fmt.Errorf("toastMilliseconds = %d, want %d", got, want)
		}
		return nil
	})

	sc.Then(`^a list offers the page sizes ([0-9, and]+)$`, func(ctx context.Context, spoken string) error {
		answer, err := worldFrom(ctx).adminSettings()
		if err != nil {
			return err
		}
		want, err := spokenSizes(spoken)
		if err != nil {
			return err
		}
		if got := answer.Data.AdminSettings.ListPageSizes; !slices.Equal(got, want) {
			return fmt.Errorf("listPageSizes = %v, want %v", got, want)
		}
		return nil
	})

	sc.Then(`^a list opens on (\d+) rows$`, func(ctx context.Context, want int) error {
		answer, err := worldFrom(ctx).adminSettings()
		if err != nil {
			return err
		}
		if got := answer.Data.AdminSettings.ListPageSize; got != want {
			return fmt.Errorf("listPageSize = %d, want %d", got, want)
		}
		return nil
	})

	sc.Then(`^a contact page holds at most (\d+) contacts$`, func(ctx context.Context, want int) error {
		answer, err := worldFrom(ctx).adminSettings()
		if err != nil {
			return err
		}
		if got := answer.Data.AdminSettings.ContactPageCap; got != want {
			return fmt.Errorf("contactPageCap = %d, want %d", got, want)
		}
		return nil
	})

	sc.Then(`^dates, times, numbers and money are written in (\S+)$`, func(ctx context.Context, want string) error {
		answer, err := worldFrom(ctx).adminSettings()
		if err != nil {
			return err
		}
		if got := answer.Data.AdminSettings.FormatLocale; got != want {
			return fmt.Errorf("formatLocale = %q, want %q", got, want)
		}
		return nil
	})
}
