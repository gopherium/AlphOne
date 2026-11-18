// SPDX-License-Identifier: Elastic-2.0

package message_test

import (
	"errors"
	"math/rand/v2"
	"regexp"
	"strings"
	"testing"

	"github.com/microcosm-cc/bluemonday"

	"github.com/gopherium/alphone/plugins/email/message"
)

// styledDocuments is how many random documents are compared with bluemonday's style rules.
const styledDocuments = 5000

// maxStyleBytes is the most bytes a random style value holds.
const maxStyleBytes = 256

// unquotedBreakers lists the bytes an unquoted attribute value cannot hold.
const unquotedBreakers = " \t\n\r\f\"'=<>`"

// styleRulesBase answers the policy both passes of bluemonday's style rules start from.
func styleRulesBase() *bluemonday.Policy {
	return bluemonday.UGCPolicy().AllowRelativeURLs(false).AllowURLSchemes("cid").RequireNoFollowOnLinks(false)
}

// styleRulesFirst is the first pass of bluemonday's style rules, keeping the style attributes with no backslash.
var styleRulesFirst = styleRulesBase().AllowAttrs("style").Matching(regexp.MustCompile(`^[^\\]*$`)).Globally()

// styleRulesLast is the last pass of bluemonday's style rules, checking every declaration of a style attribute.
var styleRulesLast = styleRulesBase().
	RequireNoReferrerOnFullyQualifiedLinks(true).
	AddTargetBlankToFullyQualifiedLinks(true).
	AllowStyles(message.AllowedStyles()...).Globally()

// errOverLimit reports a write that would take a limitedWriter past its limit.
var errOverLimit = errors.New("the safe HTML is over the limit")

// limitedWriter keeps at most limit bytes, failing each write that would pass it.
type limitedWriter struct {
	// out holds the bytes written.
	out strings.Builder
	// limit is the most bytes out may hold.
	limit int
}

// Write adds p, failing when it would take the writer past its limit.
func (w *limitedWriter) Write(p []byte) (int, error) {
	if w.out.Len()+len(p) > w.limit {
		return 0, errOverLimit
	}
	return w.out.Write(p)
}

// sanitizedByStyleRules answers html made safe by bluemonday's style rules within limit bytes, and whether it fits.
func sanitizedByStyleRules(html string, limit int) (string, bool) {
	first := styleRulesFirst.Sanitize(html)
	if strings.TrimSpace(first) == "" {
		if len(first) > limit {
			return "", false
		}
		return first, true
	}
	out := &limitedWriter{limit: limit}
	if err := styleRulesLast.SanitizeReaderToWriter(strings.NewReader(first), out); err != nil {
		return "", false
	}
	return out.out.String(), true
}

// keptTags lists the element names bluemonday keeps.
var keptTags = []string{"p", "span", "a", "img", "br", "hr", "div", "td", "table", "tr", "b", "P", "SPAN", "A"}

// droppedTags lists the element names bluemonday drops, with or without their content.
var droppedTags = []string{
	"font", "textarea", "title", "xmp", "iframe", "script", "style", "svg", "math", "noscript", "plaintext",
}

// documentTexts lists the texts random documents hold between their tags.
var documentTexts = []string{
	"Lamp", "a < b", "x & y", `"q"`, "'q'", "&amp;", "&#13;", "&#92;", "&quot;", "\r\n", " ", "style", "style=",
	"<!--c-->", `<!-- style="x" -->`, "]]>", "<![CDATA[x]]>", "&lt;p style=&quot;x&quot;&gt;",
}

// documentAttributes lists the attributes other than a style that random tags hold.
var documentAttributes = []string{
	`href="https://example.com/"`, `href="cid:a"`, `src="cid:logo"`, `title="style"`, `title='a "b"'`, `dir=ltr`,
	`class="x"`, `alt="style=&quot;color:red&quot;"`, `data-x="1"`, `href="/rel"`,
}

// styleKeys lists the spellings of the style attribute key.
var styleKeys = []string{"style", "style", "style", "STYLE", "Style"}

// oddStyleProperties lists the property names past the allowed ones that random styles hold.
var oddStyleProperties = []string{
	"COLOR", "Font-Family", "-webkit-color", "mso-padding-top", "-moz--webkit-width", "-webkit--moz-height",
	"w\U00000130dth", "bac\U0000212Aground-color", "position", "font", "margin", "background", "x", "", " ",
	"/*c*/color", "color/*c*/", "col or", "\U000000A0color", "{color", "color}", "-->color", "<!--color",
	"\U0000FEFFcolor", "color\xff", "'color'", "url(color", "text-align ", "TEXT-ALIGN", "prince-font-size",
}

// styleProperties lists the property names random styles hold.
var styleProperties = append(message.AllowedStyles(), oddStyleProperties...)

