// SPDX-License-Identifier: Elastic-2.0

package message_test

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/gopherium/alphone/plugins/email/message"
)

func TestParseRefusesAMessageOverTheByteCap(t *testing.T) {
	t.Parallel()

	raw := manyParts(3)

	limits := message.Limits{MaxBytes: int64(len(raw)) - 1, MaxParts: 1, MaxContainers: 1, MaxFieldBytes: 1 << 10}

	_, err := message.Parse(bytes.NewReader(raw), limits)

	if !errors.Is(err, message.ErrTooLarge) {
		t.Errorf("Parse() of %d bytes under a cap of %d error = %v, want ErrTooLarge", len(raw), len(raw)-1, err)
	}
}

func TestParseTakesAMessageAtTheByteCap(t *testing.T) {
	t.Parallel()

	for _, maxBytes := range []int64{int64(len(manyParts(3))), math.MaxInt64} {
		t.Run(fmt.Sprint(maxBytes), func(t *testing.T) {
			t.Parallel()

			limits := message.Limits{MaxBytes: maxBytes, MaxParts: 3, MaxContainers: 1, MaxFieldBytes: 1 << 10}

			m, err := message.Parse(bytes.NewReader(manyParts(3)), limits)

			if err != nil || len(m.Attachments) != 3 {
				t.Errorf("Parse() under a cap of %d error = %v, want three attachments", maxBytes, err)
			}
		})
	}
}

func TestParseRefusesUnsetLimits(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		limits message.Limits
	}{
		{"a zero byte cap", message.Limits{MaxParts: 100, MaxContainers: 10, MaxFieldBytes: 1 << 10}},
		{"a negative byte cap", message.Limits{MaxBytes: -1, MaxParts: 100, MaxContainers: 10, MaxFieldBytes: 1 << 10}},
		{"a zero part limit", message.Limits{MaxBytes: 1 << 20, MaxContainers: 10, MaxFieldBytes: 1 << 10}},
		{"a negative part limit", message.Limits{MaxBytes: 1 << 20, MaxParts: -1, MaxContainers: 10, MaxFieldBytes: 1 << 10}},
		{"a zero container limit", message.Limits{MaxBytes: 1 << 20, MaxParts: 100, MaxFieldBytes: 1 << 10}},
		{
			"a negative container limit",
			message.Limits{MaxBytes: 1 << 20, MaxParts: 100, MaxContainers: -1, MaxFieldBytes: 1 << 10},
		},
		{"a zero field cap", message.Limits{MaxBytes: 1 << 20, MaxParts: 100, MaxContainers: 10}},
		{"a negative field cap", message.Limits{MaxBytes: 1 << 20, MaxParts: 100, MaxContainers: 10, MaxFieldBytes: -1}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			unread := iotest.ErrReader(errors.New("the reader was read"))

			if _, err := message.Parse(unread, c.limits); !errors.Is(err, message.ErrLimitsUnset) {
				t.Errorf("Parse() with limits %+v error = %v, want ErrLimitsUnset", c.limits, err)
			}
		})
	}
}

func TestParseRefusesTooManyParts(t *testing.T) {
	t.Parallel()

	raw := manyParts(4)

	limits := message.Limits{MaxBytes: int64(len(raw)), MaxParts: 3, MaxContainers: 1, MaxFieldBytes: 1 << 10}

	_, err := message.Parse(bytes.NewReader(raw), limits)

	if !errors.Is(err, message.ErrTooManyParts) {
		t.Errorf("Parse() of four parts under a limit of three error = %v, want ErrTooManyParts", err)
	}
}

func TestParseRefusesDeepNestingQuickly(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
	}{
		{"1000 nested parts", nestedParts(1000)},
		{"50 unclosed nested parts", unclosedParts(50)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			limits := message.Limits{MaxBytes: int64(len(c.raw)), MaxParts: 100, MaxContainers: 10, MaxFieldBytes: 1 << 10}

			if err := parseWithinDeadline(t, c.raw, limits); !errors.Is(err, message.ErrTooManyParts) {
				t.Errorf("Parse() of %s under a container limit of 10 error = %v, want ErrTooManyParts", c.name, err)
			}
		})
	}
}

