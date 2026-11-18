// SPDX-License-Identifier: Elastic-2.0

package message

import (
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"golang.org/x/net/html"
)

func TestCappedBuilderWritesUpToItsLimit(t *testing.T) {
	t.Parallel()

	b := &cappedBuilder{limit: 6}

	if n, err := b.Write([]byte("Lamp")); n != 4 || err != nil {
		t.Errorf("Write(%q) = %d, %v, want 4, nil", "Lamp", n, err)
	}
	if n, err := b.Write([]byte("Shade")); n != 0 || !errors.Is(err, errOverCap) {
		t.Errorf("Write(%q) past the limit = %d, %v, want 0, %v", "Shade", n, err, errOverCap)
	}
	if got := b.String(); got != "Lamp" {
		t.Errorf("String() = %q, want %q", got, "Lamp")
	}
}

func TestWithFilteredStylesAnswersADocumentWithNoStyleAsItIs(t *testing.T) {
	t.Parallel()

	document := `<p title="x">Lamp</p>`

	reader, ok := withFilteredStyles(document).(*strings.Reader)
	if !ok {
		t.Fatalf("withFilteredStyles(%q) is not a *strings.Reader", document)
	}
	if got, err := io.ReadAll(reader); string(got) != document || err != nil {
		t.Errorf("withFilteredStyles(%q) reads %q, %v, want %q, nil", document, got, err, document)
	}
}

func TestStyleRewriterRewritesOnlyTagsWithAStyle(t *testing.T) {
	t.Parallel()

	document := `<p style="color:red">Lamp</p><a title="x">style</a>tail`
	want := `<p style="color: red">Lamp</p><a title="x">style</a>tail`

	if got, err := io.ReadAll(iotest.OneByteReader(withFilteredStyles(document))); string(got) != want || err != nil {
		t.Errorf("withFilteredStyles(%q) reads %q, %v, want %q, nil", document, got, err, want)
	}
}

func TestStyleRewriterWritesEachStyledTagAsTokenStringWrites(t *testing.T) {
	t.Parallel()

	tags := []string{
		`<p dir="ltr" style="color:red" title="a&amp;&#39;&lt;&gt;&#34;&#13;b">`,
		`<span style="font-family: ` + strings.Repeat("a,", 4<<10) + `a">`,
		`<br style="color:red"/>`,
		`<img style="position:fixed" alt="x"/>`,
		`<td style="padding-top: ` + strings.Repeat("4px ", 5<<10) + `">`,
		`<a href="https://example.com/style">`,
		`<span style="color: red">`,
	}
	var document, want strings.Builder
	for _, tag := range tags {
		tokens := html.NewTokenizer(strings.NewReader(tag))
		tokens.Next()
		document.WriteString(tag + "Lamp")
		want.WriteString(withFilteredStyle(tokens.Token()).String() + "Lamp")
	}

	if got, err := io.ReadAll(withFilteredStyles(document.String())); string(got) != want.String() || err != nil {
		t.Errorf("withFilteredStyles(%.80q) reads %.80q, %v, want %.80q, nil", document.String(), got, err, want.String())
	}
}
