// SPDX-License-Identifier: Elastic-2.0

package task_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gopherium/alphone/internal/task"
)

// clockAnswer holds what one call building a [task.Clock] returned.
type clockAnswer struct {
	got task.Clock
	err error
}

// answerOf bundles the two results of a call building a [task.Clock].
func answerOf(got task.Clock, err error) clockAnswer {
	return clockAnswer{got: got, err: err}
}

// wantClockAt fails the test unless the call answered minutes past midnight and no error.
func wantClockAt(t *testing.T, call string, answer clockAnswer, minutes int) {
	t.Helper()
	want := task.Clock{Minutes: minutes, Set: true}
	if answer.err != nil || answer.got != want {
		t.Errorf("%s = %#v, %v, want %#v, nil", call, answer.got, answer.err, want)
	}
}

// wantNoClock fails the test unless the call answered no time and [task.ErrInvalidClock].
func wantNoClock(t *testing.T, call string, answer clockAnswer) {
	t.Helper()
	if !errors.Is(answer.err, task.ErrInvalidClock) || answer.got != (task.Clock{}) {
		t.Errorf("%s = %#v, %v, want no time and %v", call, answer.got, answer.err, task.ErrInvalidClock)
	}
}

func TestTheZeroClockHoldsNoTime(t *testing.T) {
	t.Parallel()

	var none task.Clock

	if none.Set {
		t.Errorf("zero Clock Set = true, want no time")
	}
	if got := none.String(); got != "" {
		t.Errorf("zero Clock String() = %q, want empty", got)
	}
}

func TestNewClockKeepsEveryMinuteOfTheDay(t *testing.T) {
	t.Parallel()

	for _, minutes := range []int{0, 570, 1439} {
		wantClockAt(t, fmt.Sprintf("NewClock(%d)", minutes), answerOf(task.NewClock(minutes)), minutes)
	}
}

func TestNewClockRefusesMinutesOutsideTheDay(t *testing.T) {
	t.Parallel()

	for _, minutes := range []int{-1, 1440, 1500} {
		wantNoClock(t, fmt.Sprintf("NewClock(%d)", minutes), answerOf(task.NewClock(minutes)))
	}
}

func TestParseClockReadsHoursAndMinutes(t *testing.T) {
	t.Parallel()

	tests := map[string]int{"00:00": 0, "09:05": 545, "11:00": 660, "23:59": 1439}

	for text, minutes := range tests {
		wantClockAt(t, fmt.Sprintf("ParseClock(%q)", text), answerOf(task.ParseClock(text)), minutes)
	}
}

func TestParseClockRefusesAnythingButTwoDigitHoursAndMinutes(t *testing.T) {
	t.Parallel()

	refused := []string{
		"", "24:00", "25:00", "9:00", "09:0", "09:00:30", "11:60", "0900", "09-00", " 9:00",
		"+9:00", "-1:00", "ab:cd", "09:5x", "１１:００",
	}

	for _, text := range refused {
		wantNoClock(t, fmt.Sprintf("ParseClock(%q)", text), answerOf(task.ParseClock(text)))
	}
}

func TestClockStringWritesTwoDigitHoursAndMinutes(t *testing.T) {
	t.Parallel()

	tests := map[int]string{0: "00:00", 545: "09:05", 660: "11:00", 1439: "23:59"}

	for minutes, want := range tests {
		if got := (task.Clock{Minutes: minutes, Set: true}).String(); got != want {
			t.Errorf("Clock{%d}.String() = %q, want %q", minutes, got, want)
		}
	}
}

func TestEveryMinuteOfTheDayReadsBackWhatItWrites(t *testing.T) {
	t.Parallel()

	for minutes := range 1440 {
		written, err := task.NewClock(minutes)
		if err != nil {
			t.Fatalf("NewClock(%d) error = %v, want nil", minutes, err)
		}
		text := written.String()
		wantClockAt(t, fmt.Sprintf("ParseClock(%q)", text), answerOf(task.ParseClock(text)), minutes)
	}
}
