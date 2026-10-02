// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	gqlclient "github.com/99designs/gqlgen/client"
	"github.com/google/uuid"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/graphres"
)

// contactPageResult is the shape a contact page read answers.
type contactPageResult struct {
	ContactPage struct {
		Items []struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			Identities []struct {
				Channel string `json:"channel"`
			} `json:"identities"`
		} `json:"items"`
		Total int `json:"total"`
		Limit int `json:"limit"`
	} `json:"contactPage"`
}

// pageNames returns the names a contact page read lists in order.
func pageNames(answer contactPageResult) []string {
	names := make([]string, 0, len(answer.ContactPage.Items))
	for _, item := range answer.ContactPage.Items {
		names = append(names, item.Name)
	}
	return names
}

func TestContactPageAnswersItsContactsTheirTotalAndItsLimit(t *testing.T) {
	t.Parallel()

	resolver, contacts, _ := newDBResolver(t)
	for _, name := range []string{"Maria Perez", "Ada Lovelace", "Grace Hopper"} {
		mustSeedContact(t, contacts, name)
	}
	client := newGraphClient(t, resolver, uuid.Must(uuid.NewV7()))

	var first, rest contactPageResult
	client.MustPost(`{ contactPage(limit: 2) { items { id name } total limit } }`, &first)
	client.MustPost(`{ contactPage(limit: 2, offset: 2) { items { id name } total limit } }`, &rest)

	if got := pageNames(first); !slices.Equal(got, []string{"Ada Lovelace", "Grace Hopper"}) {
		t.Errorf("first page = %v, want the first two in name order", got)
	}
	if got := pageNames(rest); !slices.Equal(got, []string{"Maria Perez"}) {
		t.Errorf("second page = %v, want the last one", got)
	}
	if first.ContactPage.Total != 3 || first.ContactPage.Limit != 2 || rest.ContactPage.Total != 3 {
		t.Errorf("pages count %d and %d under a limit of %d, want 3 twice under 2",
			first.ContactPage.Total, rest.ContactPage.Total, first.ContactPage.Limit)
	}
}

func TestContactPageHandsTheStoreItsSearchChannelsAndOrder(t *testing.T) {
	t.Parallel()

	contacts := &stubContactStore{total: 7}
	client := newGraphClient(t, newStubResolver(contacts, &stubTaskStore{}), uuid.Must(uuid.NewV7()))

	var answer contactPageResult
	client.MustPost(`{ contactPage(q: "184 467", channels: ["whatsapp"], orderBy: CREATED_AT, order: DESC,
		limit: 20, offset: 40) { total limit } }`, &answer)

	wantFilter := contact.Filter{Query: "184 467", Digits: "184467", Channels: []string{"whatsapp"}}
	if got := contacts.pagedFilter; got.Query != wantFilter.Query || got.Digits != wantFilter.Digits ||
		!slices.Equal(got.Channels, wantFilter.Channels) {
		t.Errorf("filter = %+v, want %+v", got, wantFilter)
	}
	if want := (contact.Page{ByCreated: true, Descending: true, Limit: 20, Offset: 40}); contacts.pagedPage != want {
		t.Errorf("page = %+v, want %+v", contacts.pagedPage, want)
	}
	if answer.ContactPage.Total != 7 || answer.ContactPage.Limit != 20 {
		t.Errorf("answer = %+v, want the store's total 7 under the limit 20", answer.ContactPage)
	}
}

func TestContactPageOpensOnTheNameInAscendingOrder(t *testing.T) {
	t.Parallel()

	contacts := &stubContactStore{}
	client := newGraphClient(t, newStubResolver(contacts, &stubTaskStore{}), uuid.Must(uuid.NewV7()))

	var answer contactPageResult
	client.MustPost(`{ contactPage { total } }`, &answer)

	if contacts.pagedPage.ByCreated || contacts.pagedPage.Descending {
		t.Errorf("page = %+v, want the name in ascending order", contacts.pagedPage)
	}
}

// pagedUnder returns a client over a stub store whose resolver pages 10 by default and 30 at most.
func pagedUnder(t *testing.T, contacts *stubContactStore) *gqlclient.Client {
	t.Helper()
	resolver := newStubResolver(contacts, &stubTaskStore{})
	resolver.Paging = graphres.Paging{Size: 10, Cap: 30}
	return newGraphClient(t, resolver, uuid.Must(uuid.NewV7()))
}

