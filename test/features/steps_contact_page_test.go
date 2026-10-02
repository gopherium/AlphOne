// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/cucumber/godog"
)

// contactPageQuery reads one page of the contact directory through the graph.
const contactPageQuery = `query($q: String, $channels: [String!], $orderBy: ContactOrderBy!, $order: SortOrder!,
	$limit: Int, $offset: Int) {
	contactPage(q: $q, channels: $channels, orderBy: $orderBy, order: $order, limit: $limit, offset: $offset) {
		items { name identities { channel } }
		total
		limit
	}
}`

// contactPageAnswer is the envelope every contact page step reads.
type contactPageAnswer struct {
	Data struct {
		ContactPage *struct {
			Items []struct {
				Name       string `json:"name"`
				Identities []struct {
					Channel string `json:"channel"`
				} `json:"identities"`
			} `json:"items"`
			Total int `json:"total"`
			Limit int `json:"limit"`
		} `json:"contactPage"`
	} `json:"data"`
	Errors []struct {
		Message    string `json:"message"`
		Extensions struct {
			Reason string `json:"reason"`
			Meta   struct {
				Max int `json:"max"`
			} `json:"meta"`
		} `json:"extensions"`
	} `json:"errors"`
}

// pageArguments returns the contact page variables sorting by name ascending.
func pageArguments() map[string]any {
	return map[string]any{"orderBy": "NAME", "order": "ASC"}
}

// readContactPage posts the contact page query under a bearer secret with the given variables.
func (w *world) readContactPage(ctx context.Context, secret string, variables map[string]any) error {
	body, err := json.Marshal(map[string]any{"query": contactPageQuery, "variables": variables})
	if err != nil {
		return fmt.Errorf("encoding the contact page read: %w", err)
	}
	_, err = w.postGraphWith(ctx, secret, string(body))
	return err
}

// contactPage decodes the contact page the last read answered.
func (w *world) contactPage() (contactPageAnswer, error) {
	var answer contactPageAnswer
	if err := json.Unmarshal(w.answered, &answer); err != nil {
		return answer, fmt.Errorf("decoding %s: %w", w.answered, err)
	}
	if len(answer.Errors) > 0 || answer.Data.ContactPage == nil {
		return answer, fmt.Errorf("the graph answered no contact page, answered %s", w.answered)
	}
	return answer, nil
}

// listedNames returns the names the last contact page lists in order.
func (w *world) listedNames() ([]string, error) {
	answer, err := w.contactPage()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(answer.Data.ContactPage.Items))
	for _, item := range answer.Data.ContactPage.Items {
		names = append(names, item.Name)
	}
	return names, nil
}

// registerContactPageSteps binds the contact page steps and the world lifecycle.
func registerContactPageSteps(sc *godog.ScenarioContext, t *testing.T) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, worldKey{}, newWorld(t)), nil
	})

	sc.Given(`^a running AlphOne holding a user with an API token$`, func(ctx context.Context) error {
		if worldFrom(ctx).secret == "" {
			return fmt.Errorf("the scenario holds no token")
		}
		return nil
	})
	bindReachableContactStep(sc)
	bindTenantSteps(sc)
	bindContactPageGivens(sc)
	bindContactPageReads(sc)
	bindContactPageThens(sc)
}

// bindContactPageGivens binds the steps storing the contacts a page reads.
func bindContactPageGivens(sc *godog.ScenarioContext) {
	sc.Given(`^these contacts, created in this order:$`, func(ctx context.Context, table *godog.Table) error {
		w := worldFrom(ctx)
		for _, name := range tableColumn(table) {
			if _, err := w.seedContact(ctx, name); err != nil {
				return err
			}
		}
		return nil
	})
}

