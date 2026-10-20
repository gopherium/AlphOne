// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"time"

	"github.com/gopherium/framework/goncierge"
	accounts "github.com/gopherium/framework/gonsole/auth"
	accountspg "github.com/gopherium/framework/gonsole/auth/postgres"

	"github.com/gopherium/alphone/internal/role"
)

// recordTimeout bounds storing one command record when ALPHONE_COMMAND_RECORD_TIMEOUT names no other.
const recordTimeout = 5 * time.Second

// recordsLimit is how many records account:records lists when ALPHONE_COMMAND_RECORDS_LIMIT names no other.
const recordsLimit = 50

// accountConfig returns the configuration of the account commands and of the check on the acting account.
func accountConfig(registry *goncierge.Registry) accounts.Config {
	return accounts.Config{
		Roles:         accounts.DeclaredRoles(registry, role.ManageUsers),
		Capability:    role.ManageUsers,
		RecordTimeout: recordTimeout,
		RecordsLimit:  recordsLimit,
		Stores:        accountspg.Stores,
	}
}
