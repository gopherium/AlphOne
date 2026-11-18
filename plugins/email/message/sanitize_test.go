// SPDX-License-Identifier: Elastic-2.0

package message_test

import (
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

// shorthands lists the style properties AllowedStyles must never hold.
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
		{"blanks with a carriage return", "\r\n", "\r\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got, kept := message.Sanitize(c.html, wideLimit); got != c.want || !kept {
				t.Errorf("Sanitize(%q) = %q, %t, want %q, true", c.html, got, kept, c.want)
			}
		})
	}
}

// sanitized answers html made safe to show within wideLimit, failing t when Sanitize drops it.
func sanitized(t *testing.T, html string) string {
	t.Helper()
	safe, kept := message.Sanitize(html, wideLimit)
	if !kept {
		t.Fatalf("Sanitize(%q) dropped the HTML under a limit of %d", html, wideLimit)
	}
	return safe
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

			if got := sanitized(t, valid); got != valid {
				t.Errorf("Sanitize(%q) = %q, want it unchanged", valid, got)
			}
			if got := sanitized(t, invalid); got != `<span>Text</span>` {
				t.Errorf("Sanitize(%q) = %q, want the style dropped", invalid, got)
			}
		})
	}
}

// styleDeclarationCases lists style values of a paragraph with the safe HTML Sanitize answers for each.
var styleDeclarationCases = []struct {
	name  string
	style string
	want  string
}{
	{"important at the end", "color: red !important", `<p style="color: red">Lamp</p>`},
	{
		"important on two declarations",
		"color: red !important; font-weight: bold !important",
		`<p style="color: red; font-weight: bold">Lamp</p>`,
	},
	{"important followed by a no-break space", "color: red !important&#160;", `<p>Lamp</p>`},
	{"dotless i in important", "color: red !\U00000131mportant", `<p>Lamp</p>`},
	{"comment inside a declaration", "color: /* note */ red", `<p>Lamp</p>`},
	{"comment before a declaration", "/* note */ color: red", `<p style="color: red">Lamp</p>`},
	{"comment hiding a semicolon", "color: red/*;*/; font-weight: bold", `<p style="font-weight: bold">Lamp</p>`},
	{"comment inside the property", "col/**/or: red", `<p>Lamp</p>`},
	{"line feed after the last semicolon", "color: red;\n", `<p>Lamp</p>`},
	{"tab after the last semicolon", "color: red;\t", `<p>Lamp</p>`},
	{"vertical tab after the last semicolon", "color: red;\v", `<p>Lamp</p>`},
	{"line feed with no semicolon", "color: red\n", `<p style="color: red">Lamp</p>`},
	{"carriage return and line feed", "color: red\r\n", `<p style="color: red">Lamp</p>`},
	{"two semicolons", "color: red;;", `<p>Lamp</p>`},
	{"semicolon alone", ";", `<p>Lamp</p>`},
	{"two colons", "color:: red", `<p>Lamp</p>`},
	{"last colon wins", "x: color: red", `<p style="color: red">Lamp</p>`},
	{"value with a colon", "color: red: blue", `<p>Lamp</p>`},
	{"colon with no value", "color:", `<p>Lamp</p>`},
	{"closing brace", "color: red} font-weight: bold", `<p style="color: red">Lamp</p>`},
	{"leading brace", "{color: red}", `<p style="color: red">Lamp</p>`},
	{"brace after a space", " {color: red}", `<p>Lamp</p>`},
	{"two leading braces", "{{color: red}", `<p>Lamp</p>`},
	{"quote right after the brace", "color: red}'", `<p>Lamp</p>`},
	{"quote after a space past the brace", "color: red} 'x", `<p style="color: red">Lamp</p>`},
	{"unclosed quote", "font-family: 'arial", `<p>Lamp</p>`},
	{"url holding a semicolon before a colour", "x: url(a;b); color: red", `<p style="color: red">Lamp</p>`},
	{"upper case URL", "x: URL(a;b); color: red", `<p>Lamp</p>`},
	{"mixed case Url", "x: Url(a;b); color: red", `<p>Lamp</p>`},
	{"quoted url", "x: url('a;b'); color: red", `<p style="color: red">Lamp</p>`},
	{"url after a digit", "x: 5url(a;b); color: red", `<p>Lamp</p>`},
	{"url after a full stop", "x: .url(a;b); color: red", `<p style="color: red">Lamp</p>`},
	{"url after a minus", "x: -url(a;b); color: red", `<p>Lamp</p>`},
	{"byte order mark before a url", "\U0000FEFFurl(a;b): x; color: red", `<p style="color: red">Lamp</p>`},
	{"byte order mark before a property", "\U0000FEFFcolor: red", `<p>Lamp</p>`},
	{"url with no closing parenthesis before a colour", "x: url(;color:red", `<p style="color: red">Lamp</p>`},
	{"attribute matcher in a property", "x~=y: red; color: red", `<p style="color: red">Lamp</p>`},
	{"no-break space before a property", "\U000000A0color: red", `<p style="color: red">Lamp</p>`},
	{
		"two prefixes in bluemonday's order",
		"-webkit--moz-color: red",
		`<p style="-webkit--moz-color: red">Lamp</p>`,
	},
	{"two prefixes in the other order", "-moz--webkit-color: red", `<p>Lamp</p>`},
	{"Kelvin sign in a colour", "color: blac\U0000212A", "<p style=\"color: blac\U0000212A\">Lamp</p>"},
	{
		"dotted capital I in a font name",
		"font-family: ar\U00000130al",
		"<p style=\"font-family: ar\U00000130al\">Lamp</p>",
	},
	{"empty font family", "font-family: ", `<p style="font-family: ">Lamp</p>`},
	{"font family of commas", "font-family: ,", `<p style="font-family: ,">Lamp</p>`},
	{"empty quotes", "font-family: ''", `<p>Lamp</p>`},
	{"keyword list", "text-align: center, left", `<p style="text-align: center, left">Lamp</p>`},
	{"length list", "width: auto,auto", `<p style="width: auto,auto">Lamp</p>`},
	{"two lengths in a list", "width: 10px,10px", `<p>Lamp</p>`},
	{"length beside a keyword", "width: auto, 10px", `<p>Lamp</p>`},
	{"two hex colours in a list", "color: #fff,#fff", `<p>Lamp</p>`},
	{"colour with a trailing comma", "color: red,", `<p>Lamp</p>`},
	{"transparent", "color: transparent", `<p>Lamp</p>`},
	{"lightsteelblue", "color: lightsteelblue", `<p>Lamp</p>`},
	{
		"entity semicolon",
		"color: red&#59; font-weight: bold",
		`<p style="color: red; font-weight: bold">Lamp</p>`,
	},
	{"entity letters", "color: &#114;ed", `<p style="color: red">Lamp</p>`},
	{"double escaped entity", "color: &amp;#114;ed", `<p>Lamp</p>`},
	{"form feed entity", "font-family: a,&#12; b", "<p style=\"font-family: a,\n b\">Lamp</p>"},
	{"carriage return entity", "font-family: a,&#13;b", "<p style=\"font-family: a,\nb\">Lamp</p>"},
	{
		"no-break space entity in a list",
		"font-family: arial,&nbsp;sans-serif",
		"<p style=\"font-family: arial,\U000000A0sans-serif\">Lamp</p>",
	},
	{"broken byte", "color: red\xff", `<p>Lamp</p>`},
	{"U+FFFE in a string", "font-family: '\U0000FFFE'", `<p>Lamp</p>`},
	{"U+FFFE outside a string", "color: red; x: \U0000FFFE", `<p style="color: red">Lamp</p>`},
	{"U+FFFF in a string before a colour", "x: '\U0000FFFF'; color: red", `<p>Lamp</p>`},
}

