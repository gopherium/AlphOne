// SPDX-License-Identifier: Elastic-2.0

package apitoken

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// ErrMalformedScope reports a scope that is neither the wildcard nor an area and its access.
var ErrMalformedScope = errors.New("apitoken: malformed scope")

// ErrNoScopes reports a token asked to carry no scope at all.
var ErrNoScopes = errors.New("apitoken: no scopes")

// Wildcard is the scope granting every area for reading and writing.
const Wildcard = "*"

// readAccess names the access granting reads of an area.
const readAccess = "read"

// writeAccess names the access granting reads and writes of an area.
const writeAccess = "write"

// Scopes is the sorted set of areas a token may act in.
type Scopes []string

// Full returns the scopes granting every area, the set a legacy token holds.
func Full() Scopes {
	return Scopes{Wildcard}
}

// ParseScopes returns the canonical scopes of one stored, space separated, list.
func ParseScopes(stored string) Scopes {
	entries := strings.Fields(stored)
	if len(entries) == 0 {
		return nil
	}
	return Scopes(entries).Canonical()
}

// Canonical returns the scopes sorted and without repeats, leaving the receiver alone.
func (s Scopes) Canonical() Scopes {
	sorted := slices.Clone(s)
	slices.Sort(sorted)
	return slices.Compact(sorted)
}

// String returns the stored form, every scope joined by one space.
func (s Scopes) String() string {
	return strings.Join(s, " ")
}

// Allows reports whether the scopes admit the area at the asked access.
func (s Scopes) Allows(area string, write bool) bool {
	writing := area + ":" + writeAccess
	reading := area + ":" + readAccess
	for _, entry := range s {
		if entry == Wildcard || entry == writing {
			return true
		}
		if !write && entry == reading {
			return true
		}
	}
	return false
}

// Validate reports the first scope that is neither the wildcard nor an area and its access.
func (s Scopes) Validate() error {
	if len(s) == 0 {
		return ErrNoScopes
	}
	for _, entry := range s {
		if err := validateScope(entry); err != nil {
			return err
		}
	}
	return nil
}

// validateScope reports whether one scope reads as the wildcard or as an area and its access.
func validateScope(entry string) error {
	if strings.ContainsFunc(entry, unicode.IsSpace) {
		return fmt.Errorf("%w: %q", ErrMalformedScope, entry)
	}
	if entry == Wildcard {
		return nil
	}
	area, access, divided := strings.Cut(entry, ":")
	if !divided || area == "" || (access != readAccess && access != writeAccess) {
		return fmt.Errorf("%w: %q", ErrMalformedScope, entry)
	}
	return nil
}
