// SPDX-License-Identifier: Elastic-2.0

// Package role narrows what a user may do, where a scope narrows what a token carries.
package role

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
)

// ErrLastAdmin reports a change that would leave the deployment with no enabled admin.
var ErrLastAdmin = errors.New("the last admin cannot be unseated")

// ErrUnknownTier reports a tier no deployment knows.
var ErrUnknownTier = errors.New("unknown tier")

// ErrEmptyRole reports a role declared with no name.
var ErrEmptyRole = errors.New("empty role")

// Role is the tier a user stands in.
type Role string

// The tiers the core declares.
const (
	// Admin manages users beside everything a member reaches.
	Admin Role = "admin"
	// Member works the product without reaching user management.
	Member Role = "member"
)

// Capability is a named permission a decision point asks for.
type Capability string

// ManageUsers is the capability administering accounts.
const ManageUsers Capability = "manage_users"

// Registry holds every role a deployment knows and the capabilities each carries.
type Registry struct {
	mu      sync.RWMutex
	carried map[Role][]Capability
}

// NewRegistry returns a registry holding the core roles and nothing a plugin declares.
func NewRegistry() *Registry {
	return &Registry{carried: map[Role][]Capability{Admin: {ManageUsers}, Member: {}}}
}

// Default is the registry the deployment reads, which the host fills at wiring.
var Default = NewRegistry()

// Grant gives a role the capabilities, declaring the role when the registry does not hold it.
func (r *Registry) Grant(held Role, capabilities ...Capability) error {
	if held == "" {
		return ErrEmptyRole
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	carried := r.carried[held]
	for _, capability := range capabilities {
		if !slices.Contains(carried, capability) {
			carried = append(carried, capability)
		}
	}
	r.carried[held] = carried
	return nil
}

// Can reports whether a role holds the capability, an unknown role holding none.
func (r *Registry) Can(held Role, capability Capability) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Contains(r.carried[held], capability)
}

// CapabilitiesOf returns the capabilities a role carries, named for a caller outside this package.
func (r *Registry) CapabilitiesOf(held Role) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	named := make([]string, 0, len(r.carried[held]))
	for _, capability := range r.carried[held] {
		named = append(named, string(capability))
	}
	return named
}

// Roles returns every role the registry holds, in stored order.
func (r *Registry) Roles() []Role {
	r.mu.RLock()
	defer r.mu.RUnlock()
	held := make([]Role, 0, len(r.carried))
	for name := range r.carried {
		held = append(held, name)
	}
	slices.Sort(held)
	return held
}

// Parse returns the role the text names, refusing any role the registry does not hold.
func (r *Registry) Parse(text string) (Role, error) {
	roles := r.Roles()
	if slices.Contains(roles, Role(text)) {
		return Role(text), nil
	}
	named := make([]string, 0, len(roles))
	for _, held := range roles {
		named = append(named, string(held))
	}
	return "", fmt.Errorf("%w: %q, want one of %s", ErrUnknownTier, text, strings.Join(named, " or "))
}

// Privileged returns the stored form of every role that administers accounts.
func (r *Registry) Privileged() []string {
	var named []string
	for _, held := range r.Roles() {
		if r.Can(held, ManageUsers) {
			named = append(named, string(held))
		}
	}
	return named
}

// Outranks reports whether the caller holds every capability the target holds.
func (r *Registry) Outranks(caller, target Role) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, capability := range r.carried[target] {
		if !slices.Contains(r.carried[caller], capability) {
			return false
		}
	}
	return true
}

// Grantable returns the roles the caller outranks, the one holding most first.
func (r *Registry) Grantable(caller Role) []Role {
	var granted []Role
	for _, held := range r.Roles() {
		if r.Outranks(caller, held) {
			granted = append(granted, held)
		}
	}
	slices.SortStableFunc(granted, func(a, b Role) int {
		return cmp.Compare(len(r.CapabilitiesOf(b)), len(r.CapabilitiesOf(a)))
	})
	return granted
}

// Grant gives a role the capabilities in the default registry, declaring it when absent.
func Grant(held Role, capabilities ...Capability) error {
	return Default.Grant(held, capabilities...)
}

// Can reports whether a role holds the capability in the default registry.
func Can(held Role, capability Capability) bool {
	return Default.Can(held, capability)
}

// CapabilitiesOf returns the capabilities a role carries in the default registry.
func CapabilitiesOf(held Role) []string {
	return Default.CapabilitiesOf(held)
}

// Privileged returns every role administering accounts in the default registry.
func Privileged() []string {
	return Default.Privileged()
}

// Outranks reports whether the caller holds every capability the target holds in the default registry.
func Outranks(caller, target Role) bool {
	return Default.Outranks(caller, target)
}

// Grantable returns the roles the caller outranks in the default registry.
func Grantable(caller Role) []Role {
	return Default.Grantable(caller)
}

// Of returns the tier the stored text names, member for anything it cannot read.
func Of(stored string) Role {
	if Role(stored) == Admin {
		return Admin
	}
	return Member
}

// Tiers returns every tier the default registry holds in its stored form.
func Tiers() []string {
	roles := Default.Roles()
	named := make([]string, 0, len(roles))
	for _, held := range roles {
		named = append(named, string(held))
	}
	return named
}

// Parse returns the tier the text names, refusing any tier the default registry does not hold.
func Parse(text string) (Role, error) {
	return Default.Parse(text)
}

// String returns the stored form of the tier.
func (r Role) String() string {
	return string(r)
}

// Allows reports whether the tier reaches a field, given whether that field is admin only.
func (r Role) Allows(adminOnly bool) bool {
	return !adminOnly || r == Admin
}
