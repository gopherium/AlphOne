// SPDX-License-Identifier: Elastic-2.0

package message_test

import (
	"strings"
	"testing"
	"time"

	"github.com/gopherium/alphone/plugins/email/message"
)

func TestSanitizeFinishesOnALongStyleValue(t *testing.T) {
	cases := []struct {
		name   string
		prefix string
		unit   string
		size   int
		kept   bool
	}{
		{"a value of url openings", "color: ", "url(", 32 << 10, false},
		{"declarations of url openings", "", "x:url(;", 32 << 10, false},
		{"a list of font names", "font-family: ", "a,", 16 << 10, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			long := `<span style="` + c.prefix + strings.Repeat(c.unit, c.size/len(c.unit)) + `">Text</span>`
			want := `<span>Text</span>`
			if c.kept {
				want = long
			}
			done := make(chan string, 1)
			timer := time.NewTimer(deadline)
			defer timer.Stop()

			go func() {
				safe, _ := message.Sanitize(long, wideLimit)
				done <- safe
			}()

			select {
			case got := <-done:
				if got != want {
					t.Errorf("Sanitize() of %d bytes of %s = %.80q, want %.80q", len(long), c.name, got, want)
				}
			case <-timer.C:
				t.Fatalf("Sanitize() of %d bytes of %s took over %v", len(long), c.name, deadline)
			}
		})
	}
}