func TestSanitizeFiltersStyleDeclarations(t *testing.T) {
	t.Parallel()

	for _, c := range styleDeclarationCases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			html := `<p style="` + c.style + `">Lamp</p>`
			if got, kept := message.Sanitize(html, wideLimit); got != c.want || !kept {
				t.Errorf("Sanitize(%q) = %q, %t, want %q, true", html, got, kept, c.want)
			}
		})
	}
}

func TestSanitizeKeepsStyleAttributesInPlace(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		html string
		want string
	}{
		{"a link left bare by its style", `<a style="position: fixed">Lamp</a>`, `Lamp`},
		{
			"a link left bare around a bold word",
			`<a style="position: fixed"><b>Lamp</b></a> tail`,
			`<b>Lamp</b> tail`,
		},
		{"an image left bare by its style", `<img style="position: fixed">`, ``},
		{"a rule left bare by its style", `<hr style="position: fixed"/>`, `<hr/>`},
		{
			"a bare link inside a styled block",
			`<div style="color: red"><a style="position: fixed">Lamp</a> tail</div>`,
			`<div style="color: red">Lamp tail</div>`,
		},
		{
			"a link whose style is rejected",
			`<a href="https://example.com/" style="position: fixed">Lamp</a>`,
			`<a href="https://example.com/" rel="noreferrer noopener" target="_blank">Lamp</a>`,
		},
		{
			"a link whose style is kept",
			`<a href="https://example.com/" style="color: red">Lamp</a>`,
			`<a href="https://example.com/" style="color: red" rel="noreferrer noopener" target="_blank">Lamp</a>`,
		},
		{"a self-closing line break", `<br style="color: red"/>`, `<br style="color: red"/>`},
		{
			"a style between two attributes",
			`<p title="Lamp" style="color: red" dir="ltr">Lamp</p>`,
			`<p title="Lamp" style="color: red" dir="ltr">Lamp</p>`,
		},
		{
			"a style before another attribute",
			`<p style="color: red" title="Lamp">Lamp</p>`,
			`<p style="color: red" title="Lamp">Lamp</p>`,
		},
		{"an upper case style holding a rejected property", `<P STYLE="position: fixed">Lamp</P>`, `<p>Lamp</p>`},
		{"a single quoted style", `<p style='color: red'>Lamp</p>`, `<p style="color: red">Lamp</p>`},
		{"an unquoted style", `<p style=color:red>Lamp</p>`, `<p style="color: red">Lamp</p>`},
		{"a style with no value", `<p style>Lamp</p>`, `<p>Lamp</p>`},
		{"an empty style", `<p style="">Lamp</p>`, `<p>Lamp</p>`},
		{
			"a second style",
			`<p style="color: red" style="font-weight: bold">Lamp</p>`,
			`<p style="color: red">Lamp</p>`,
		},
		{
			"a link whose address holds the word style",
			`<a href="https://example.com/style">Lamp</a>`,
			`<a href="https://example.com/style" rel="noreferrer noopener" target="_blank">Lamp</a>`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got, kept := message.Sanitize(c.html, wideLimit); got != c.want || !kept {
				t.Errorf("Sanitize(%q) = %q, %t, want %q, true", c.html, got, kept, c.want)
			}
		})
	}
}

