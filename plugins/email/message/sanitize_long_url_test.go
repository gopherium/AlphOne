// SPDX-License-Identifier: Elastic-2.0

//go:build !race

package message_test

import (
	"strings"
	"testing"
	"time"

	"github.com/gopherium/alphone/plugins/email/message"
)

func TestSanitizeFinishesOnALongRunOfURLOpenings(t *testing.T) {
	cases := []struct {
		name   string
		prefix string
		unit   string
	}{
		{"a value of url openings", "color: ", "url("},
		{"declarations of url openings", "", "x:url(;"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hostile := `<span style="` + c.prefix + strings.Repeat(c.unit, 256<<10/len(c.unit)) + `">Text</span>`
			done := make(chan string, 1)
			timer := time.NewTimer(deadline)
			defer timer.Stop()

			go func() {
				safe, _ := message.Sanitize(hostile, wideLimit)
				done <- safe
			}()

			select {
			case got := <-done:
				if got != `<span>Text</span>` {
					t.Errorf("Sanitize() of %d bytes of %s = %.80q, want the style dropped", len(hostile), c.name, got)
				}
			case <-timer.C:
				t.Fatalf("Sanitize() of %d bytes of %s took over %v", len(hostile), c.name, deadline)
			}
		})
	}
}