func TestParseCountsEveryContainer(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		field string
		want  error
	}{
		{"a field on one line", "Content-Type: multipart/mixed; boundary=\"inner\"", message.ErrTooManyParts},
		{"a field folded over two lines", "Content-Type: multipart/mixed;\r\n boundary=\"inner\"", message.ErrTooManyParts},
		{
			"a field continued on a line with no indent",
			"Content-Type: multipart/mixed;\r\nboundary=\"inner\"", message.ErrTooManyParts,
		},
		{
			"a field continued past a line the reader skips",
			"Content-Type: multipart/mixed;\r\n:skipped\r\n boundary=\"inner\"", message.ErrTooManyParts,
		},
		{"a lowercase field name", "content-type: multipart/mixed; boundary=\"inner\"", message.ErrTooManyParts},
		{"a blank before the colon", "Content-Type : multipart/mixed; boundary=\"inner\"", message.ErrTooManyParts},
		{
			"a parameter name in an encoded word",
			"Content-Type: multipart/mixed; =?us-ascii?b?Ym91bmRhcnk=?==inner", message.ErrTooManyParts,
		},
		{
			"a boundary split into sections",
			"Content-Type: multipart/mixed; boundary*0=\"in\"; boundary*1=\"ner\"", message.ErrTooManyParts,
		},
		{"a field with no boundary", "Content-Type: text/plain", nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			raw := "Content-Type: multipart/mixed; boundary=\"outer\"\r\n\r\n--outer\r\n" + c.field +
				"\r\n\r\n--inner\r\nContent-Type: text/plain\r\n\r\nText.\r\n--inner--\r\n--outer--\r\n"
			limits := message.Limits{MaxBytes: 1 << 20, MaxParts: 100, MaxContainers: 1, MaxFieldBytes: 1 << 10}

			if _, err := message.Parse(strings.NewReader(raw), limits); !errors.Is(err, c.want) {
				t.Errorf("Parse() of a second container in %q under a limit of one error = %v, want %v", c.field, err, c.want)
			}
		})
	}
}

func TestParseTakesAMessageAtTheContainerLimit(t *testing.T) {
	t.Parallel()

	raw := nestedParts(2)

	for _, c := range []struct {
		maxContainers int
		want          error
	}{{3, nil}, {2, message.ErrTooManyParts}} {
		t.Run(fmt.Sprint(c.maxContainers), func(t *testing.T) {
			t.Parallel()

			limits := message.Limits{
				MaxBytes: int64(len(raw)), MaxParts: 100, MaxContainers: c.maxContainers, MaxFieldBytes: 1 << 10,
			}

			if _, err := message.Parse(bytes.NewReader(raw), limits); !errors.Is(err, c.want) {
				t.Errorf("Parse() of three containers under a limit of %d error = %v, want %v", c.maxContainers, err, c.want)
			}
		})
	}
}

func TestParseRefusesALongMediaFieldQuickly(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"a type field in the top part", "Content-Type: text/plain; " + manyParameters(80000) + "\r\n\r\nText.\r\n"},
		{
			"a disposition field in a nested part",
			"Content-Type: multipart/mixed; boundary=\"parts\"\r\n\r\n--parts\r\nContent-Type: text/plain\r\n" +
				"Content-Disposition: attachment; " + manyParameters(80000) + "\r\n\r\nText.\r\n--parts--\r\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			limits := message.Limits{MaxBytes: int64(len(c.raw)), MaxParts: 100, MaxContainers: 10, MaxFieldBytes: 1 << 10}

			if err := parseWithinDeadline(t, []byte(c.raw), limits); !errors.Is(err, message.ErrTooLarge) {
				t.Errorf("Parse() of %d bytes with %s error = %v, want ErrTooLarge", len(c.raw), c.name, err)
			}
		})
	}
}

