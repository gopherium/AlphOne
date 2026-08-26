// SPDX-License-Identifier: Elastic-2.0

// Package tenant defines the tenant a caller stands in.
package tenant

import (
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

// DefaultID identifies the tenant every unplaced user belongs to.
var DefaultID = sdk.DefaultTenantID

// DefaultMachineGrace is the grace window an install serves when it configures none.
const DefaultMachineGrace = 14 * 24 * time.Hour

// Tenant is one organization served by the install.
type Tenant struct {
	ID            uuid.UUID
	Name          string
	Deactivated   bool
	DeactivatedAt time.Time
}

// AcceptsMachineTraffic reports whether the tenant still records what a channel delivers.
func (t Tenant) AcceptsMachineTraffic(now time.Time, grace time.Duration) bool {
	if !t.Deactivated {
		return true
	}
	return now.Sub(t.DeactivatedAt) < grace
}
