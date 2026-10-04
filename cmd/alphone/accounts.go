// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/gopherium/framework/gonsole"
	accounts "github.com/gopherium/framework/gonsole/auth"

	"github.com/gopherium/alphone/internal/role"
)

// recordTimeout bounds storing one command record when ALPHONE_COMMAND_RECORD_TIMEOUT names no other.
const recordTimeout = 5 * time.Second

// recordsLimit is how many records account:records lists when ALPHONE_COMMAND_RECORDS_LIMIT names no other.
const recordsLimit = 50

// accountConfig returns the configuration of the account commands and of the check on the acting account.
func accountConfig(registry *role.Registry) accounts.Config {
	return accounts.Config{
		Roles:         declaredRoles(registry),
		Capability:    string(role.ManageUsers),
		RecordTimeout: recordTimeout,
		RecordsLimit:  recordsLimit,
	}
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
