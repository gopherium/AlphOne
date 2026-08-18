// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/gopherium/alphone/internal/apitoken"
)

// declaredAreas names the areas the built schema declares, read once on first use.
var declaredAreas = sync.OnceValue(func() map[string]bool {
	return NewScopeMap(ExecutableSchema(nil).Schema()).Areas()
})

// Areas names every area the mapped root fields declare.
func (m ScopeMap) Areas() map[string]bool {
	areas := make(map[string]bool, len(m))
	for _, needed := range m {
		areas[needed.area] = true
	}
	return areas
}

// DeclaredAreas names every area the built schema declares, in order.
func DeclaredAreas() []string {
	return slices.Sorted(maps.Keys(declaredAreas()))
}

// ValidateScopes refuses a grant that is malformed or names an area no schema declares.
func ValidateScopes(granted apitoken.Scopes) error {
	if err := granted.Validate(); err != nil {
		return err
	}
	known := declaredAreas()
	for _, entry := range granted {
		if area := apitoken.AreaOf(entry); area != "" && !known[area] {
			return fmt.Errorf("%w: %q", apitoken.ErrUnknownArea, area)
		}
	}
	return nil
}
