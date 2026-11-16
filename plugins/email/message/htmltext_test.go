// SPDX-License-Identifier: Elastic-2.0

package message_test

import (
	"testing"
	"unicode/utf8"

	"github.com/gopherium/alphone/plugins/email/message"
)

// wideLimit is a limit no document in these tests reaches.
const wideLimit = 1 << 20

// layoutCases lists HTML documents and the text TextFromHTML lays each out as.
var layoutCases = []struct {
	name     string
	document string
	want     string
}{
	{"a run of blanks between words", "<p>Could  you\n\tsend \r\n a quote?</p>", "Could you send a quote?"},
	{"blanks at the start and the end of a line", "<p>  Could you send a quote?  </p>", "Could you send a quote?"},
	{"words split by inline elements", "Could you <b>send</b> a <i>quote</i>?", "Could you send a quote?"},
	{"bold text", "<p>Please call me about the <b>quote</b>.</p>", "Please call me about the quote."},
	{"a line break", "Maria Perez<br>184467235", "Maria Perez\n184467235"},
	{"blanks around a line break", "Maria Perez <br> 184467235", "Maria Perez\n184467235"},
	{"line breaks in a row", "Quote<br><br><br><br>Lamp", "Quote\n\nLamp"},
	{"a self-closing line break", "Quote<br/>Lamp", "Quote\nLamp"},
	{"two paragraphs", "<p>Quote</p>\n<p>Lamp</p>", "Quote\n\nLamp"},
	{"two divisions", "<div>Quote</div>\n<div>Lamp</div>", "Quote\nLamp"},
	{"a division inside text", "Quote<div>Lamp</div>Shade", "Quote\nLamp\nShade"},
	{"a line break at the end of a division", "<div>Quote<br></div><div>Lamp</div>", "Quote\nLamp"},
	{"a heading over a paragraph", "<h1>Quote</h1><p>Lamp</p>", "Quote\n\nLamp"},
	{"a heading of the last level", "<p>Quote</p><h6>Lamp</h6>Shade", "Quote\n\nLamp\n\nShade"},
	{"a rule", "Quote<hr>Lamp", "Quote\nLamp"},
	{"a section of each block kind", "<header>A</header><section>B</section><footer>C</footer>", "A\nB\nC"},
	{"an end tag of a line break", "Thanks</br>Maria Perez", "Thanks\nMaria Perez"},
	{"a centered block", "Lamp<center>Shade</center>Bulb", "Lamp\nShade\nBulb"},
	{"details with a summary", "Lamp<details><summary>Shade</summary>Bulb</details>", "Lamp\nShade\nBulb"},
	{"a legend of a field set", "Lamp<fieldset><legend>Shade</legend>Bulb</fieldset>", "Lamp\nShade\nBulb"},
	{
		"a dialog, a heading group and a search block",
		"Lamp<dialog>Shade</dialog><hgroup>Bulb</hgroup><search>Cord</search>",
		"Lamp\nShade\nBulb\nCord",
	},
	{"an example block", "Lamp<xmp>Shade</xmp>Bulb", "Lamp\n\nShade\n\nBulb"},
	{"a listing block", "Lamp<listing>Shade</listing>Bulb", "Lamp\n\nShade\n\nBulb"},
	{"a plain text block", "Lamp<plaintext>Shade", "Lamp\n\nShade"},
	{"a menu list", "Lamp<menu><li>Shade</menu>Bulb", "Lamp\n\n* Shade\n\nBulb"},
	{"a directory list", "Lamp<dir><li>Shade</dir>Bulb", "Lamp\n\n* Shade\n\nBulb"},
	{"text after a table cell end tag", "<table><td>Lamp</td>Shade</table>", "Lamp Shade"},
	{"a list", "<p>Parts:</p><ul><li>Lamp</li><li>Shade</li></ul><p>Thanks</p>", "Parts:\n\n* Lamp\n* Shade\n\nThanks"},
	{"list items with no closing tag", "<ul><li>Lamp<li>Shade</ul>", "* Lamp\n* Shade"},
	{"an ordered list", "<ol><li>Lamp</li><li>Shade</li></ol>", "* Lamp\n* Shade"},
	{"an empty list item", "<ul><li></li><li>Shade</li></ul>", "* Shade"},
	{"a nested list", "<ul><li>Lamp<ul><li>Shade</li></ul></li></ul>", "* Lamp\n\n* Shade"},
	{"a quote after a paragraph", "<p>Yes.</p><blockquote><p>Could you send a quote?</p></blockquote>",
		"Yes.\n\n> Could you send a quote?"},
	{"a quote of two paragraphs", "<blockquote><p>Quote</p><p>Lamp</p></blockquote>", "> Quote\n>\n> Lamp"},
	{"a quote with a line break", "<blockquote>Quote<br>Lamp</blockquote>", "> Quote\n> Lamp"},
	{
		"nested quotes",
		"<p>Yes.</p><blockquote>On Monday:<blockquote>Could you send a quote?</blockquote>Thanks</blockquote><p>Bye</p>",
		"Yes.\n\n> On Monday:\n>\n>> Could you send a quote?\n>\n> Thanks\n\nBye",
	},
	{"two quotes in a row", "<blockquote>Quote</blockquote><blockquote>Lamp</blockquote>", "> Quote\n\n> Lamp"},
	{"a list inside a quote", "<blockquote><ul><li>Lamp</li></ul></blockquote>", "> * Lamp"},
	{"a quote closed more often than opened", "</blockquote><blockquote>Quote</blockquote></blockquote>Lamp",
		"> Quote\n\nLamp"},
	{"a web link", `<a href="https://example.com/lamp">The lamp</a>`, "The lamp (https://example.com/lamp)"},
	{"a plain web link", `<a href="http://example.com/">The shop</a>`, "The shop (http://example.com/)"},
	{"a link whose text is its address", `<a href="https://example.com/lamp">https://example.com/lamp</a>`,
		"https://example.com/lamp"},
	{"a mail link", `<a href="mailto:maria.perez@example.com">Write to us</a>`,
		"Write to us (mailto:maria.perez@example.com)"},
	{"a mail link whose text is its address", `<a href="mailto:maria.perez@example.com">maria.perez@example.com</a>`,
		"maria.perez@example.com"},
	{"a phone link", `<a href="tel:184467235">Call us</a>`, "Call us"},
	{"a script link", `<a href="javascript:alert(1)">Open</a>`, "Open"},
	{"a link with no scheme", `<a href="/lamp">Lamp</a>`, "Lamp"},
	{"a link with no address", `<a name="top">Top</a>`, "Top"},
	{"a link with an uppercase scheme", `<a href="HTTPS://example.com/">Shop</a>`, "Shop (HTTPS://example.com/)"},
	{"a link with blanks around its address", `<a href=" https://example.com/ ">Shop</a>`,
		"Shop (https://example.com/)"},
	{"a link in a sentence", `See <a href="https://example.com/">the shop</a> today.`,
		"See the shop (https://example.com/) today."},
	{"a link left open", `<a href="https://example.com/">Shop`, "Shop (https://example.com/)"},
	{"a link with no text", `<a href="https://example.com/"></a>`, "(https://example.com/)"},
	{"an image that is the only content of a link", `<a href="https://example.com/"><img alt="Lamp shop"></a>`,
		"Lamp shop (https://example.com/)"},
	{"an image beside text in a link", `<a href="https://example.com/"><img alt="Logo"> Lamp shop</a>`,
		"Lamp shop (https://example.com/)"},
	{"an image after text in a link", `<a href="https://example.com/">Lamp shop <img alt="Logo"></a>`,
		"Lamp shop (https://example.com/)"},
	{"two images in a link", `<a href="https://example.com/"><img alt="Lamp"><img alt="Shade"></a>`,
		"(https://example.com/)"},
	{"an image with no alt text in a link", `<a href="https://example.com/"><img src="https://example.com/a.png"></a>`,
		"(https://example.com/)"},
	{"an image in a phone link", `<a href="tel:184467235"><img alt="Call us"></a>`, ""},
	{"an image outside a link", `<p>The wall <img alt="wall.png" src="cid:img1@example.com"> photo</p>`,
		"The wall photo"},
	{
		"a head",
		"<html><head><title>Quote</title><style>p { color: red }</style></head><body><p>Lamp</p></body></html>",
		"Lamp",
	},
	{"a head with no closing tag", "<head><title>Quote</title><body>Lamp", "Lamp"},
	{"a head ended by a paragraph", "<head><p>Lamp</p>", "Lamp"},
	{"a head with a meta and no end tag", `<html><head><meta charset="utf-8"><div>Lamp</div></html>`, "Lamp"},
	{"text inside a head", "<head>Lamp</head>", "Lamp"},
	{"a self-closing head", "<head/><p>Lamp</p>", "Lamp"},
	{"a head after body text", "<p>Lamp</p><head><p>Shade</p>", "Lamp\n\nShade"},
	{"a head inside a quote", "<blockquote><head><style>p {}</style><p>Lamp</p></blockquote><p>Shade</p>",
		"> Lamp\n\nShade"},
	{"a title in the body", "<title>Quote</title>Lamp", "Lamp"},
	{"a script", `<p>Lamp</p><script>document.write("Shade")</script>`, "Lamp"},
	{"a self-closing script", `<script/>document.write("Shade")</script>Lamp`, "Lamp"},
	{"a style in the body", "<style>p { color: red }</style><p>Lamp</p>", "Lamp"},
	{"a template", "<template><p>Shade</p></template><p>Lamp</p>", "Lamp"},
	{"nested templates", "<template><template>Bulb</template>Shade</template>Lamp", "Lamp"},
	{"a noscript", `<noscript><img src="https://example.com/a.png"></noscript>Lamp`, "Lamp"},
	{"an inline frame", `<iframe><p>Shade</p></iframe>Lamp`, "Lamp"},
	{"a comment and a doctype", "<!DOCTYPE html><!-- Shade --><p>Lamp</p>", "Lamp"},
	{"a self-closing title in an SVG image", "<svg><title/></svg><p>Lamp</p>", "Lamp"},
	{"a self-closing style in an SVG image", "<svg><style/></svg><p>Lamp</p>", "Lamp"},
	{"a self-closing script in an SVG image", "<svg><script/></svg><p>Lamp</p>", "Lamp"},
	{"a self-closing frame in a formula", "<math><iframe/>Lamp</math>", "Lamp"},
	{"a self-closing style after a formula token", "<p>Lamp <math><mi>x</mi><style/></math> Shade</p>", "Lamp x Shade"},
	{"a closed title in an SVG image", "<svg><title>Logo</title></svg><p>Lamp</p>", "Lamp"},
	{"a link inside a title of an SVG image", `<svg><title><a href="https://example.com/">Logo</a></title></svg>Lamp`,
		"Lamp"},
	{"a title in an SVG image ended by the image", "<svg><title>Logo</svg><p>Lamp</p>", "Lamp"},
	{"a style in an SVG image", "<svg><style>.a { fill: red }</style><text>Lamp</text></svg>", "Lamp"},
	{"a style in an SVG image ended by a bold tag", "<svg><style><b>Lamp</b></style></svg>", "Lamp"},
	{"a style in an SVG image ended by the group around it", "<svg><g><style>.a {}</g><text>Lamp</text></svg>", "Lamp"},
	{"character data in an SVG image", "<svg><text><![CDATA[Lamp]]></text></svg>", "Lamp"},
	{"character data outside an SVG image", "<![CDATA[Shade]]>Lamp", "Lamp"},
	{"a self-closing script in the HTML of an SVG image",
		"<svg><foreignObject><script/>Shade</script></foreignObject></svg>Lamp", "Lamp"},
	{"an SVG image inside the HTML of an SVG image",
		"<svg><foreignObject><p>Lamp</p><svg><title/></svg></foreignObject></svg><p>Shade</p>", "Lamp\n\nShade"},
	{"an SVG image inside a template", "<template><svg><title></template>Lamp", "Lamp"},
	{"an end tag of an SVG image inside a template",
		"<svg><foreignObject><template></svg>Shade</template></foreignObject><title/>Lamp</svg>", "Lamp"},
	{"a preformatted block", "<p>Code:</p><pre>  lamp = 1\n    shade  = 2</pre><p>Done</p>",
		"Code:\n\n  lamp = 1\n    shade  = 2\n\nDone"},
	{"a preformatted block in a quote", "<blockquote><pre>lamp\n  shade</pre></blockquote>", "> lamp\n>   shade"},
	{"blank lines in a preformatted block", "<pre>lamp\n\n\n\nshade</pre>", "lamp\n\nshade"},
	{"entities", "&lt;lamp&gt; &amp; &quot;shade&quot; caf&eacute; &#233; &#x263A;", `<lamp> & "shade" café é ☺`},
	{"a no-break space", "Lamp&nbsp;shade", "Lamp shade"},
	{"a broken byte and a NUL", "<p>Te\xffxt\x00.</p>", "Te�xt."},
	{"a NUL character reference", "Lamp&#0;", "Lamp�"},
	{
		"a table",
		"<table><tr><td>Lamp</td><td>12</td></tr><tr><th>Shade</th><td>4</td></tr></table>",
		"Lamp 12\nShade 4",
	},
	{"a table between paragraphs", "<p>Prices</p><table><tr><td>Lamp</td></tr></table><p>Thanks</p>",
		"Prices\n\nLamp\n\nThanks"},
	{"blank lines at the start and the end", "<br><br><p>Lamp</p><br><br>\n", "Lamp"},
	{"no text at all", "<p> </p><br>", ""},
}

