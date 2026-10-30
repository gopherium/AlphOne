// SPDX-License-Identifier: Elastic-2.0

package message_test

import (
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gopherium/alphone/plugins/email/message"
)

// styleSamples lists a valid and an invalid value of every allowed style property, in the order AllowedStyles answers.
var styleSamples = []struct {
	property string
	valid    string
	invalid  string
}{
	{"color", "red", "nonsense"},
	{"background-color", "#ffffff", "url(https://example.com/a.png)"},
	{"text-align", "center", "middle"},
	{"vertical-align", "top", "center"},
	{"font-weight", "bold", "heavy"},
	{"font-style", "italic", "slanted"},
	{"font-size", "14px", "huge"},
	{"font-family", "arial, sans-serif", "url(x)"},
	{"width", "100%", "wide"},
	{"height", "20px", "tall"},
	{"padding-top", "4px", "auto"},
	{"padding-right", "4px", "auto"},
	{"padding-bottom", "4px", "auto"},
	{"padding-left", "4px", "auto"},
	{"margin-top", "0", "4px 8px"},
	{"margin-right", "0", "4px 8px"},
	{"margin-bottom", "0", "4px 8px"},
	{"margin-left", "0", "4px 8px"},
	{"border-collapse", "collapse", "merge"},
	{"white-space", "nowrap", "wrap"},
}

// shorthands lists the style properties bluemonday v1.0.27 checks by trying every split of their words.
var shorthands = []string{
	"animation", "background", "border", "border-bottom", "border-image", "border-image-slice", "border-left",
	"border-radius", "border-right", "border-spacing", "border-style", "border-top", "border-width", "box-shadow",
	"column-rule", "columns", "flex", "flex-flow", "font", "grid", "grid-area", "grid-column", "grid-gap", "grid-row",
	"grid-template", "list-style", "margin", "object-position", "outline", "padding", "text-decoration",
	"text-shadow", "transition",
}

func TestSanitize(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		html string
		want string
	}{
		{"a script element", `<p>Hi</p><script>alert(1)</script>`, `<p>Hi</p>`},
		{"an onclick attribute", `<p onclick="alert(1)">Hi</p>`, `<p>Hi</p>`},
		{"a javascript link", `<a href="javascript:alert(1)">Quote</a>`, `Quote`},
		{
			"an https link",
			`<a href="https://example.com/quote">Quote</a>`,
			`<a href="https://example.com/quote" rel="noreferrer noopener" target="_blank">Quote</a>`,
		},
		{"a mailto link", `<a href="mailto:info@example.com">Info</a>`, `<a href="mailto:info@example.com">Info</a>`},
		{"a relative link", `<a href="/admin">Admin</a>`, `Admin`},
		{"a cid image", `<img src="cid:img1@example.com">`, `<img src="cid:img1@example.com">`},
		{"a data image", `<img src="data:image/png;base64,iVBORw0KGgo=">`, ``},
		{
			"an https image",
			`<img src="https://example.com/logo.png" alt="Logo">`,
			`<img src="https://example.com/logo.png" alt="Logo">`,
		},
		{"a style element", `<style>p { color: red }</style><p>Hi</p>`, `<p>Hi</p>`},
		{
			"a table",
			`<table><tr><td>Total</td><td>120</td></tr></table>`,
			`<table><tr><td>Total</td><td>120</td></tr></table>`,
		},
		{"an iframe", `<iframe src="https://example.com/frame"></iframe>`, ``},
		{"a form", `<form action="https://example.com/form"><input name="q"></form>`, ``},
		{"an object", `<object data="https://example.com/movie.swf"></object>`, ``},
		{"an embed", `<embed src="https://example.com/movie.swf">`, ``},
		{
			"an allowed style beside a refused one",
			`<p style="color: red; position: fixed">Hi</p>`,
			`<p style="color: red">Hi</p>`,
		},
		{"a style holding an escape", `<span style="color: \72 ed">Text</span>`, `<span>Text</span>`},
		{
			"a style holding an escape written as an entity",
			`<span style="color: &#92;72 ed">Text</span>`,
			`<span>Text</span>`,
		},
		{
			"a valid style beside an escaped one",
			`<span style="color: red; font-family: \61 rial">Text</span>`,
			`<span>Text</span>`,
		},
		{
			"a whole document",
			`<html><head><title>Quote</title></head><body><p>Hi</p></body></html>`,
			`<p>Hi</p>`,
		},
		{"empty", ``, ``},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got := message.Sanitize(c.html, nil); got != c.want {
				t.Errorf("Sanitize(%q) = %q, want %q", c.html, got, c.want)
			}
		})
	}
}

