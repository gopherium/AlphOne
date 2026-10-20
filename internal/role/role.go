// SPDX-License-Identifier: Elastic-2.0

// Package role holds the roles the core grants and the capabilities they carry, kept in a goncierge registry.
package role

import "github.com/gopherium/framework/goncierge"

// Source is the name the core grants its roles under.
const Source = "core"

// Role is the tier a user stands in.
type Role string

// The tiers the core declares.
const (
	// Admin manages users beside everything a member reaches.
	Admin Role = "admin"
	// Member works the product without reaching user management.
	Member Role = "member"
)

// ManageUsers is the capability administering accounts.
const ManageUsers = "manage_users"

// ManageWebhooks is the capability creating webhooks and managing every webhook of a workspace.
const ManageWebhooks = "manage_webhooks"

// coreRoles is every role the core grants, with the capabilities each carries.
var coreRoles = []goncierge.Role{
	{Name: Admin.String(), Capabilities: []string{ManageUsers, ManageWebhooks}},
	{Name: Member.String()},
}

// NewRegistry returns a registry holding the core roles and nothing a plugin declares.
func NewRegistry() *goncierge.Registry {
	return mustRegistry(coreRoles)
}

// mustRegistry returns a registry holding the roles under the core source, panicking when the name rule refuses one.
func mustRegistry(roles []goncierge.Role) *goncierge.Registry {
	registry := goncierge.New()
	if err := registry.Replace(Source, roles); err != nil {
		panic(err)
	}
	return registry
}

// Declare replaces the roles a source declares in the registry, keeping its new capabilities off the admin role.
func Declare(registry *goncierge.Registry, source string, roles []goncierge.Role) error {
	return goncierge.Declare(registry, goncierge.Rules{}, source, roles)
}

// String returns the stored form of the tier.
func (r Role) String() string {
	return string(r)
}
