// SPDX-License-Identifier: Elastic-2.0

// Package tenant defines the tenant a caller stands in.
package tenant

import "github.com/google/uuid"

// DefaultID identifies the tenant every unplaced user belongs to.
var DefaultID = uuid.MustParse("00000000-0000-7000-8000-000000000001")

// Tenant is one organization served by the install.
type Tenant struct {
	ID   uuid.UUID
	Name string
}
