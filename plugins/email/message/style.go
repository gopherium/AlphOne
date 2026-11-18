// SPDX-License-Identifier: Elastic-2.0

package message

import "strings"

// important is the flag a style value loses at its end.
const important = "!important"

// vendorPrefixes lists the prefixes a style property loses before its check, once each and in this order.
var vendorPrefixes = []string{
	"-webkit-", "-moz-", "-ms-", "-o-", "mso-", "-xv-", "-atsc-", "-wap-", "-khtml-", "prince-", "-ah-", "-hp-",
	"-ro-", "-rim-", "-tc-",
}

// filterStyle answers the declarations of a style value that Sanitize keeps, empty when it keeps none.
func filterStyle(value string) string {
	kept := &keptStyle{}
	if !readDeclarations(styleText(value), kept.add) {
		return ""
	}
	return kept.out.String()
}

// styleText answers value as the style reader takes it, with a closing semicolon and every line break a line feed.
func styleText(value string) string {
	value = strings.TrimRight(value, " ")
	if !strings.HasSuffix(value, ";") {
		value += ";"
	}
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.ReplaceAll(value, "\f", "\n")
}

// styleDeclaration is one property and its value as written in a style value.
type styleDeclaration struct {
	// property is the property name, trimmed.
	property string
	// value is the value, trimmed and without a closing !important.
	value string
}

// name answers the property lower cased and stripped of its vendor prefixes.
func (d styleDeclaration) name() string {
	name := strings.ToLower(d.property)
	for _, prefix := range vendorPrefixes {
		name = strings.TrimPrefix(name, prefix)
	}
	return name
}

// keptStyle builds the declarations of a style value that pass their check.
type keptStyle struct {
	// out holds the declarations kept so far.
	out strings.Builder
}

// add keeps d when its property is allowed and its value passes that property's check.
func (k *keptStyle) add(d styleDeclaration) {
	check, ok := styleChecks[d.name()]
	if !ok || !check(strings.ToLower(d.value)) {
		return
	}
	if k.out.Len() > 0 {
		k.out.WriteString("; ")
	}
	k.out.WriteString(d.property)
	k.out.WriteString(": ")
	k.out.WriteString(d.value)
}

// readDeclarations hands each declaration of text to keep, and reports whether text reads without an error.
func readDeclarations(text string, keep func(styleDeclaration)) bool {
	r := styleReader{scanner: styleScanner{text: text}}
	r.next()
	if r.token.kind == tokenOpen {
		r.next()
	}
	for {
		switch r.token.kind {
		case tokenEnd, tokenError:
			return r.token.kind == tokenEnd
		case tokenSkipped:
			r.next()
		case tokenClose:
			r.next()
			return r.token.kind != tokenError
		default:
			d, ok := r.declaration()
			if !ok {
				return false
			}
			keep(d)
		}
	}
}

// styleReader reads the declarations of a style value token by token.
type styleReader struct {
	// scanner cuts the style value into tokens.
	scanner styleScanner
	// token is the current token.
	token styleToken
}

// next moves to the next token.
func (r *styleReader) next() {
	r.token = r.scanner.next()
}

// declaration reads the declaration at the current token, the property ending at its last colon.
func (r *styleReader) declaration() (styleDeclaration, bool) {
	var d styleDeclaration
	start := r.token.start
	for {
		switch r.token.kind {
		case tokenColon:
			d.property, start = strings.TrimSpace(r.scanner.text[start:r.token.start]), r.token.end
		case tokenSemicolon, tokenClose:
			d.value = strings.TrimSpace(withoutImportant(r.scanner.text[start:r.token.start]))
			r.skipSemicolon()
			return d, d.property != ""
		case tokenEnd, tokenError:
			return d, false
		}
		r.next()
	}
}

// skipSemicolon moves past the current token when it is a semicolon.
func (r *styleReader) skipSemicolon() {
	if r.token.kind == tokenSemicolon {
		r.next()
	}
}

// withoutImportant answers value without a closing !important and the white space before it.
func withoutImportant(value string) string {
	rest := strings.TrimRight(value, blanks)
	head := len(rest) - len(important)
	if head < 0 || !strings.EqualFold(rest[head:], important) {
		return value
	}
	return strings.TrimRight(rest[:head], blanks)
}
