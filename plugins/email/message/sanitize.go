// SPDX-License-Identifier: Elastic-2.0

package message

import (
	"errors"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
)

// allowedStyles lists the longhand style properties a style attribute keeps.
var allowedStyles = []string{
	"color", "background-color", "text-align", "vertical-align", "font-weight", "font-style", "font-size",
	"font-family", "width", "height", "padding-top", "padding-right", "padding-bottom", "padding-left",
	"margin-top", "margin-right", "margin-bottom", "margin-left", "border-collapse", "white-space",
}

// withoutEscapedStyles is the base policy keeping only the style attributes that hold no backslash.
var withoutEscapedStyles = basePolicy().AllowAttrs("style").Matching(regexp.MustCompile(`^[^\\]*$`)).Globally()

// safePolicy is the policy of the last pass of Sanitize, keeping the style attributes filterStyle wrote.
var safePolicy = basePolicy().
	RequireNoReferrerOnFullyQualifiedLinks(true).
	AddTargetBlankToFullyQualifiedLinks(true).
	AllowAttrs("style").Globally()

// AllowedStyles answers the style properties Sanitize keeps in a style attribute.
func AllowedStyles() []string {
	return slices.Clone(allowedStyles)
}

// Sanitize answers html made safe to show when that fits in limit bytes, and whether it fits.
func Sanitize(html string, limit int) (string, bool) {
	first := withoutEscapedStyles.Sanitize(html)
	if strings.TrimSpace(first) == "" {
		if len(first) > limit {
			return "", false
		}
		return first, true
	}
	out := &cappedBuilder{limit: limit}
	if err := safePolicy.SanitizeReaderToWriter(withFilteredStyles(first), out); err != nil {
		return "", false
	}
	return out.String(), true
}

// withFilteredStyles answers document with the value of each style attribute put through filterStyle.
func withFilteredStyles(document string) io.Reader {
	if !strings.Contains(document, "style") {
		return strings.NewReader(document)
	}
	r := &styleRewriter{rest: document}
	r.tokens = html.NewTokenizer(&r.tag)
	return r
}

// styleRewriter hands out a document bluemonday wrote, each tag holding a style rewritten.
type styleRewriter struct {
	// rest is the part of the document not cut into pieces yet.
	rest string
	// piece is what is left of the current piece.
	piece string
	// tag holds the tag tokens reads next.
	tag strings.Reader
	// tokens reads each tag holding a style in turn.
	tokens *html.Tokenizer
}

// Read hands out the next bytes of the rewritten document.
func (r *styleRewriter) Read(p []byte) (int, error) {
	if r.piece == "" {
		if r.rest == "" {
			return 0, io.EOF
		}
		r.cut()
	}
	n := copy(p, r.piece)
	r.piece = r.piece[n:]
	return n, nil
}

// cut moves the text run or the tag at the start of rest into piece, a tag holding a style rewritten.
func (r *styleRewriter) cut() {
	end := r.endAfter('<', 0)
	tag := end == 0
	if tag {
		end = r.endAfter('>', 1)
	}
	r.piece, r.rest = r.rest[:end], r.rest[end:]
	if tag && strings.Contains(r.piece, "style") {
		r.rewriteTag()
	}
}

// endAfter answers the offset of the first c in rest moved on by past, or the length of rest when it holds no c.
func (r *styleRewriter) endAfter(c byte, past int) int {
	if i := strings.IndexByte(r.rest, c); i >= 0 {
		return i + past
	}
	return len(r.rest)
}

// rewriteTag replaces the start tag in piece with the same tag, its style filtered.
func (r *styleRewriter) rewriteTag() {
	r.tag.Reset(r.piece)
	r.tokens.Next()
	out := &strings.Builder{}
	out.Grow(len(r.piece))
	writeStartTag(out, withFilteredStyle(r.tokens.Token()))
	r.piece = out.String()
}

// withFilteredStyle answers start with its style attribute put through filterStyle, dropped when it keeps nothing.
func withFilteredStyle(start html.Token) html.Token {
	for i, attr := range start.Attr {
		if attr.Key == "style" {
			start.Attr[i].Val = filterStyle(attr.Val)
		}
	}
	start.Attr = slices.DeleteFunc(start.Attr, isEmptyStyle)
	return start
}

// isEmptyStyle reports whether attr is a style attribute with no value.
func isEmptyStyle(attr html.Attribute) bool {
	return attr.Key == "style" && attr.Val == ""
}

// attributeEscaper escapes an attribute value as golang.org/x/net/html writes it.
var attributeEscaper = strings.NewReplacer(
	"&", "&amp;", "'", "&#39;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "\r", "&#13;",
)

// writeStartTag writes start to out as its String method writes it.
func writeStartTag(out *strings.Builder, start html.Token) {
	out.WriteString("<" + start.Data)
	for _, attr := range start.Attr {
		out.WriteString(" " + attr.Key + `="`)
		_, _ = attributeEscaper.WriteString(out, attr.Val)
		out.WriteByte('"')
	}
	if start.Type == html.SelfClosingTagToken {
		out.WriteByte('/')
	}
	out.WriteByte('>')
}

// basePolicy answers the policy both passes of Sanitize start from.
func basePolicy() *bluemonday.Policy {
	return bluemonday.UGCPolicy().AllowRelativeURLs(false).AllowURLSchemes("cid").RequireNoFollowOnLinks(false)
}

// errOverCap reports a write that would take a cappedBuilder past its limit.
var errOverCap = errors.New("message: the safe HTML is over the body cap")

// cappedBuilder builds a string of at most limit bytes, failing each write that would pass it.
type cappedBuilder struct {
	// out holds the bytes written.
	out strings.Builder
	// limit is the most bytes out may hold.
	limit int
}

// Write adds p, failing when it would take the builder past its limit.
func (b *cappedBuilder) Write(p []byte) (int, error) {
	return b.WriteString(string(p))
}

// WriteString adds s, failing when it would take the builder past its limit.
func (b *cappedBuilder) WriteString(s string) (int, error) {
	if b.out.Len()+len(s) > b.limit {
		return 0, errOverCap
	}
	return b.out.WriteString(s)
}

// String answers the bytes written.
func (b *cappedBuilder) String() string {
	return b.out.String()
}
