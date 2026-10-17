// SPDX-License-Identifier: Elastic-2.0

package task

import (
	"errors"
	"fmt"
)

// ErrInvalidClock reports a time of day outside 00:00 to 23:59 or not written as HH:MM.
var ErrInvalidClock = errors.New("task: invalid time of day")

const (
	// minutesPerHour is the count of minutes in one hour.
	minutesPerHour = 60
	// minutesPerDay is the count of minutes in one day.
	minutesPerDay = 24 * minutesPerHour
	// clockLayoutLength is the length of a time of day written as HH:MM.
	clockLayoutLength = len("15:04")
)

// Clock is a wall clock time of day in whole minutes, the zero value holding no time.
type Clock struct {
	Minutes int
	Set     bool
}

// NewClock returns the time of day minutes past midnight, or [ErrInvalidClock] outside 00:00 to 23:59.
func NewClock(minutes int) (Clock, error) {
	if minutes < 0 || minutes >= minutesPerDay {
		return Clock{}, ErrInvalidClock
	}
	return Clock{Minutes: minutes, Set: true}, nil
}

// ParseClock reads a time of day written exactly as HH:MM, or reports [ErrInvalidClock].
func ParseClock(text string) (Clock, error) {
	hours, minutes, read := splitClock(text)
	if !read || minutes >= minutesPerHour {
		return Clock{}, ErrInvalidClock
	}
	return NewClock(hours*minutesPerHour + minutes)
}

// splitClock returns the hours and minutes text spells, and whether text has exactly the HH:MM shape.
func splitClock(text string) (int, int, bool) {
	if len(text) != clockLayoutLength || text[2] != ':' {
		return 0, 0, false
	}
	hours, hoursRead := twoDigits(text[:2])
	minutes, minutesRead := twoDigits(text[3:])
	return hours, minutes, hoursRead && minutesRead
}

// String returns c as HH:MM, empty when c holds no time.
func (c Clock) String() string {
	if !c.Set {
		return ""
	}
	return fmt.Sprintf("%02d:%02d", c.Minutes/minutesPerHour, c.Minutes%minutesPerHour)
}

// twoDigits returns the number two ASCII digits spell, and whether pair holds exactly that.
func twoDigits(pair string) (int, bool) {
	tens, ones := pair[0]-'0', pair[1]-'0'
	if tens > 9 || ones > 9 {
		return 0, false
	}
	return int(tens)*10 + int(ones), true
}