func TestTextFromHTMLLaysOutTheText(t *testing.T) {
	t.Parallel()

	for _, c := range layoutCases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			text, cut := message.TextFromHTML(c.document, wideLimit)

			if text != c.want || cut {
				t.Errorf("TextFromHTML(%q) = %q, %t, want %q, false", c.document, text, cut, c.want)
			}
		})
	}
}

func TestTextFromHTMLStopsAtTheLimit(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		document string
		limit    int
		want     string
		cut      bool
	}{
		{"a cut inside a word", "<p>Could you send a quote?</p>", 12, "Could you se", true},
		{"a cut at a blank", "<p>Could you send a quote?</p>", 9, "Could you", true},
		{"a text at the limit", "<p>Lamp</p>", 4, "Lamp", false},
		{"blanks and empty blocks after the limit", "<p>Lamp</p>  \n<p> </p><br>", 4, "Lamp", false},
		{"a cut inside a letter", "<p>aééé</p>", 4, "aé", true},
		{"a cut inside a quote prefix", "<blockquote><blockquote><blockquote>Lamp", 2, ">>", true},
		{"a cut after a quote prefix", "<blockquote><blockquote><blockquote>Lamp", 4, ">>> ", true},
		{"a cut inside a blank line", "<p>Lamp</p><p>Shade</p>", 5, "Lamp\n", true},
		{"a cut inside a replacement letter", "Te\xff\xffxt", 3, "Te", true},
		{"a cut inside the address of a link", `<a href="https://example.com/">Shop</a>`, 10, "Shop (http", true},
		{"a cut before the address of a link", `<a href="https://example.com/">Shop</a>`, 4, "Shop", true},
		{"a limit of zero", "<p>Lamp</p>", 0, "", true},
		{"an empty document at a limit of zero", "", 0, "", false},
		{"a document of tags alone at a limit of zero", "<p></p><br>", 0, "", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			text, cut := message.TextFromHTML(c.document, c.limit)

			if text != c.want || cut != c.cut {
				t.Errorf("TextFromHTML(%q, %d) = %q, %t, want %q, %t", c.document, c.limit, text, cut, c.want, c.cut)
			}
		})
	}
}

func TestTextFromHTMLAnswersTheStartOfTheWholeText(t *testing.T) {
	t.Parallel()

	document := "<p>Yes, the café can take the lamp.</p><blockquote>On Monday:<blockquote><ul><li>Lamp ☺</li>" +
		`<li><a href="https://example.com/">Shade</a></li></ul></blockquote></blockquote><pre> a  b</pre>` + "Te\xffxt"
	whole, cut := message.TextFromHTML(document, wideLimit)
	if cut {
		t.Fatalf("TextFromHTML(%q) cut the text, want it whole", document)
	}

	for limit := range len(whole) + 2 {
		text, cut := message.TextFromHTML(document, limit)

		if want := startOf(whole, limit); text != want || cut != (limit < len(whole)) {
			t.Errorf("TextFromHTML(document, %d) = %q, %t, want %q, %t", limit, text, cut, want, limit < len(whole))
		}
		if !utf8.ValidString(text) {
			t.Errorf("TextFromHTML(document, %d) = %q, want valid UTF-8", limit, text)
		}
	}
}

// startOf answers the longest start of text within limit bytes that ends where a letter starts.
func startOf(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	end := limit
	for !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end]
}