// bindContactPageReads binds the steps reading the contact page.
func bindContactPageReads(sc *godog.ScenarioContext) {
	sc.When(`^the caller reads the contact page$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		return w.readContactPage(ctx, w.secret, pageArguments())
	})

	sc.When(`^the caller reads the contact page (\d+) at a time from offset (\d+)$`,
		func(ctx context.Context, limit, offset int) error {
			w := worldFrom(ctx)
			variables := pageArguments()
			variables["limit"] = limit
			variables["offset"] = offset
			return w.readContactPage(ctx, w.secret, variables)
		})

	sc.When(`^the caller searches the contact page for "([^"]*)"$`, func(ctx context.Context, term string) error {
		w := worldFrom(ctx)
		variables := pageArguments()
		variables["q"] = term
		return w.readContactPage(ctx, w.secret, variables)
	})

	sc.When(`^the caller filters the contact page to the ([a-z]+) channel$`,
		func(ctx context.Context, channel string) error {
			w := worldFrom(ctx)
			variables := pageArguments()
			variables["channels"] = []string{channel}
			return w.readContactPage(ctx, w.secret, variables)
		})

	sc.When(`^the caller sorts the contact page by ([A-Z_]+) (ASC|DESC)$`,
		func(ctx context.Context, orderBy, order string) error {
			w := worldFrom(ctx)
			return w.readContactPage(ctx, w.secret, map[string]any{"orderBy": orderBy, "order": order})
		})

	sc.When(`^that token reads the contact page$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		if w.altSecret == "" {
			return fmt.Errorf("the scenario minted no second token")
		}
		return w.readContactPage(ctx, w.altSecret, pageArguments())
	})
}

// bindContactPageThens binds the steps checking the contact page answered.
func bindContactPageThens(sc *godog.ScenarioContext) {
	sc.Then(`^the page lists:$`, func(ctx context.Context, table *godog.Table) error {
		names, err := worldFrom(ctx).listedNames()
		if err != nil {
			return err
		}
		if want := tableColumn(table); !slices.Equal(names, want) {
			return fmt.Errorf("the page lists %v, want %v", names, want)
		}
		return nil
	})

	sc.Then(`^the page lists no contacts$`, func(ctx context.Context) error {
		names, err := worldFrom(ctx).listedNames()
		if err != nil {
			return err
		}
		if len(names) != 0 {
			return fmt.Errorf("the page lists %v, want none", names)
		}
		return nil
	})

	sc.Then(`^the page counts (\d+) contacts?$`, func(ctx context.Context, want int) error {
		answer, err := worldFrom(ctx).contactPage()
		if err != nil {
			return err
		}
		if got := answer.Data.ContactPage.Total; got != want {
			return fmt.Errorf("total = %d, want %d", got, want)
		}
		return nil
	})

	sc.Then(`^the page holds at most (\d+) contacts$`, func(ctx context.Context, want int) error {
		answer, err := worldFrom(ctx).contactPage()
		if err != nil {
			return err
		}
		if got := answer.Data.ContactPage.Limit; got != want {
			return fmt.Errorf("limit = %d, want %d", got, want)
		}
		return nil
	})

	sc.Then(`^the read is refused as out of range up to (\d+)$`, func(ctx context.Context, most int) error {
		w := worldFrom(ctx)
		var answer contactPageAnswer
		if err := json.Unmarshal(w.answered, &answer); err != nil {
			return fmt.Errorf("decoding %s: %w", w.answered, err)
		}
		if len(answer.Errors) == 0 {
			return fmt.Errorf("the graph answered the page, answered %s", w.answered)
		}
		refused := answer.Errors[0].Extensions
		if refused.Reason != "first_out_of_range" || refused.Meta.Max != most {
			return fmt.Errorf("refused with %q up to %d, want first_out_of_range up to %d", refused.Reason,
				refused.Meta.Max, most)
		}
		return nil
	})

	sc.Then(`^"([^"]*)" is listed reachable on ([a-z]+)$`, func(ctx context.Context, name, channel string) error {
		answer, err := worldFrom(ctx).contactPage()
		if err != nil {
			return err
		}
		for _, item := range answer.Data.ContactPage.Items {
			if item.Name != name {
				continue
			}
			for _, identity := range item.Identities {
				if identity.Channel == channel {
					return nil
				}
			}
			return fmt.Errorf("%q is listed with %+v, want the %s channel", name, item.Identities, channel)
		}
		return fmt.Errorf("the page lists no %q, answered %s", name, worldFrom(ctx).answered)
	})
}
