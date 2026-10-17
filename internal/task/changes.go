// SPDX-License-Identifier: Elastic-2.0

package task

import (
	"errors"
	"time"
)

// ErrInvalidStatus reports that a task status is neither open nor done.
var ErrInvalidStatus = errors.New("task: invalid status")

// Changes carries the fields a partial task update replaces, a nil field kept and a DueTime of no time cleared.
type Changes struct {
	Title    *string
	DueOn    *time.Time
	DueTime  *Clock
	Status   *string
	Priority *int
}

// Apply returns t with the given changes validated and applied.
func (t Task) Apply(c Changes) (Task, error) {
	title, err := changedTitle(t.Title, c.Title)
	if err != nil {
		return Task{}, err
	}
	if err := validateGiven(c.Status, ValidateStatus); err != nil {
		return Task{}, err
	}
	if err := validateGiven(c.Priority, ValidatePriority); err != nil {
		return Task{}, err
	}
	t.Title = title
	t.Status = givenOr(c.Status, t.Status)
	t.Priority = givenOr(c.Priority, t.Priority)
	t.DueOn = givenOr(c.DueOn, t.DueOn)
	t.DueTime = givenOr(c.DueTime, t.DueTime)
	return t, nil
}

// changedTitle returns the title an update leaves, trimmed and validated when the update names one.
func changedTitle(current string, given *string) (string, error) {
	if given == nil {
		return current, nil
	}
	return ValidateTitle(*given)
}

// validateGiven returns what validate reports for the value a change names, nil when it names none.
func validateGiven[T any](given *T, validate func(T) error) error {
	if given == nil {
		return nil
	}
	return validate(*given)
}

// givenOr returns the value a change names, or current when it names none.
func givenOr[T any](given *T, current T) T {
	if given == nil {
		return current
	}
	return *given
}

// ValidateStatus reports [ErrInvalidStatus] for anything but open and done.
func ValidateStatus(status string) error {
	if status != StatusOpen && status != StatusDone {
		return ErrInvalidStatus
	}
	return nil
}
