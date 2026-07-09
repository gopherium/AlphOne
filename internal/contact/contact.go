// SPDX-License-Identifier: Elastic-2.0

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

// ErrNotFound reports that no contact exists for the requested ID.
var ErrNotFound = errors.New("contact: not found")

// Contact is a person tracked by the CRM.
type Contact struct {
	ID        uuid.UUID
	Name      string
	CreatedAt time.Time
}

// New returns a [Contact] with the given name, trimmed of surrounding
// whitespace.
func New(name string) (Contact, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return Contact{}, ErrEmptyName
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Contact{}, fmt.Errorf("contact: generate id: %w", err)
	}
	return Contact{
		ID:        id,
		Name:      trimmedName,
		CreatedAt: time.Now().UTC(),
	}, nil
}