// styleValues lists the value pieces random styles hold.
var styleValues = []string{
	"red", "RED", "Red", "#fff", "#FFFFFF", "#ffff", "#12345678", "rgb(1,2,3)", "rgb(1, 2, 3)", "rgb( 1,2,3)",
	"rgba(1,2,3,0.5)", "hsl(1,2%,3%)", "hsla(1,2%,3%,0)", "transparent", "lightsteeelblue", "blac\U0000212A",
	"10px", "-1.5em", ".5rem", "0", "5.", "1e3", "100%", "auto", "AUTO", "inherit", "initial", "unset",
	"\U00000130nherit", "arial", "Arial", "'arial black'", "&#34;arial&#34;", "sans-serif", "times new roman",
	"url(x)", "url(a;b)", "calc(1px + 2px)", "bold", "700", "450", "center", "left", "justify", "nowrap",
	"pre-wrap", "collapse", "separate", "top", "middle", "-2px", "small", "x-large", "italic", "oblique",
	"normal", "\U000000A0", "\U0000FFFE", "\xff", "\x00", "&#0;", "&#13;", "&#12;", "&amp;", "&lt;", "'", "/*",
	"*/", "<!--", "-->", "{", "}", "U+12", "#", "@x", "(", ")", ",", ",,", "\t", "\n", " ", "", "a", "-", "--",
	"'a;b'", "' '", "1", "12px/1.5",
}

// validStyleValues lists values each allowed property keeps, by lower case property name.
var validStyleValues = map[string][]string{
	"color":            {"red", "RED", "#fff", "rgb(1,2,3)", "rgba(1,2,3,0.5)", "hsl(1,2%,3%)", "inherit", "red,blue"},
	"background-color": {"#ffffff", "white", "rgb(0,0,0)", "initial", "hsla(1,2%,3%,1)"},
	"text-align":       {"center", "left", "right", "justify", "CENTER", "inherit"},
	"vertical-align":   {"top", "middle", "-2px", "10%", "baseline", "text-bottom"},
	"font-weight":      {"bold", "700", "normal", "bolder", "BOLD"},
	"font-style":       {"italic", "normal", "oblique"},
	"font-size":        {"14px", "small", "1.2em", "120%", "x-large", "-1px"},
	"font-family":      {"arial", "'times new roman'", "sans-serif", "Arial Black", "", "-apple-system"},
	"width":            {"100%", "auto", "600", "0", "10px"},
	"height":           {"20px", "auto", "1.5em"},
	"padding-top":      {"4px", "0", "-4px"},
	"padding-left":     {"4px", "0"},
	"margin-top":       {"0", "auto", "-10px"},
	"margin-left":      {"0", "auto"},
	"border-collapse":  {"collapse", "separate"},
	"white-space":      {"nowrap", "pre", "pre-line", "normal"},
}

// styleJoins lists what random styles put between the pieces of one value.
var styleJoins = []string{" ", ",", ", ", " ,", "\n", "\t", "", "\U000000A0", ",\n", "  ", " , "}

// importantFlags lists the spellings of the important flag random styles hold.
var importantFlags = []string{
	" !important", "!important", " !IMPORTANT", " ! important", " !important ", "\t!important",
}

// styleOddities lists the odd pieces random styles hold at random places.
var styleOddities = []string{
	"/*c*/", "'", "&#34;", "url(", "url(a;b)", "URL(a;b)", ")", "(", "{", "}", "<!--", "-->", "!important",
	"\x00", "&#0;", "\xff", "\U0000FEFF", "\U0000FFFE", "\U000000A0", "&#12;", "&#13;", "\r", "\t", "\n", ";", ":",
	",", "#", "@x", "U+1", "&amp;", "&lt;", "'a;b'", "/*", "*/", "\\", "&#92;", "\v", "\x01", "\U00000130",
	"\U0000212A", ".", "-", "5", "%", " ",
}

// styleMood holds the pools one random style draws its properties, colons, ends and odd pieces from.
type styleMood struct {
	// properties lists the property names of its declarations.
	properties []string
	// colons lists what comes between a property and its value.
	colons []string
	// ends lists what ends a declaration.
	ends []string
	// oddities lists the odd pieces put in at random places, none when empty.
	oddities []string
}

// styleMoods lists a mild mood that keeps more declarations and a wild one that draws from every pool.
var styleMoods = []styleMood{
	{
		properties: message.AllowedStyles(),
		colons:     []string{":", ": ", " : ", ":\n", ":\t"},
		ends:       []string{";", "; ", ";  ", " ;"},
	},
	{
		properties: styleProperties,
		colons:     []string{":", ": ", " : ", "::", "", ":\n", ":\t", ": /*c*/ ", ":&#13;", "&#58;", ": :", ":  "},
		ends:       []string{";", "; ", ";;", "}", " ;", ";\n", ";/*c*/", ";  ", "", "&#59;", "\r\n;", ";\t", "; ;"},
		oddities:   styleOddities,
	},
}

// pick answers one element of list drawn by random.
func pick(random *rand.Rand, list []string) string {
	return list[random.IntN(len(list))]
}

