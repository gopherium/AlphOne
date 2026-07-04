// SPDX-License-Identifier: AGPL-3.0-or-later

// Package contact defines the person at the center of the CRM.
package contact

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrEmptyName reports that a contact name is empty or only whitespace.
var ErrEmptyName = errors.New("contact: empty name")

// Contact is a person tracked by the CRM.
type Contact struct {
	ID        uuid.UUID
	Name      string
	CreatedAt time.Time
}

// New returns a Contact with the given name, trimmed of surrounding
// whitespace.
func New(name string) (Contact, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return Contact{}, ErrEmptyName
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Contact{}, fmt.Errorf("contact: generate id: %w", err)
	}
	return Contact{
		ID:        id,
		Name:      trimmed,
		CreatedAt: time.Now().UTC(),
	}, nil
}
