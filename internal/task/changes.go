// SPDX-License-Identifier: Elastic-2.0

package task

import (
	"errors"
	"time"
)

// ErrInvalidStatus reports that a task status is neither open nor done.
var ErrInvalidStatus = errors.New("task: invalid status")

// Changes carries the fields a partial task update replaces, leaving nil
// fields unchanged.
type Changes struct {
	Title    *string
	DueOn    *time.Time
	Status   *string
	Priority *int
}

// Apply returns t with the given changes validated and applied.
func (t Task) Apply(c Changes) (Task, error) {
	if c.Title != nil {
		title, err := ValidateTitle(*c.Title)
		if err != nil {
			return Task{}, err
		}
		t.Title = title
	}
	if c.Status != nil {
		if err := ValidateStatus(*c.Status); err != nil {
			return Task{}, err
		}
		t.Status = *c.Status
	}
	if c.Priority != nil {
		if err := ValidatePriority(*c.Priority); err != nil {
			return Task{}, err
		}
		t.Priority = *c.Priority
	}
	if c.DueOn != nil {
		t.DueOn = *c.DueOn
	}
	return t, nil
}

// ValidateStatus reports [ErrInvalidStatus] for anything but open and done.
func ValidateStatus(status string) error {
	if status != StatusOpen && status != StatusDone {
		return ErrInvalidStatus
	}
	return nil
}
