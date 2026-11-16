// SPDX-License-Identifier: Elastic-2.0

package message

import (
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// allowedStyles lists the longhand style properties a style attribute keeps.
var allowedStyles = []string{
	"color", "background-color", "text-align", "vertical-align", "font-weight", "font-style", "font-size",
	"font-family", "width", "height", "padding-top", "padding-right", "padding-bottom", "padding-left",
	"margin-top", "margin-right", "margin-bottom", "margin-left", "border-collapse", "white-space",
}

// withoutEscapedStyles is the base policy keeping only the style attributes that hold no backslash.
var withoutEscapedStyles = basePolicy().AllowAttrs("style").Matching(regexp.MustCompile(`^[^\\]*$`)).Globally()

// safePolicy is the policy of the last pass of Sanitize.
var safePolicy = basePolicy().
	RequireNoReferrerOnFullyQualifiedLinks(true).
	AddTargetBlankToFullyQualifiedLinks(true).
	AllowStyles(allowedStyles...).Globally()

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
	if err := safePolicy.SanitizeReaderToWriter(strings.NewReader(first), out); err != nil {
		return "", false
	}
	return out.String(), true
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
