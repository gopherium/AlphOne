// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCellAtIgnoresAnIndexTheRowCannotAnswer(t *testing.T) {
	t.Parallel()

	cells := []string{"Maria Perez", "maria@example.com"}

	tests := map[string]string{
		"past the row": "9",
		"negative":     "-1",
		"not a number": "first",
	}
	for name, index := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := cellAt(cells, index); got != "" {
				t.Errorf("cellAt(%q) = %q, want an empty string", index, got)
			}
		})
	}
}

func TestDraftOfSplitsTheNameFromTheIdentities(t *testing.T) {
	t.Parallel()

	cells := []string{"Maria Perez", "maria@example.com", ""}
	assigned := mapping{"0": fieldContactName, "1": fieldEmail, "2": fieldPhone}

	got := draftOf(cells, assigned)

	if got.name != "Maria Perez" {
		t.Errorf("name = %q, want Maria Perez", got.name)
	}
	if len(got.identities) != 1 {
		t.Fatalf("identities = %d, want the blank phone left out", len(got.identities))
	}
	if diff := cmp.Diff("maria@example.com", got.identities[0].Identifier); diff != "" {
		t.Errorf("identifier mismatch (-want +got):\n%s", diff)
	}
	if string(got.identities[0].Channel) != string(fieldEmail) {
		t.Errorf("channel = %q, want %q", got.identities[0].Channel, fieldEmail)
	}
}
