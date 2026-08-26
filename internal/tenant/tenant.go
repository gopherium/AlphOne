// SPDX-License-Identifier: Elastic-2.0

// Package tenant defines the tenant a caller stands in.
package tenant

import (
	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

// DefaultID identifies the tenant every unplaced user belongs to.
var DefaultID = sdk.DefaultTenantID

// Tenant is one organization served by the install.
type Tenant struct {
	ID          uuid.UUID
	Name        string
	Deactivated bool
}
