// SPDX-License-Identifier: Elastic-2.0

package message

import (
	"bytes"
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// blanks lists the bytes HTML reads as white space.
const blanks = " \t\n\f\r"

// blockBreaks maps each element that starts and ends a line to the line breaks it leaves around itself.
var blockBreaks = map[string]int{
	"address": 1, "article": 1, "aside": 1, "blockquote": 2, "center": 1, "dd": 1, "details": 1, "dialog": 1,
	"dir": 2, "div": 1, "dl": 1, "dt": 1, "fieldset": 1, "figcaption": 1, "figure": 1, "footer": 1, "form": 1,
	"h1": 2, "h2": 2, "h3": 2, "h4": 2, "h5": 2, "h6": 2, "header": 1, "hgroup": 1, "hr": 1, "legend": 1, "li": 1,
	"listing": 2, "main": 1, "menu": 2, "nav": 1, "ol": 2, "p": 2, "plaintext": 2, "pre": 2, "search": 1,
	"section": 1, "summary": 1, "table": 2, "tr": 1, "ul": 2, "xmp": 2,
}

// skippedElements lists the elements whose content the text leaves out.
var skippedElements = []string{
	"title", "script", "style", "template", "iframe", "noembed", "noframes", "noscript",
}

// shownSchemes lists the link schemes whose address the text writes after the link.
var shownSchemes = []string{"http", "https", "mailto"}

// maxForeignDepth is the most open SVG and math elements the text tracks.
const maxForeignDepth = 32

// svgHTMLPoints lists the SVG elements whose content is HTML again.
var svgHTMLPoints = []string{"foreignobject", "desc", "title"}

// mathHTMLPoints lists the math elements whose content is HTML again.
var mathHTMLPoints = []string{"mi", "mo", "mn", "ms", "mtext"}

// breakouts lists the start tags that end the SVG and math elements open around them.
var breakouts = map[string]bool{
	"b": true, "big": true, "blockquote": true, "body": true, "br": true, "center": true, "code": true, "dd": true,
	"div": true, "dl": true, "dt": true, "em": true, "embed": true, "h1": true, "h2": true, "h3": true, "h4": true,
	"h5": true, "h6": true, "head": true, "hr": true, "i": true, "img": true, "li": true, "listing": true,
	"menu": true, "meta": true, "nobr": true, "ol": true, "p": true, "pre": true, "ruby": true, "s": true,
	"small": true, "span": true, "strong": true, "strike": true, "sub": true, "sup": true, "table": true,
	"tt": true, "u": true, "ul": true, "var": true,
}

// TextFromHTML answers the text document shows as plain lines up to limit bytes, and whether it had more.
func TextFromHTML(document string, limit int) (text string, cut bool) {
	w := &textWriter{limit: limit, lineStart: true}
	tokens := html.NewTokenizer(strings.NewReader(document))
	for !w.cut {
		tokens.AllowCDATA(len(w.foreign) > 0)
		switch kind := tokens.Next(); kind {
		case html.ErrorToken:
			w.closeLink()
			return w.out.String(), w.cut
		case html.TextToken:
			w.text(tokens.Text())
		case html.StartTagToken, html.SelfClosingTagToken:
			w.start(tokens, tag{name: tagName(tokens), opens: true, closed: kind == html.SelfClosingTagToken})
		case html.EndTagToken:
			w.end(tag{name: tagName(tokens)})
		}
	}
	return w.out.String(), true
}

// textWriter lays out the text of an HTML document as plain lines up to a byte limit.
type textWriter struct {
	// out holds the text written so far.
	out strings.Builder
	// limit is the most bytes out may hold.
	limit int
	// cut reports that the text had more than limit bytes.
	cut bool
	// breaks counts the line breaks due before the next word, two at most.
	breaks int
	// lineStart reports that nothing is written on the current line yet.
	lineStart bool
	// space reports that a blank is due before the next word on the current line.
	space bool
	// bullet reports that a list marker is due at the start of the next line.
	bullet bool
	// marks holds one quote mark for each quote level reached so far.
	marks []byte
	// depth is the quote level of the current line.
	depth int
	// gap is the lowest quote level since the last word, the level of a blank line before the next one.
	gap int
	// pre counts the preformatted blocks open.
	pre int
	// skipped names the element whose content is left out, empty when none is.
	skipped string
	// skipDepth counts the open elements named skipped.
	skipDepth int
	// skipFloor is the count of open foreign elements below which the skipped element has ended, zero outside them.
	skipFloor int
	// skipBase is the count of open foreign elements when a skipped element outside them started, zero otherwise.
	skipBase int
	// foreign holds the open SVG and math elements, innermost last.
	foreign []foreignElement
	// link is the open link whose address may follow its text, nil when none is.
	link *textLink
}

// tag is a start or end tag of a document.
type tag struct {
	// name is the lower case name of the tag.
	name string
	// opens reports that the tag is a start tag.
	opens bool
	// closed reports that the tag is a self-closing start tag.
	closed bool
	// foreign reports that the tag was read as SVG or math content.
	foreign bool
}

// depthStep answers how t changes the count of open elements of its name.
func (t tag) depthStep() int {
	if t.opens {
		return 1
	}
	return -1
}

// startsSkip reports whether t starts a skipped element left open.
func (t tag) startsSkip() bool {
	return slices.Contains(skippedElements, t.name) && (!t.foreign || !t.closed)
}

// opensRoot reports whether t starts an SVG or math element left open.
func (t tag) opensRoot() bool {
	return (t.name == "svg" || t.name == "math") && !t.closed
}

// foreignElement is an open SVG or math element.
type foreignElement struct {
	// name is the lower case tag name of the element.
	name string
	// math reports that the element is a math element, not an SVG one.
	math bool
}

// htmlPoint reports whether the content of e is HTML again.
func (e foreignElement) htmlPoint() bool {
	if e.math {
		return slices.Contains(mathHTMLPoints, e.name)
	}
	return slices.Contains(svgHTMLPoints, e.name)
}

// textLink is an open link to a web or mail address.
type textLink struct {
	// href is the address of the link.
	href string
	// mailbox is the address without its scheme for a mail link, empty for a web link.
	mailbox string
	// start is where the text of the link starts in the output, -1 before it has any.
	start int
	// alt is the alt text of the last image inside the link.
	alt string
	// images counts the images inside the link.
	images int
}

// text writes the content of a text token, as written inside a preformatted block and word by word elsewhere.
func (w *textWriter) text(content []byte) {
	if w.skipped != "" {
		return
	}
	if w.pre > 0 {
		w.preformatted(content)
		return
	}
	w.words(content)
}

// words writes each word of content cleaned, a run of blanks between two words one space.
func (w *textWriter) words(content []byte) {
	for len(content) > 0 && !w.cut {
		word := bytes.TrimLeft(content, blanks)
		w.space = w.space || len(word) < len(content)
		end := bytes.IndexAny(word, blanks)
		if end < 0 {
			end = len(word)
		}
		if clean := cleanBytes(word[:end]); len(clean) > 0 {
			w.write(clean)
		}
		content = word[end:]
	}
}

// preformatted writes each line of content cleaned and as written, each line end a line break.
func (w *textWriter) preformatted(content []byte) {
	for !w.cut {
		line, rest, found := bytes.Cut(content, []byte("\n"))
		if line = cleanBytes(line); len(line) > 0 {
			w.write(line)
		}
		if !found {
			return
		}
		w.lineBreak()
		content = rest
	}
}

// write writes word after the line breaks, quote marks, list marker or space due before it.
func (w *textWriter) write(word []byte) {
	w.lead()
	if w.link != nil && w.link.start < 0 {
		w.link.start = w.out.Len()
	}
	w.put(word)
}

// lead writes the line breaks, quote marks, list marker or space due before the next word.
func (w *textWriter) lead() {
	if w.breaks > 0 && w.out.Len() > 0 {
		w.put([]byte("\n"))
		if w.breaks > 1 {
			w.put(w.marks[:w.gap])
			w.put([]byte("\n"))
		}
		w.lineStart = true
	}
	if w.lineStart {
		w.startLine()
	} else if w.space {
		w.put([]byte(" "))
	}
	w.breaks, w.space, w.bullet, w.lineStart, w.gap = 0, false, false, false, w.depth
}

// startLine writes the quote marks and the list marker that open a line.
func (w *textWriter) startLine() {
	if w.depth > 0 {
		w.put(w.marks[:w.depth])
		w.put([]byte(" "))
	}
	if w.bullet {
		w.put([]byte("* "))
	}
}

// put adds text to the output, cut where a letter starts when it reaches past the limit.
func (w *textWriter) put(text []byte) {
	if w.cut {
		return
	}
	if room := max(0, w.limit-w.out.Len()); len(text) > room {
		text = cutOnLetter(text, room)
		w.cut = true
	}
	w.out.Write(text)
}

// breakLine asks for at least n line breaks before the next word, none when n is zero.
func (w *textWriter) breakLine(n int) {
	if n > 0 {
		w.breaks = max(w.breaks, n)
		w.space = false
	}
}

// lineBreak asks for one more line break before the next word, up to one blank line.
func (w *textWriter) lineBreak() {
	w.breaks = min(w.breaks+1, 2)
	w.space = false
}

// start lays out the start tag t that tokens holds.
func (w *textWriter) start(tokens *html.Tokenizer, t tag) {
	t.foreign = w.openForeign(tokens, t)
	if w.skipping(t) || w.startSkip(t) {
		return
	}
	w.layOutStart(tokens, t)
}

// layOutStart lays out the start tag t that tokens holds.
func (w *textWriter) layOutStart(tokens *html.Tokenizer, t tag) {
	w.breakLine(blockBreaks[t.name])
	switch t.name {
	case "br":
		w.lineBreak()
	case "li":
		w.bullet = true
	case "td", "th":
		w.space = true
	case "blockquote":
		w.enterQuote()
	case "pre":
		w.pre++
	case "a":
		w.openLink(tokens)
	case "img":
		w.image(tokens)
	}
}

// end lays out the end tag t.
func (w *textWriter) end(t tag) {
	t.foreign = w.closeForeign(t)
	if w.skipping(t) {
		return
	}
	w.breakLine(blockBreaks[t.name])
	switch t.name {
	case "br":
		w.lineBreak()
	case "td", "th":
		w.space = true
	case "blockquote":
		w.leaveQuote()
	case "pre":
		w.pre = max(0, w.pre-1)
	case "a":
		w.closeLink()
	}
}

// skipping reports whether t is left out, ending a closed skip.
func (w *textWriter) skipping(t tag) bool {
	if w.skipped == "" {
		return false
	}
	if w.skipFloor > 0 {
		return w.skippingForeign()
	}
	if t.name == w.skipped && !t.foreign {
		w.skipDepth += t.depthStep()
	}
	if w.skipDepth <= 0 {
		w.skipped, w.foreign, w.skipBase = "", w.foreign[:w.skipBase], 0
	}
	return true
}

// skippingForeign reports whether the skipped element inside SVG or math is still open, ending the skip once it closed.
func (w *textWriter) skippingForeign() bool {
	if len(w.foreign) >= w.skipFloor {
		return true
	}
	w.skipped, w.skipFloor = "", 0
	return false
}

// startSkip starts leaving out the element t starts when it is a skipped element left open, reporting whether it did.
func (w *textWriter) startSkip(t tag) bool {
	if !t.startsSkip() {
		return false
	}
	w.skipped, w.skipDepth = t.name, 1
	if t.foreign {
		w.skipFloor = len(w.foreign)
	} else {
		w.skipBase = len(w.foreign)
	}
	return true
}

// inForeign reports whether the next tag is read as SVG or math content.
func (w *textWriter) inForeign() bool {
	n := len(w.foreign)
	return n > 0 && !w.foreign[n-1].htmlPoint()
}

// openForeign tracks the start tag t that tokens holds, reporting whether it was read as SVG or math content.
func (w *textWriter) openForeign(tokens *html.Tokenizer, t tag) bool {
	if !w.inForeign() {
		w.openRoot(t)
		return false
	}
	if breakouts[t.name] {
		w.breakOut()
		return false
	}
	tokens.NextIsNotRawText()
	if !t.closed {
		w.pushForeign(foreignElement{name: t.name, math: w.foreign[len(w.foreign)-1].math})
	}
	return true
}

// openRoot opens the SVG or math element t starts, if t starts one left open.
func (w *textWriter) openRoot(t tag) {
	if t.opensRoot() {
		w.pushForeign(foreignElement{name: t.name, math: t.name == "math"})
	}
}

// breakOut closes the open SVG and math elements down to the innermost one whose content is HTML.
func (w *textWriter) breakOut() {
	for w.inForeign() {
		w.foreign = w.foreign[:len(w.foreign)-1]
	}
}

// pushForeign opens e unless maxForeignDepth elements are open already.
func (w *textWriter) pushForeign(e foreignElement) {
	if len(w.foreign) < maxForeignDepth {
		w.foreign = append(w.foreign, e)
	}
}

// closeForeign closes the innermost foreign element with the name of t above skipBase, reporting whether it did.
func (w *textWriter) closeForeign(t tag) bool {
	for i := len(w.foreign) - 1; i >= w.skipBase; i-- {
		if w.foreign[i].name == t.name {
			w.foreign = w.foreign[:i]
			return true
		}
	}
	return false
}

// enterQuote opens one more quote level.
func (w *textWriter) enterQuote() {
	w.depth++
	if w.depth > len(w.marks) {
		w.marks = append(w.marks, '>')
	}
}

// leaveQuote closes the innermost quote level when one is open.
func (w *textWriter) leaveQuote() {
	w.depth = max(0, w.depth-1)
	w.gap = min(w.gap, w.depth)
}

// openLink opens the link tokens holds when it leads to a web or mail address, closing any link still open.
func (w *textWriter) openLink(tokens *html.Tokenizer) {
	w.closeLink()
	href := strings.Trim(attribute(tokens, "href"), blanks)
	scheme, address, found := strings.Cut(href, ":")
	if scheme = strings.ToLower(scheme); !found || !slices.Contains(shownSchemes, scheme) {
		return
	}
	w.link = &textLink{href: href, start: -1}
	if scheme == "mailto" {
		w.link.mailbox = address
	}
}

// image notes the alt text of an image inside an open link.
func (w *textWriter) image(tokens *html.Tokenizer) {
	if w.link != nil {
		w.link.images++
		w.link.alt = attribute(tokens, "alt")
	}
}

// closeLink writes the address of the open link after its text when the text does not show it already.
func (w *textWriter) closeLink() {
	l := w.link
	if l == nil {
		return
	}
	if l.start < 0 && l.images == 1 {
		w.words([]byte(l.alt))
	}
	w.link = nil
	text := ""
	if l.start >= 0 {
		text = w.out.String()[l.start:]
	}
	shown := text == l.href || l.mailbox != "" && text == l.mailbox
	if !shown {
		w.space = true
		w.words([]byte("(" + l.href + ")"))
	}
}

// tagName answers the lower case name of the tag tokens holds.
func tagName(tokens *html.Tokenizer) string {
	name, _ := tokens.TagName()
	return string(name)
}

// attribute answers the clean value of the attribute called name of the tag tokens holds, empty when it has none.
func attribute(tokens *html.Tokenizer, name string) string {
	for more := true; more; {
		var key, value []byte
		key, value, more = tokens.TagAttr()
		if string(key) == name {
			return cleanText(string(value))
		}
	}
	return ""
}

// cleanBytes answers text without NUL bytes and with each run of broken UTF-8 turned into one replacement character.
func cleanBytes(text []byte) []byte {
	if utf8.Valid(text) && bytes.IndexByte(text, 0) < 0 {
		return text
	}
	return []byte(cleanText(string(text)))
}
