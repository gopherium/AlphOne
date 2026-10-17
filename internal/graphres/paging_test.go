// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	gqlclient "github.com/99designs/gqlgen/client"
	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/task"
)

// refusedRange reads the reason, the upper bound and the message of the first error in a raw response.
func refusedRange(t *testing.T, raw json.RawMessage) (string, float64, string) {
	t.Helper()
	var parsed []struct {
		Message    string `json:"message"`
		Extensions struct {
			Reason string `json:"reason"`
			Meta   struct {
				Max float64 `json:"max"`
			} `json:"meta"`
		} `json:"extensions"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || len(parsed) == 0 {
		t.Fatalf("no errors in response: %s (%v)", raw, err)
	}
	return parsed[0].Extensions.Reason, parsed[0].Extensions.Meta.Max, parsed[0].Message
}

func TestAConfiguredCapRefusesAConnectionPageAboveIt(t *testing.T) {
	t.Parallel()

	resolver := newStubResolver(&stubContactStore{}, &stubTaskStore{})
	resolver.Paging = graphres.Paging{Cap: 5}
	client := newGraphClient(t, resolver, uuid.Must(uuid.NewV7()))

	for name, doc := range map[string]string{
		"contacts": `{ contacts(first: 6) { pageInfo { hasNextPage } } }`,
		"tasks":    `{ tasks(date: "2026-08-06", first: 6) { pageInfo { hasNextPage } } }`,
	} {
		response, err := client.RawPost(doc)
		if err != nil {
			t.Fatalf("%s RawPost() error = %v, want nil", name, err)
		}
		reason, most, message := refusedRange(t, response.Errors)
		if reason != "first_out_of_range" || most != 5 {
			t.Errorf("%s refused with %q up to %v, want first_out_of_range up to the configured 5", name, reason, most)
		}
		if message != "graph: first must be between 1 and 5" {
			t.Errorf("%s message = %q, want the configured cap named", name, message)
		}
	}
}

func TestAConfiguredSizeAnswersAConnectionAskedForNoSize(t *testing.T) {
	t.Parallel()

	contacts := &stubContactStore{}
	resolver := newStubResolver(contacts, &stubTaskStore{})
	resolver.Paging = graphres.Paging{Size: 7}
	client := newGraphClient(t, resolver, uuid.Must(uuid.NewV7()))

	var page struct {
		Contacts connectionResult `json:"contacts"`
	}
	client.MustPost(`{ contacts { pageInfo { hasNextPage } } }`, &page)

	if contacts.listedLimit != 8 {
		t.Errorf("rows asked of the store = %d, want the configured 7 and one probe row", contacts.listedLimit)
	}
}

func TestAnUnconfiguredResolverBoundsPagesByTheDefaults(t *testing.T) {
	t.Parallel()

	contacts := &stubContactStore{}
	client := newGraphClient(t, newStubResolver(contacts, &stubTaskStore{}), uuid.Must(uuid.NewV7()))

	var page struct {
		Contacts connectionResult `json:"contacts"`
	}
	client.MustPost(`{ contacts { pageInfo { hasNextPage } } }`, &page)
	if contacts.listedLimit != graphres.DefaultPageSize+1 {
		t.Errorf("rows asked of the store = %d, want the default %d and one probe row",
			contacts.listedLimit, graphres.DefaultPageSize)
	}

	response, err := client.RawPost(fmt.Sprintf(`{ contacts(first: %d) { pageInfo { hasNextPage } } }`,
		graphres.DefaultPageCap+1))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if _, most, _ := refusedRange(t, response.Errors); most != graphres.DefaultPageCap {
		t.Errorf("refused up to %v, want the default cap %d", most, graphres.DefaultPageCap)
	}
}

// maxWalkedPages bounds every walk through a connection.
const maxWalkedPages = 10

// taskDayPage reads one page of the open tasks due on 2026-08-06.
const taskDayPage = `query($first: Int, $after: String) {
	tasks(date: "2026-08-06", first: $first, after: $after) {
		edges { node { title dueTime } }
		pageInfo { hasNextPage endCursor }
	}
}`

// walkTaskDay reads every open task due on 2026-08-06 through the connection, size tasks a page.
func walkTaskDay(t *testing.T, client *gqlclient.Client, size int) []string {
	t.Helper()
	var walked []string
	var after *string
	for range maxWalkedPages {
		var page struct {
			Tasks struct {
				Edges []struct {
					Node struct {
						Title   string  `json:"title"`
						DueTime *string `json:"dueTime"`
					} `json:"node"`
				} `json:"edges"`
				PageInfo struct {
					HasNextPage bool    `json:"hasNextPage"`
					EndCursor   *string `json:"endCursor"`
				} `json:"pageInfo"`
			} `json:"tasks"`
		}
		if err := client.Post(taskDayPage, &page, gqlclient.Var("first", size), gqlclient.Var("after", after)); err != nil {
			t.Fatalf("tasks(first: %d) error = %v, want nil", size, err)
		}
		for _, edge := range page.Tasks.Edges {
			walked = append(walked, fmt.Sprintf("%s (%s)", edge.Node.Title, answeredTime(edge.Node.DueTime)))
		}
		if !page.Tasks.PageInfo.HasNextPage {
			return walked
		}
		after = page.Tasks.PageInfo.EndCursor
	}
	t.Fatalf("the walk at %d a page passed %d pages, walked %v", size, maxWalkedPages, walked)
	return nil
}

// mustClock returns the time of day text spells.
func mustClock(t *testing.T, text string) task.Clock {
	t.Helper()
	clock, err := task.ParseClock(text)
	if err != nil {
		t.Fatalf("ParseClock(%q) error = %v, want nil", text, err)
	}
	return clock
}

func TestTasksWalkTheTimedWorkOfADayBeforeTheUntimedWorkThroughTheCursor(t *testing.T) {
	t.Parallel()

	resolver, _, tasks := newDBResolver(t)
	assignee := uuid.Must(uuid.NewV7())
	day := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	for _, seeded := range []task.Input{
		{Title: "No time first"},
		{Title: "At eleven", DueTime: mustClock(t, "11:00")},
		{Title: "At midnight", DueTime: mustClock(t, "00:00")},
		{Title: "No time second"},
		{Title: "At nine", DueTime: mustClock(t, "09:00")},
		{Title: "Raised with no time", Priority: 1},
	} {
		seeded.DueOn, seeded.AssigneeID = day, assignee
		mustSeedTask(t, tasks, seeded)
	}
	client := newGraphClient(t, resolver, assignee)
	want := []string{
		"Raised with no time (none)",
		"At midnight (00:00)",
		"At nine (09:00)",
		"At eleven (11:00)",
		"No time first (none)",
		"No time second (none)",
	}

	for _, size := range []int{1, 2, 4} {
		if walked := walkTaskDay(t, client, size); !slices.Equal(walked, want) {
			t.Errorf("walked %d a page = %v, want %v", size, walked, want)
		}
	}
}
