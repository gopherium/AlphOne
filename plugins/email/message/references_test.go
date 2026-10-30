// SPDX-License-Identifier: Elastic-2.0

package message_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/gopherium/alphone/plugins/email/message"
)

// referencesLine opens the References header line.
const referencesLine = "References: "

// headerLineLimit is the longest header line the mail standard allows.
const headerLineLimit = 998

func TestReplyReferences(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		references []string
		inReplyTo  []string
		messageID  string
		want       []string
	}{
		{
			"the parent's references then its id",
			[]string{"r1@example.com", "r2@example.com"}, []string{"r2@example.com"}, "p@example.com",
			[]string{"r1@example.com", "r2@example.com", "p@example.com"},
		},
		{
			"one In-Reply-To id when the parent has no references",
			nil, []string{"r1@example.com"}, "p@example.com",
			[]string{"r1@example.com", "p@example.com"},
		},
		{
			"two In-Reply-To ids left unused",
			nil, []string{"r1@example.com", "r2@example.com"}, "p@example.com",
			[]string{"p@example.com"},
		},
		{"only the parent's id", nil, nil, "p@example.com", []string{"p@example.com"}},
		{
			"the parent's id already in its references",
			[]string{"r1@example.com", "p@example.com"}, nil, "p@example.com",
			[]string{"r1@example.com", "p@example.com"},
		},
		{"no parent id", []string{"r1@example.com"}, nil, "", []string{"r1@example.com"}},
		{"nothing to refer to", nil, nil, "", nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := message.ReplyReferences(c.references, c.inReplyTo, c.messageID)

			if !slices.Equal(got, c.want) {
				t.Errorf("ReplyReferences(%q, %q, %q) = %q, want %q", c.references, c.inReplyTo, c.messageID, got, c.want)
			}
		})
	}
}

func TestReplyReferencesLeavesTheParentListAlone(t *testing.T) {
	t.Parallel()

	references := make([]string, 1, 4)
	references[0] = "r1@example.com"

	first := message.ReplyReferences(references, nil, "p@example.com")
	message.ReplyReferences(references, nil, "q@example.com")

	if want := []string{"r1@example.com", "p@example.com"}; !slices.Equal(first, want) {
		t.Errorf("first ReplyReferences() = %q after a second call, want %q", first, want)
	}
}

func TestFormatReferencesTrimsToTheLineLimit(t *testing.T) {
	t.Parallel()

	ids := longIDs(40)

	got := message.FormatReferences(ids)

	if length := len(referencesLine + got); length > headerLineLimit {
		t.Fatalf("FormatReferences() line is %d characters, want at most %d", length, headerLineLimit)
	}
	kept := message.ParseIDs(got)
	if len(kept) < 3 || kept[0] != ids[0] {
		t.Fatalf("FormatReferences() kept %q, want the first id %q and at least two more", kept, ids[0])
	}
	tail := kept[1:]
	if !slices.Equal(tail, ids[len(ids)-len(tail):]) {
		t.Errorf("FormatReferences() kept %q after the first id, want the newest ids in order", tail)
	}
	oneMore := append([]string{ids[0]}, ids[len(ids)-len(tail)-1:]...)
	if length := len(referencesLine + message.FormatIDs(oneMore)); length <= headerLineLimit {
		t.Errorf("FormatReferences() dropped an id that fits, one more makes a line of %d characters", length)
	}
}

func TestFormatReferencesKeepsAShortList(t *testing.T) {
	t.Parallel()

	ids := []string{"r1@example.com", "r2@example.com", "p@example.com"}

	if got, want := message.FormatReferences(ids), message.FormatIDs(ids); got != want {
		t.Errorf("FormatReferences(%q) = %q, want %q", ids, got, want)
	}
}

func TestFormatReferencesKeepsTheFirstAndLastTwoOverTheLimit(t *testing.T) {
	t.Parallel()

	ids := longIDs(3)
	for i := range ids {
		ids[i] = strings.Repeat("x", headerLineLimit/2) + ids[i]
	}

	if got, want := message.FormatReferences(ids), message.FormatIDs(ids); got != want {
		t.Errorf("FormatReferences() of three ids over the limit = %q, want all three", got)
	}
}

func TestFormatReferencesLeavesOutUncleanIDs(t *testing.T) {
	t.Parallel()

	injected := "p@example.com>\r\nBcc: x@example.com"
	cases := []struct {
		name string
		ids  []string
	}{
		{"an unclean id last", append(longIDs(40), injected)},
		{
			"a long unclean id second",
			slices.Concat([]string{"first@example.com", strings.Repeat("x", 600) + " @example.com"}, longIDs(40)),
		},
		{"a parent id with a line break", message.ReplyReferences(longIDs(2), nil, injected)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := message.FormatReferences(c.ids)

			if strings.ContainsAny(got, "\r\n") || len(referencesLine+got) > headerLineLimit {
				t.Errorf("FormatReferences() = %q, want one line of at most %d characters", got, headerLineLimit)
			}
			given := func(id string) bool { return slices.Contains(c.ids, id) }
			if extra := slices.DeleteFunc(message.ParseIDs(got), given); len(extra) > 0 {
				t.Errorf("FormatReferences() wrote %q, ids it was not given", extra)
			}
		})
	}
}

func TestFormatReferencesOfNoIDsIsEmpty(t *testing.T) {
	t.Parallel()

	if got := message.FormatReferences(nil); got != "" {
		t.Errorf("FormatReferences(nil) = %q, want empty", got)
	}
}

// longIDs answers count distinct message ids of about sixty characters each.
func longIDs(count int) []string {
	ids := make([]string, count)
	for i := range ids {
		ids[i] = fmt.Sprintf("%02d.%s@example.com", i, strings.Repeat("a", 40))
	}
	return ids
}