// oneIn answers one element of list on one draw in n, else the empty string.
func oneIn(random *rand.Rand, n int, list []string) string {
	if len(list) == 0 || random.IntN(n) != 0 {
		return ""
	}
	return pick(random, list)
}

// randomStyle answers a random style value of at most maxStyleBytes bytes.
func randomStyle(random *rand.Rand) string {
	mood := styleMoods[random.IntN(len(styleMoods))]
	var style strings.Builder
	style.WriteString(oneIn(random, 10, mood.oddities))
	for range 1 + random.IntN(4) {
		style.WriteString(mood.declaration(random))
	}
	style.WriteString(oneIn(random, 6, mood.oddities))
	value := mood.withOddity(random, style.String())
	return value[:min(len(value), maxStyleBytes)]
}

// declaration answers a random declaration with its end.
func (m styleMood) declaration(random *rand.Rand) string {
	property := pick(random, m.properties)
	return property + pick(random, m.colons) + randomValue(random, valuesFor(random, property)) +
		oneIn(random, 8, m.oddities) + oneIn(random, 10, importantFlags) + pick(random, m.ends)
}

// withOddity answers style with an odd piece put in at a random offset on one draw in eight.
func (m styleMood) withOddity(random *rand.Rand, style string) string {
	at := random.IntN(len(style) + 1)
	return style[:at] + oneIn(random, 8, m.oddities) + style[at:]
}

// valuesFor answers the values property keeps on two draws in three when it has any, else every value piece.
func valuesFor(random *rand.Rand, property string) []string {
	if valid, ok := validStyleValues[strings.ToLower(strings.TrimSpace(property))]; ok && random.IntN(3) != 0 {
		return valid
	}
	return styleValues
}

// randomValue answers 1 to 3 pieces of values joined at random.
func randomValue(random *rand.Rand, values []string) string {
	value := pick(random, values)
	for range random.IntN(3) {
		value += pick(random, styleJoins) + pick(random, values)
	}
	return value
}

// randomStyleAttribute answers a style attribute holding a random value, double quoted, single quoted or unquoted.
func randomStyleAttribute(random *rand.Rand) string {
	key, value := pick(random, styleKeys), randomStyle(random)
	switch random.IntN(4) {
	case 0:
		return key + "='" + strings.ReplaceAll(value, "'", "&#39;") + "'"
	case 1:
		if value != "" && !strings.ContainsAny(value, unquotedBreakers) {
			return key + "=" + value
		}
	}
	return key + `="` + value + `"`
}

// randomAttribute answers a random style attribute on half the draws, else another attribute.
func randomAttribute(random *rand.Rand) string {
	if random.IntN(2) == 0 {
		return randomStyleAttribute(random)
	}
	return pick(random, documentAttributes)
}

// randomTagName answers a dropped element name on one draw in five, else a kept one.
func randomTagName(random *rand.Rand) string {
	if random.IntN(5) == 0 {
		return pick(random, droppedTags)
	}
	return pick(random, keptTags)
}

// randomStartTag answers a random start tag of up to three attributes, self-closing on one draw in five.
func randomStartTag(random *rand.Rand) string {
	tag := "<" + randomTagName(random)
	for range random.IntN(4) {
		tag += " " + randomAttribute(random)
	}
	if random.IntN(5) == 0 {
		tag += "/"
	}
	return tag + ">"
}

// randomDocumentPart answers a random start tag, end tag or text.
func randomDocumentPart(random *rand.Rand) string {
	switch random.IntN(6) {
	case 0, 1:
		return randomStartTag(random)
	case 2:
		return "</" + randomTagName(random) + ">"
	default:
		return pick(random, documentTexts)
	}
}

// randomStyledDocument answers a random document of 1 to 8 parts.
func randomStyledDocument(random *rand.Rand) string {
	var document strings.Builder
	for range 1 + random.IntN(8) {
		document.WriteString(randomDocumentPart(random))
	}
	return document.String()
}

// randomLimit answers a limit of 0 to 159 bytes on one draw in five, else wideLimit.
func randomLimit(random *rand.Rand) int {
	if random.IntN(5) == 0 {
		return random.IntN(160)
	}
	return wideLimit
}

func TestSanitizeKeepsTheSameStylesAsBluemondaysStyleRules(t *testing.T) {
	t.Parallel()

	random := rand.New(rand.NewPCG(1, 2))
	kept := 0
	for range styledDocuments {
		document, limit := randomStyledDocument(random), randomLimit(random)
		got, gotFits := message.Sanitize(document, limit)
		want, wantFits := sanitizedByStyleRules(document, limit)
		if got != want || gotFits != wantFits {
			t.Fatalf("Sanitize(%q, %d) = %q, %t, want %q, %t as bluemonday's style rules answer", document, limit, got,
				gotFits, want, wantFits)
		}
		if strings.Contains(got, ` style="`) {
			kept++
		}
	}

	if kept < styledDocuments/20 {
		t.Errorf("%d of %d random documents kept a style, want at least a twentieth of them", kept, styledDocuments)
	}
}
