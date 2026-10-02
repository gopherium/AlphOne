// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/graphres"
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