func TestParseTakesAMediaFieldAtTheFieldCap(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		field string
		value string
	}{
		{"a type field", "Content-Type: text/plain; charset=us-ascii", "text/plain; charset=us-ascii"},
		{"a folded type field", "Content-Type: text/plain;\r\n charset=us-ascii", "text/plain; charset=us-ascii"},
		{"a disposition field", "Content-Disposition: inline; filename=\"note.txt\"", "inline; filename=\"note.txt\""},
	}

	for _, c := range cases {
		for _, limit := range []struct {
			maxFieldBytes int
			want          error
		}{{len(c.value), nil}, {len(c.value) - 1, message.ErrTooLarge}} {
			t.Run(fmt.Sprint(c.name, " ", limit.maxFieldBytes), func(t *testing.T) {
				t.Parallel()

				raw := c.field + "\r\n\r\nText.\r\n"
				limits := message.Limits{
					MaxBytes: int64(len(raw)), MaxParts: 100, MaxContainers: 10, MaxFieldBytes: limit.maxFieldBytes,
				}

				if _, err := message.Parse(strings.NewReader(raw), limits); !errors.Is(err, limit.want) {
					t.Errorf("Parse() of %s under a field cap of %d error = %v, want %v",
						c.name, limit.maxFieldBytes, err, limit.want)
				}
			})
		}
	}
}

// parseWithinDeadline answers the error Parse answers for raw under limits, failing t when Parse runs past deadline.
func parseWithinDeadline(t *testing.T, raw []byte, limits message.Limits) error {
	t.Helper()
	done := make(chan error, 1)
	timer := time.NewTimer(deadline)
	defer timer.Stop()

	go func() {
		_, err := message.Parse(bytes.NewReader(raw), limits)
		done <- err
	}()

	select {
	case err := <-done:
		return err
	case <-timer.C:
		t.Fatalf("Parse() of %d bytes took over %v", len(raw), deadline)
		return nil
	}
}

// manyParts answers a raw message holding count small attachments.
func manyParts(count int) []byte {
	var b strings.Builder
	b.WriteString("Content-Type: multipart/mixed; boundary=\"parts\"\r\n\r\n")
	for i := range count {
		fmt.Fprintf(&b, "--parts\r\nContent-Type: application/octet-stream\r\n"+
			"Content-Disposition: attachment; filename=\"part-%d.bin\"\r\n\r\npart %d\r\n", i, i)
	}
	b.WriteString("--parts--\r\n")
	return []byte(b.String())
}

// nestedParts answers a raw message of depth closed multipart containers below the top one, each inside the last.
func nestedParts(depth int) []byte {
	b := bytes.NewBuffer(unclosedParts(depth))
	fmt.Fprintf(b, "--level-%04d\r\nContent-Type: text/plain\r\n\r\nThe innermost part.\r\n", depth)
	for level := depth; level >= 0; level-- {
		fmt.Fprintf(b, "--level-%04d--\r\n", level)
	}
	return b.Bytes()
}

// unclosedParts answers a raw message of depth unclosed multipart containers below the top one, each inside the last.
func unclosedParts(depth int) []byte {
	var b bytes.Buffer
	b.WriteString("Content-Type: multipart/mixed; boundary=\"level-0000\"\r\n\r\n")
	for level := 1; level <= depth; level++ {
		fmt.Fprintf(&b, "--level-%04d\r\nContent-Type: multipart/mixed; boundary=\"level-%04d\"\r\n\r\n", level-1, level)
	}
	return b.Bytes()
}

// manyParameters answers count media type parameters, each with its own name.
func manyParameters(count int) string {
	names := make([]string, count)
	for i := range names {
		names[i] = fmt.Sprintf("p%d=v", i)
	}
	return strings.Join(names, "; ")
}
