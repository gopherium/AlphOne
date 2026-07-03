// SPDX-License-Identifier: AGPL-3.0-or-later

// Package contact defines the person at the center of the CRM.
package contact

import (
	"errors"
	"strings"
)

// ErrEmptyName reports that a contact name is empty or only whitespace.
var ErrEmptyName = errors.New("contact: empty name")

// Contact is a person tracked by the CRM.
type Contact struct {
	Name string
}

// New creates a Contact from a raw name, trimming surrounding whitespace.
func New(name string) (Contact, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return Contact{}, ErrEmptyName
	}
	return Contact{Name: trimmed}, nil
}