func TestSanitizeHandsEachImageSourceToTheRewrite(t *testing.T) {
	t.Parallel()

	var seen []string
	rewrite := func(source *url.URL) {
		seen = append(seen, source.String())
		if source.Scheme == "cid" {
			*source = url.URL{Path: "/files/" + source.Opaque}
		}
	}

	got := message.Sanitize(`<img src="cid:img1@example.com"><img src="https://example.com/logo.png">`, rewrite)

	if want := `<img src="/files/img1@example.com"><img src="https://example.com/logo.png">`; got != want {
		t.Errorf("Sanitize() = %q, want %q", got, want)
	}
	if want := []string{"cid:img1@example.com", "https://example.com/logo.png"}; !slices.Equal(seen, want) {
		t.Errorf("rewrite saw %q, want %q", seen, want)
	}
}

func TestStyleSamplesCoverEveryAllowedStyle(t *testing.T) {
	t.Parallel()

	var sampled []string
	for _, s := range styleSamples {
		sampled = append(sampled, s.property)
	}

	if allowed := message.AllowedStyles(); !slices.Equal(sampled, allowed) {
		t.Errorf("styleSamples cover %q, want every allowed style %q", sampled, allowed)
	}
}

func TestAllowedStylesCannotBeChanged(t *testing.T) {
	t.Parallel()

	message.AllowedStyles()[0] = "font"

	if got := message.AllowedStyles()[0]; got == "font" {
		t.Errorf("AllowedStyles()[0] = %q after a caller changed its copy", got)
	}
}

func TestSanitizeChecksEachAllowedStyle(t *testing.T) {
	t.Parallel()

	for _, s := range styleSamples {
		t.Run(s.property, func(t *testing.T) {
			t.Parallel()

			valid := `<span style="` + s.property + `: ` + s.valid + `">Text</span>`
			invalid := `<span style="` + s.property + `: ` + s.invalid + `">Text</span>`

			if got := message.Sanitize(valid, nil); got != valid {
				t.Errorf("Sanitize(%q) = %q, want it unchanged", valid, got)
			}
			if got := message.Sanitize(invalid, nil); got != `<span>Text</span>` {
				t.Errorf("Sanitize(%q) = %q, want the style dropped", invalid, got)
			}
		})
	}
}

func TestEveryAllowedStyleFinishesOnAHostileValue(t *testing.T) {
	for _, s := range styleSamples {
		t.Run(s.property, func(t *testing.T) {
			hostile := `<span style="` + s.property + `: ` + strings.Repeat(s.valid+" ", 39) + s.invalid + `">Text</span>`
			done := make(chan string, 1)
			timer := time.NewTimer(deadline)
			defer timer.Stop()

			go func() {
				done <- message.Sanitize(hostile, nil)
			}()

			select {
			case got := <-done:
				if got != `<span>Text</span>` {
					t.Errorf("Sanitize() of 40 words of %s = %q, want the style dropped", s.property, got)
				}
			case <-timer.C:
				t.Fatalf("Sanitize() of 40 words of %s took over %v", s.property, deadline)
			}
		})
	}
}

func TestSanitizeFinishesOnALongRunOfEscapes(t *testing.T) {
	cases := []struct {
		name   string
		escape string
	}{
		{"backslashes", `\61 `},
		{"backslashes written as entities", `&#92;61 `},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hostile := `<span style="position: ` + strings.Repeat(c.escape, 512<<10/len(c.escape)) + `">Text</span>`
			done := make(chan string, 1)
			timer := time.NewTimer(deadline)
			defer timer.Stop()

			go func() {
				done <- message.Sanitize(hostile, nil)
			}()

			select {
			case got := <-done:
				if got != `<span>Text</span>` {
					t.Errorf("Sanitize() of %d bytes of %s = %q, want the style dropped", len(hostile), c.name, got)
				}
			case <-timer.C:
				t.Fatalf("Sanitize() of %d bytes of %s took over %v", len(hostile), c.name, deadline)
			}
		})
	}
}

func TestFontIsNeverAllowed(t *testing.T) {
	t.Parallel()

	if slices.Contains(message.AllowedStyles(), "font") {
		t.Errorf("AllowedStyles() = %q, want no font", message.AllowedStyles())
	}
	if got := message.Sanitize(`<span style="font: bold 12px arial">Text</span>`, nil); got != `<span>Text</span>` {
		t.Errorf("Sanitize() of a valid font style = %q, want the style dropped", got)
	}
}

func TestNoShorthandIsAllowed(t *testing.T) {
	t.Parallel()

	for _, property := range message.AllowedStyles() {
		if slices.Contains(shorthands, property) {
			t.Errorf("AllowedStyles() holds %s, a style bluemonday checks by trying every split of its words", property)
		}
	}
}