func TestSanitizeWritesAngleBracketsBesideAStyleEscaped(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		html string
		want string
	}{
		{
			"a link address beside a rejected style",
			`<a href="https://example.com/?a>b" style="position: fixed">Lamp</a>`,
			`<a href="https://example.com/?a&gt;b" rel="noreferrer noopener" target="_blank">Lamp</a>`,
		},
		{
			"a link address beside a kept style",
			`<a href="https://example.com/?a>b" style="color: red">Lamp</a>`,
			`<a href="https://example.com/?a&gt;b" style="color: red" rel="noreferrer noopener" target="_blank">Lamp</a>`,
		},
		{"an image address beside a rejected style", `<img src="cid:a>b" style="position: fixed">`, `<img src="cid:a&gt;b">`},
		{
			"a text before a styled element",
			`<p>a > b <span style="position: fixed">Lamp</span></p>`,
			`<p>a &gt; b <span>Lamp</span></p>`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got, kept := message.Sanitize(c.html, wideLimit); got != c.want || !kept {
				t.Errorf("Sanitize(%q) = %q, %t, want %q, true", c.html, got, kept, c.want)
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
				safe, _ := message.Sanitize(hostile, wideLimit)
				done <- safe
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
				safe, _ := message.Sanitize(hostile, wideLimit)
				done <- safe
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
	if got := sanitized(t, `<span style="font: bold 12px arial">Text</span>`); got != `<span>Text</span>` {
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
