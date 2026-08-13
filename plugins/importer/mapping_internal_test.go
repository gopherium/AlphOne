// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"strings"
	"testing"
)

// coreOnly returns the registry a plugin serving no provider answers with.
func coreOnly() registry {
	return registry{fields: coreFields, owner: map[fieldName]int{}}
}

func TestBuildMappingRefusesAnUnusableSet(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		assignments []assignment
		want        string
	}{
		"a column the import does not carry": {
			assignments: []assignment{{Column: 7, Field: fieldContactName}},
			want:        "carries no column 7",
		},
		"a field the registry does not carry": {
			assignments: []assignment{{Column: 0, Field: "nickname"}},
			want:        `carries no field "nickname"`,
		},
		"two assignments on one column": {
			assignments: []assignment{
				{Column: 0, Field: fieldContactName},
				{Column: 0, Field: fieldEmail},
			},
			want: "two assignments claim column 0",
		},
		"two assignments on one field": {
			assignments: []assignment{
				{Column: 0, Field: fieldContactName},
				{Column: 1, Field: fieldContactName},
			},
			want: `two assignments claim the field "name"`,
		},
		"nothing claiming the required field": {
			assignments: []assignment{{Column: 0, Field: fieldEmail}},
			want:        errRequiredFieldUnmapped.Error(),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assigned, err := buildMapping(tc.assignments, 2, coreOnly())

			if assigned != nil {
				t.Errorf("mapping = %v, want nil for a refused set", assigned)
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want one naming %q", err, tc.want)
			}
		})
	}
}

func TestBuildMappingKeysByColumn(t *testing.T) {
	t.Parallel()

	assigned, err := buildMapping([]assignment{
		{Column: 1, Field: fieldEmail},
		{Column: 0, Field: fieldContactName},
	}, 2, coreOnly())

	if err != nil {
		t.Fatalf("buildMapping() error = %v, want nil", err)
	}
	if assigned["0"] != fieldContactName || assigned["1"] != fieldEmail {
		t.Errorf("mapping = %v, want each field under its column index", assigned)
	}
}
