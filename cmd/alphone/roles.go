// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/gopherium/framework/goncierge"

	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/sdk"
)

// errReservedID reports a plugin under the id the core grants its roles under.
var errReservedID = errors.New("the id is reserved for the core roles")

// errSecondTopRole reports a second plugin naming the deployment's top role.
var errSecondTopRole = errors.New("only one plugin may name the top role")

// errEmptyTopRole reports a plugin naming an empty top role.
var errEmptyTopRole = errors.New("the top role is empty")

// errUndeclaredTopRole reports a plugin naming a top role it does not declare.
var errUndeclaredTopRole = errors.New("the plugin does not declare its top role")

// errShippedTopRole reports a plugin naming a role AlphOne ships as the top role.
var errShippedTopRole = errors.New("AlphOne ships the top role")

// errTopRoleLacksAdminCapability reports a top role missing a capability admin carries.
var errTopRoleLacksAdminCapability = errors.New("the top role lacks a capability admin carries")

// pluginRoles is the roles one plugin declares, read once.
type pluginRoles struct {
	id    string
	roles []sdk.RoleDeclaration
}

// topRole is the top role one plugin names, empty when no plugin names one.
type topRole struct {
	owner string
	name  string
}

// declareRoles declares every plugin's roles in load order, then gives the top role every capability the others carry.
func declareRoles(registry *goncierge.Registry, registered []sdk.Plugin) error {
	declared, top, err := readRoles(registered)
	if err != nil {
		return err
	}
	if err := declareEach(registry, "", declared); err != nil {
		return err
	}
	if top.name == "" {
		return nil
	}
	others := slices.DeleteFunc(slices.Clone(declared), func(plugin pluginRoles) bool { return plugin.id == top.owner })
	if err := declareEach(registry, top.name, others); err != nil {
		return err
	}
	return checkTopRoleCarriesAdmin(registry, top)
}

// readRoles reads the roles every plugin declares and the top role one of them names, each once, in load order.
func readRoles(registered []sdk.Plugin) ([]pluginRoles, topRole, error) {
	var (
		every []pluginRoles
		top   topRole
	)
	for _, plugin := range registered {
		if plugin.ID() == role.Source {
			return nil, topRole{}, fmt.Errorf("plugin %s: %w", plugin.ID(), errReservedID)
		}
		declared, provides := rolesOf(plugin)
		named, err := nameTopRole(top, plugin, declared)
		if err != nil {
			return nil, topRole{}, err
		}
		top = named
		if provides {
			every = append(every, pluginRoles{id: plugin.ID(), roles: declared})
		}
	}
	return every, top, nil
}

// rolesOf returns the roles the plugin declares and whether it declares roles at all.
func rolesOf(plugin sdk.Plugin) ([]sdk.RoleDeclaration, bool) {
	provider, ok := plugin.(sdk.RoleProvider)
	if !ok {
		return nil, false
	}
	return provider.Roles(), true
}

// nameTopRole returns the top role the plugin names, or the one named before it when the plugin names none.
func nameTopRole(named topRole, plugin sdk.Plugin, declared []sdk.RoleDeclaration) (topRole, error) {
	provider, ok := plugin.(sdk.TopRoleProvider)
	if !ok {
		return named, nil
	}
	if named.name != "" {
		return topRole{}, fmt.Errorf("plugins %s and %s: %w", named.owner, plugin.ID(), errSecondTopRole)
	}
	top := topRole{owner: plugin.ID(), name: provider.TopRole()}
	return top, checkTopRole(top, declared)
}

// checkTopRole reports a top role that is empty, that AlphOne ships or that its plugin does not declare.
func checkTopRole(top topRole, declared []sdk.RoleDeclaration) error {
	names := func(declaration sdk.RoleDeclaration) bool { return declaration.Name == top.name }
	switch {
	case top.name == "":
		return fmt.Errorf("plugin %s: %w", top.owner, errEmptyTopRole)
	case role.NewRegistry().Known(top.name):
		return fmt.Errorf("plugin %s: %w %q", top.owner, errShippedTopRole, top.name)
	case !slices.ContainsFunc(declared, names):
		return fmt.Errorf("plugin %s: %w %q", top.owner, errUndeclaredTopRole, top.name)
	}
	return nil
}

// declareEach declares the roles each plugin declares in order, giving the top role every capability they carry.
func declareEach(registry *goncierge.Registry, top string, declared []pluginRoles) error {
	for _, plugin := range declared {
		if err := role.DeclareWithTopRole(registry, top, plugin.id, plugin.roles); err != nil {
			return fmt.Errorf("plugin %s: %w", plugin.id, err)
		}
	}
	return nil
}

// checkTopRoleCarriesAdmin reports a top role lacking a capability admin carries.
func checkTopRoleCarriesAdmin(registry *goncierge.Registry, top topRole) error {
	lacking := slices.DeleteFunc(registry.CapabilitiesOf(role.Admin.String()), func(capability string) bool {
		return registry.Can(top.name, capability)
	})
	if len(lacking) > 0 {
		return fmt.Errorf("plugin %s: %w: %s", top.owner, errTopRoleLacksAdminCapability, strings.Join(lacking, ", "))
	}
	return nil
}
