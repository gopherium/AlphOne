// SPDX-License-Identifier: Elastic-2.0

package message_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gopherium/alphone/plugins/email/message"
)

// unreachedLimit is a preview limit longer than any test text.
const unreachedLimit = 1000

// snippetCases lists texts and the preview Snippet answers for each.
var snippetCases = []struct {
	name string
	text string
	want string
}{
	{
		"a Gmail attribution line",
		"Yes, please send the quote.\r\n\r\n" +
			"On Mon, Oct 26, 2026 at 10:15 AM Maria Perez <maria.perez@example.com> wrote:\r\n\r\n" +
			"> Would you like the quote by email?\r\n",
		"Yes, please send the quote.",
	},
	{
		"a Gmail attribution line wrapped over two lines",
		"Yes, please send the quote.\r\n\r\nOn Mon, Oct 26, 2026 at 10:15 AM Maria Perez <\r\n" +
			"maria.perez@example.com> wrote:\r\n\r\n> Would you like the quote by email?\r\n",
		"Yes, please send the quote.",
	},
	{
		"an Apple Mail attribution line",
		"Thursday works.\r\n\r\nOn Oct 26, 2026, at 10:15, Maria Perez <maria.perez@example.com> wrote:\r\n\r\n" +
			"> Could we visit the site on Thursday?\r\n",
		"Thursday works.",
	},
	{
		"a Thunderbird attribution line",
		"Thursday works.\r\n\r\nOn 10/26/26 10:15 AM, Maria Perez wrote:\r\n> Could we visit the site on Thursday?\r\n",
		"Thursday works.",
	},
	{
		"a Yahoo attribution line",
		"Thursday works.\r\n\r\n" +
			"On Monday, October 26, 2026 at 10:15:00 AM GMT, Maria Perez <maria.perez@example.com> wrote:\r\n\r\n" +
			"> Could we visit the site on Thursday?\r\n",
		"Thursday works.",
	},
	{
		"an Outlook header block after a rule of underscores",
		"Thursday at 10 is fine.\r\n\r\n________________________________\r\n" +
			"From: Maria Perez <maria.perez@example.com>\r\nSent: Monday, October 26, 2026 10:15 AM\r\n" +
			"To: Info <info@example.com>\r\nSubject: Site visit\r\n\r\nCould we visit the site on Thursday at 10?\r\n",
		"Thursday at 10 is fine.",
	},
	{
		"an original message line",
		"Thursday at 10 is fine.\r\n\r\n-----Original Message-----\r\n" +
			"From: Maria Perez <maria.perez@example.com>\r\nSent: Monday, October 26, 2026 10:15 AM\r\n\r\n" +
			"Could we visit the site on Thursday at 10?\r\n",
		"Thursday at 10 is fine.",
	},
	{
		"quoted lines between the writer's own lines",
		"> Could we visit on Thursday?\r\nYes, Thursday works.\r\n> At 10?\r\nAt 10 is fine.\r\n",
		"Yes, Thursday works. At 10 is fine.",
	},
	{
		"a reply written below its quote",
		"On 10/26/26 10:15 AM, Maria Perez wrote:\r\n> Could we visit the site on Thursday at 10?\r\n\r\n" +
			"Thursday at 10 works.\r\n\r\n-- \r\nMaria Perez\r\n184467235\r\n",
		"Thursday at 10 works.",
	},
	{
		"a reply written below a quote whose attribution line wraps",
		"On Mon, Oct 26, 2026 at 10:15 AM Maria Perez <\r\nmaria.perez@example.com> wrote:\r\n" +
			"> Could we visit the site on Thursday at 10?\r\n\r\nThursday at 10 works.\r\n",
		"Thursday at 10 works.",
	},
	{
		"a signature separator",
		"Thanks for the samples.\r\n\r\n-- \r\nMaria Perez\r\n184467235\r\n",
		"Thanks for the samples.",
	},
	{
		"two dashes with no blank",
		"Bring the samples.\r\n--\r\nAnd the tape.\r\n",
		"Bring the samples. -- And the tape.",
	},
	{
		"an iPhone signature",
		"See you on Thursday.\r\n\r\nSent from my iPhone\r\n",
		"See you on Thursday.",
	},
	{
		"an Outlook for iOS signature",
		"See you on Thursday.\r\n\r\nGet Outlook for iOS\r\n",
		"See you on Thursday.",
	},
	{
		"a Gmail forward marker",
		"Please see the quote below.\r\n\r\n---------- Forwarded message ---------\r\n" +
			"From: Maria Perez <maria.perez@example.com>\r\nDate: Mon, Oct 26, 2026 at 10:15 AM\r\n" +
			"Subject: Quote\r\nTo: <info@example.com>\r\n\r\nThe quote is attached.\r\n",
		"Please see the quote below.",
	},
	{
		"an Apple Mail forward marker",
		"Please see the quote below.\r\n\r\nBegin forwarded message:\r\n\r\n" +
			"From: Maria Perez <maria.perez@example.com>\r\nSubject: Quote\r\n\r\nThe quote is attached.\r\n",
		"Please see the quote below.",
	},
	{
		"a Thunderbird forward marker",
		"Please see the quote below.\r\n\r\n-------- Forwarded Message --------\r\nSubject: Quote\r\n\r\n" +
			"The quote is attached.\r\n",
		"Please see the quote below.",
	},
	{
		"a line saying what the writer wrote",
		"I wrote: the plan is ready.\r\nPlease check it.\r\n",
		"I wrote: the plan is ready. Please check it.",
	},
	{
		"a line opening with On and ending with wrote but holding no date",
		"On reflection, here is what I wrote:\r\nThe plan is ready.\r\n",
		"On reflection, here is what I wrote: The plan is ready.",
	},
	{
		"a line opening with On and a date that is no attribution",
		"On 10/26 we met at the site.\r\nThe wall is ready.\r\n",
		"On 10/26 we met at the site. The wall is ready.",
	},
	{
		"only quoted text",
		"> Would you like the quote by email?\r\n>\r\n",
		"> Would you like the quote by email? >",
	},
	{
		"runs of blanks and line breaks",
		"Thursday \t works.\r\n\r\n\r\n  See   you then.\r\n",
		"Thursday works. See you then.",
	},
	{
		"bare line feeds",
		"Yes, please.\n\nOn Mon, Oct 26, 2026 at 10:15 AM Maria Perez <maria.perez@example.com> wrote:\n" +
			"> Would you like the quote?\n",
		"Yes, please.",
	},
	{"empty", "", ""},
}

