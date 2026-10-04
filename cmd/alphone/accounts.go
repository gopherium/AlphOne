// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"fmt"

	"github.com/gopherium/framework/gonsole"
	accounts "github.com/gopherium/framework/gonsole/auth"

	"github.com/gopherium/alphone/internal/role"
)

// accountConfig returns the configuration of the account commands over the roles registry holds.
func accountConfig(registry *role.Registry) accounts.Config {
	return accounts.Config{Roles: declaredRoles(registry)}
}

// declaredRoles returns the role table of the account commands, the plugins registered first to declare theirs.
func declaredRoles(registry *role.Registry) func(context.Context, gonsole.Call) (accounts.Roles, error) {
	return func(ctx context.Context, call gonsole.Call) (accounts.Roles, error) {
		loaded, err := call.Plugins(ctx)
		if err != nil {
			return accounts.Roles{}, err
		}
		if loaded.Failed != nil {
			return accounts.Roles{}, fmt.Errorf("register plugins: %w", loaded.Failed)
		}
		held := registry.Roles()
		known := make([]string, 0, len(held))
		capabilities := make(map[string][]string, len(held))
		for _, name := range held {
			known = append(known, name.String())
			capabilities[name.String()] = registry.CapabilitiesOf(name)
		}
		return accounts.Roles{Known: known, Privileged: registry.Privileged(), Capabilities: capabilities}, nil
	}
}