func TestContactPageFillsTheBoundsACallerLeavesOut(t *testing.T) {
	t.Parallel()

	bounds := map[string]struct {
		document string
		want     contact.Page
	}{
		"no limit":          {`{ contactPage { limit } }`, contact.Page{Limit: 10}},
		"a negative offset": {`{ contactPage(limit: 5, offset: -3) { limit } }`, contact.Page{Limit: 5}},
	}
	for name, bound := range bounds {
		contacts := &stubContactStore{}

		var answer contactPageResult
		pagedUnder(t, contacts).MustPost(bound.document, &answer)

		if contacts.pagedPage != bound.want || answer.ContactPage.Limit != bound.want.Limit {
			t.Errorf("%s pages %+v answering a limit of %d, want %+v",
				name, contacts.pagedPage, answer.ContactPage.Limit, bound.want)
		}
	}
}

func TestContactPageRefusesALimitOutsideThePaging(t *testing.T) {
	t.Parallel()

	for name, document := range map[string]string{
		"a limit past the cap": `{ contactPage(limit: 31) { limit } }`,
		"no rows at all":       `{ contactPage(limit: 0) { limit } }`,
	} {
		response, err := pagedUnder(t, &stubContactStore{}).RawPost(document)
		if err != nil {
			t.Fatalf("%s RawPost() error = %v, want nil", name, err)
		}

		reason, most, message := refusedRange(t, response.Errors)
		if reason != "first_out_of_range" || most != 30 || message != "graph: limit must be between 1 and 30" {
			t.Errorf("%s refused with %q up to %v saying %q, want first_out_of_range up to 30 naming the limit",
				name, reason, most, message)
		}
	}
}

func TestContactPageIdentitiesAreLoadedInOneBatch(t *testing.T) {
	t.Parallel()

	now := time.Now()
	rows := []contact.Contact{
		{ID: uuid.Must(uuid.NewV7()), Name: "Ada Lovelace", CreatedAt: now},
		{ID: uuid.Must(uuid.NewV7()), Name: "Grace Hopper", CreatedAt: now},
		{ID: uuid.Must(uuid.NewV7()), Name: "Maria Perez", CreatedAt: now},
	}
	maria := contact.Identity{ID: uuid.Must(uuid.NewV7()), ContactID: rows[2].ID, Channel: "whatsapp"}
	contacts := &stubContactStore{
		pageRows: rows, total: 3, identities: map[uuid.UUID][]contact.Identity{rows[2].ID: {maria}},
	}
	resolver := newStubResolver(contacts, &stubTaskStore{})
	resolver.BatchWait = 100 * time.Millisecond
	client := newGraphClient(t, resolver, uuid.Must(uuid.NewV7()))

	var answer contactPageResult
	client.MustPost(`{ contactPage { items { name identities { channel } } total } }`, &answer)

	if len(contacts.identityBatches) != 1 || len(contacts.identityBatches[0]) != 3 {
		t.Fatalf("identity batches = %v, want one batch naming the 3 listed contacts", contacts.identityBatches)
	}
	items := answer.ContactPage.Items
	if len(items) != 3 || len(items[0].Identities) != 0 || len(items[2].Identities) != 1 ||
		items[2].Identities[0].Channel != "whatsapp" {
		t.Errorf("items = %+v, want each contact with its own identities", items)
	}
}

func TestContactPageFailuresAreMaskedAsInternal(t *testing.T) {
	t.Parallel()

	failures := map[string]*stubContactStore{
		"the page":  {pageErr: errors.New("pgx: page refused")},
		"the count": {countErr: errors.New("pgx: count refused")},
	}
	for name, contacts := range failures {
		client := newGraphClient(t, newStubResolver(contacts, &stubTaskStore{}), uuid.Must(uuid.NewV7()))

		response, err := client.RawPost(`{ contactPage { total } }`)
		if err != nil {
			t.Fatalf("%s RawPost() error = %v, want nil", name, err)
		}

		if got := firstErrorCode(t, response.Errors); got != "INTERNAL" {
			t.Errorf("%s failing code = %q, want INTERNAL", name, got)
		}
	}
}

func TestContactIdentitiesNeedARequestScope(t *testing.T) {
	t.Parallel()

	resolver := newStubResolver(&stubContactStore{}, &stubTaskStore{})

	_, err := resolver.ContactResolvers().Identities(context.Background(), &model.Contact{ID: uuid.Must(uuid.NewV7())})

	if err == nil {
		t.Error("Identities() without a request scope error = nil, want error")
	}
}

func TestANegativeContactPageLimitCannotLowerThePrice(t *testing.T) {
	t.Parallel()

	honest := operationCost(t, `{ contactPage { items { id name } } }`)
	negative := operationCost(t, `{ contactPage(limit: -100000) { items { id name } } }`)

	if negative < honest {
		t.Errorf("a negative limit prices %d, want at least the default page's %d", negative, honest)
	}
	if priced := operationCost(t, `{ contactPage(limit: 200) { items { id name } } }`); priced <= honest {
		t.Errorf("a 200 row page prices %d, want above the default page's %d", priced, honest)
	}
}
