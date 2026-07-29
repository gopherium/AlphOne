// SPDX-License-Identifier: Elastic-2.0

// Package task defines the unit of work a CRM user acts on.
package task

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrEmptyTitle reports that a task title is empty or only whitespace.
var ErrEmptyTitle = errors.New("task: empty title")

// ErrInvalidPriority reports that a task priority falls outside 0 to 9.
var ErrInvalidPriority = errors.New("task: invalid priority")

// ErrNotFound reports that no task exists for the requested ID.
var ErrNotFound = errors.New("task: not found")

// Task statuses.
const (
	StatusOpen = "open"
	StatusDone = "done"
)

// maxPriority is the highest priority a task accepts.
const maxPriority = 9

// Origin references the event a task was created from.
type Origin struct {
	Source  string
	EventID uuid.UUID
}

// Input carries the values a new [Task] is built from.
type Input struct {
	Title      string
	DueOn      time.Time
	Priority   int
	AssigneeID uuid.UUID
	ContactID  uuid.UUID
	Origin     Origin
}

// Task is a single piece of work owned by a user.
type Task struct {
	ID         uuid.UUID
	AssigneeID uuid.UUID
	ContactID  uuid.UUID
	Title      string
	Status     string
	Priority   int
	DueOn      time.Time
	Origin     Origin
	CreatedAt  time.Time
}

// New returns an open [Task] built from in, with its title trimmed of
// surrounding whitespace.
func New(in Input) (Task, error) {
	title, err := ValidateTitle(in.Title)
	if err != nil {
		return Task{}, err
	}
	if err := ValidatePriority(in.Priority); err != nil {
		return Task{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Task{}, fmt.Errorf("task: generate id: %w", err)
	}
	return Task{
		ID:         id,
		AssigneeID: in.AssigneeID,
		ContactID:  in.ContactID,
		Title:      title,
		Status:     StatusOpen,
		Priority:   in.Priority,
		DueOn:      in.DueOn,
		Origin:     in.Origin,
		CreatedAt:  time.Now().UTC(),
	}, nil
}

// ValidateTitle returns the title trimmed, or [ErrEmptyTitle] when blank.
func ValidateTitle(title string) (string, error) {
	trimmedTitle := strings.TrimSpace(title)
	if trimmedTitle == "" {
		return "", ErrEmptyTitle
	}
	return trimmedTitle, nil
}

// ValidatePriority reports [ErrInvalidPriority] for a priority outside 0 to 9.
func ValidatePriority(priority int) error {
	if priority < 0 || priority > maxPriority {
		return ErrInvalidPriority
	}
	return nil
}
