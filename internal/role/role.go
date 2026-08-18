// SPDX-License-Identifier: Elastic-2.0

// Package role narrows what a user may do, where a scope narrows what a token carries.
package role

import (
	"errors"
	"fmt"
	"strings"
)

// ErrLastAdmin reports a change that would leave the deployment with no enabled admin.
var ErrLastAdmin = errors.New("the last admin cannot be unseated")

// ErrUnknownTier reports a tier no deployment knows.
var ErrUnknownTier = errors.New("unknown tier")

// Role is the tier a user stands in.
type Role string

// The tiers a user may stand in.
const (
	// Admin manages users beside everything a member reaches.
	Admin Role = "admin"
	// Member works the product without reaching user management.
	Member Role = "member"
)

// Of returns the tier the stored text names, member for anything it cannot read.
func Of(stored string) Role {
	if Role(stored) == Admin {
		return Admin
	}
	return Member
}

// Tiers returns every tier in its stored form.
func Tiers() []string {
	return []string{string(Admin), string(Member)}
}

// Parse returns the tier the text names, refusing any tier no deployment knows.
func Parse(text string) (Role, error) {
	for _, tier := range Tiers() {
		if text == tier {
			return Role(tier), nil
		}
	}
	return "", fmt.Errorf("%w: %q, want one of %s", ErrUnknownTier, text, strings.Join(Tiers(), " or "))
}

// String returns the stored form of the tier.
func (r Role) String() string {
	return string(r)
}

// Allows reports whether the tier reaches a field, given whether that field is admin only.
func (r Role) Allows(adminOnly bool) bool {
	return !adminOnly || r == Admin
}
