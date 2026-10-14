// SPDX-License-Identifier: Elastic-2.0

package importer_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

// fiveRowCSV is a comma separated export staging five contacts in file order.
const fiveRowCSV = "Name,Email\n" +
	"Row One,one@example.com\nRow Two,two@example.com\nRow Three,three@example.com\n" +
	"Row Four,four@example.com\nRow Five,five@example.com\n"

// smallPages bounds the pages small enough that five rows span several of them.
var smallPages = sdk.PageBounds{Size: 2, Cap: 3}

// pagedJob reads one page of an import's rows and contacts through the graph, answering the response.
func pagedJob(t *testing.T, graphHandler http.Handler, id, selection string) (graphImportJob, graphResponse) {
	t.Helper()
	response := decodeGraph(t, postGraph(t, graphHandler,
		`query($id: UUID!) { importJob(id: $id) { `+selection+` } }`, map[string]any{"id": id}))
	var payload struct {
		ImportJob graphImportJob
	}
	if response.Data != nil && string(response.Data) != "null" {
		if err := json.Unmarshal(response.Data, &payload); err != nil {
			t.Fatalf("decoding page payload: %v", err)
		}
	}
	return payload.ImportJob, response
}

// rowPositions returns the positions of the rows one page answered.
func rowPositions(job graphImportJob) []int {
	positions := make([]int, len(job.Rows))
	for i, row := range job.Rows {
		positions[i] = row.Position
	}
	return positions
}

// contactNames returns the names of the contacts one page answered.
func contactNames(job graphImportJob) []string {
	names := make([]string, len(job.Contacts))
	for i, contact := range job.Contacts {
		names[i] = contact.Name
	}
	return names
}

// refusedPastTheCap reports whether a response carries the first_out_of_range refusal naming the cap.
func refusedPastTheCap(t *testing.T, response graphResponse, ceiling int) {
	t.Helper()
	if len(response.Errors) == 0 {
		t.Fatal("errors are empty, want the page past the cap refused")
	}
	extensions := response.Errors[0].Extensions
	meta, _ := extensions["meta"].(map[string]any)
	if extensions["code"] != "VALIDATION" || extensions["reason"] != "first_out_of_range" {
		t.Errorf("extensions = %v, want VALIDATION first_out_of_range", extensions)
	}
	if meta["max"] != float64(ceiling) {
		t.Errorf("meta = %v, want the cap %d named", meta, ceiling)
	}
}

func TestRowsPageThroughTheStagedRowsByLimitAndOffset(t *testing.T) {
	t.Parallel()

	p, pool, _, _ := newCommittingPluginWithin(t, smallPages)
	graphHandler := newGraphHandler(t, p, pool, uuid.Must(uuid.NewV7()))
	staged := stagedUpload(t, graphHandler, fiveRowCSV)

	for name, tc := range map[string]struct {
		selection string
		want      []int
	}{
		"the host page without a limit":  {`rows { position }`, []int{1, 2}},
		"a page past an offset":          {`rows(limit: 3, offset: 2) { position }`, []int{3, 4, 5}},
		"an offset past the last row":    {`rows(limit: 3, offset: 5) { position }`, []int{}},
		"a negative offset as the first": {`rows(limit: 1, offset: -4) { position }`, []int{1}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			job, response := pagedJob(t, graphHandler, staged.ID, tc.selection)

			if len(response.Errors) != 0 {
				t.Fatalf("errors = %v, want none", response.Errors)
			}
			if got := rowPositions(job); !slices.Equal(got, tc.want) {
				t.Errorf("positions = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRowsRefuseAPagePastTheHostCap(t *testing.T) {
	t.Parallel()

	p, pool, _, _ := newCommittingPluginWithin(t, smallPages)
	graphHandler := newGraphHandler(t, p, pool, uuid.Must(uuid.NewV7()))
	staged := stagedUpload(t, graphHandler, fiveRowCSV)

	_, response := pagedJob(t, graphHandler, staged.ID, `rows(limit: 4) { position }`)

	refusedPastTheCap(t, response, smallPages.Cap)
}

func TestContactsPageThroughTheImportedContactsInRowOrder(t *testing.T) {
	t.Parallel()

	p, pool, _, _ := newCommittingPluginWithin(t, smallPages)
	graphHandler := newGraphHandler(t, p, pool, uuid.Must(uuid.NewV7()))
	staged := stagedUpload(t, graphHandler, fiveRowCSV)
	mustGraphMapping(t, graphHandler, staged.ID)
	committed := decodeGraph(t, postGraph(t, graphHandler,
		`mutation($id: UUID!) { importCommit(id: $id) { imported } }`, map[string]any{"id": staged.ID}))
	if len(committed.Errors) != 0 {
		t.Fatalf("commit errors = %v, want none", committed.Errors)
	}

	for name, tc := range map[string]struct {
		selection string
		want      []string
	}{
		"the host page without a limit": {`contacts { name }`, []string{"Row One", "Row Two"}},
		"a page past an offset":         {`contacts(limit: 3, offset: 3) { name }`, []string{"Row Four", "Row Five"}},
		"an offset past the last one":   {`contacts(limit: 3, offset: 5) { name }`, []string{}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			job, response := pagedJob(t, graphHandler, staged.ID, tc.selection)

			if len(response.Errors) != 0 {
				t.Fatalf("errors = %v, want none", response.Errors)
			}
			if got := contactNames(job); !slices.Equal(got, tc.want) {
				t.Errorf("contacts = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestContactsRefuseAPagePastTheHostCap(t *testing.T) {
	t.Parallel()

	p, pool, _, _ := newCommittingPluginWithin(t, smallPages)
	graphHandler := newGraphHandler(t, p, pool, uuid.Must(uuid.NewV7()))
	staged := stagedUpload(t, graphHandler, fiveRowCSV)

	_, response := pagedJob(t, graphHandler, staged.ID, `contacts(limit: 4) { name }`)

	refusedPastTheCap(t, response, smallPages.Cap)
}