func TestSnippet(t *testing.T) {
	t.Parallel()

	for _, c := range snippetCases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got, err := message.Snippet(c.text, unreachedLimit)

			if err != nil || got != c.want {
				t.Errorf("Snippet(%q) = %q, %v, want %q", c.text, got, err, c.want)
			}
		})
	}
}

func TestSnippetCutsAtTheLimit(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		text  string
		limit int
		want  string
	}{
		{"a cut after a letter of two bytes", "The new façade costs £120.", 11, "The new faç"},
		{"a cut after a blank", "The new façade", 8, "The new"},
		{"a text as long as the limit", "The new façade", 14, "The new façade"},
		{"a limit of one letter", "façade", 1, "f"},
		{"a word longer than the limit", "£££££££££££", 3, "£££"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got, err := message.Snippet(c.text, c.limit)

			if err != nil {
				t.Fatalf("Snippet(%q, %d) error = %v, want nil", c.text, c.limit, err)
			}
			if got != c.want {
				t.Errorf("Snippet(%q, %d) = %q, want %q", c.text, c.limit, got, c.want)
			}
			if !utf8.ValidString(got) || utf8.RuneCountInString(got) > c.limit {
				t.Errorf("Snippet(%q, %d) = %q, want valid UTF-8 of at most %d letters", c.text, c.limit, got, c.limit)
			}
		})
	}
}

func TestSnippetRefusesUnsetLimits(t *testing.T) {
	t.Parallel()

	for _, limit := range []int{0, -1} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			t.Parallel()

			got, err := message.Snippet("Thursday works.", limit)

			if !errors.Is(err, message.ErrLimitsUnset) || got != "" {
				t.Errorf("Snippet() with limit %d = %q, %v, want ErrLimitsUnset", limit, got, err)
			}
		})
	}
}

func TestSnippetFinishesOnAlternatingBlankAndAttributionLines(t *testing.T) {
	text := strings.Repeat("\nOn 1 wrote:\n", 80000)
	done := make(chan string, 1)
	timer := time.NewTimer(deadline)
	defer timer.Stop()

	go func() {
		preview, _ := message.Snippet(text, 200)
		done <- preview
	}()

	select {
	case got := <-done:
		if want := strings.Repeat("On 1 wrote: ", 17)[:200]; got != want {
			t.Errorf("Snippet() of %d bytes of attribution lines = %q, want %q", len(text), got, want)
		}
	case <-timer.C:
		t.Fatalf("Snippet() of %d bytes of attribution lines took over %v", len(text), deadline)
	}
}

func TestSnippetFixtures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		file string
		want string
	}{
		{"gmail-reply.eml", "Yes, please send the quote to this address."},
		{"apple-mail-reply.eml", "Thursday works. See you then."},
		{"outlook-reply.eml", "Thursday at 10 is fine. I will meet you at the front door."},
		{"thunderbird-reply.eml", "Thanks, Thursday at 10 works for me. Please bring the colour samples."},
	}

	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			t.Parallel()

			got, err := message.Snippet(parseFixture(t, c.file).Text, unreachedLimit)

			if err != nil || got != c.want {
				t.Errorf("Snippet(%s) = %q, %v, want %q", c.file, got, err, c.want)
			}
		})
	}
}
