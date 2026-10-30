// SPDX-License-Identifier: Elastic-2.0

package message

import (
	"net/url"
	"regexp"
	"slices"

	"github.com/microcosm-cc/bluemonday"
)

// allowedStyles lists the longhand style properties a style attribute keeps, each checked in linear time.
var allowedStyles = []string{
	"color", "background-color", "text-align", "vertical-align", "font-weight", "font-style", "font-size",
	"font-family", "width", "height", "padding-top", "padding-right", "padding-bottom", "padding-left",
	"margin-top", "margin-right", "margin-bottom", "margin-left", "border-collapse", "white-space",
}

// withoutEscapedStyles is the first pass of Sanitize, which drops every style attribute holding a backslash.
var withoutEscapedStyles = basePolicy().AllowAttrs("style").Matching(regexp.MustCompile(`^[^\\]*$`)).Globally()

// AllowedStyles answers the style properties Sanitize keeps in a style attribute.
func AllowedStyles() []string {
	return slices.Clone(allowedStyles)
}

// Sanitize answers html made safe to show, handing each image source to rewrite when rewrite is not nil.
func Sanitize(html string, rewrite func(*url.URL)) string {
	policy := basePolicy().
		RequireNoReferrerOnFullyQualifiedLinks(true).
		AddTargetBlankToFullyQualifiedLinks(true).
		RewriteSrc(rewrite).
		AllowStyles(allowedStyles...).Globally()
	return policy.Sanitize(withoutEscapedStyles.Sanitize(html))
}

// basePolicy answers the policy both passes of Sanitize start from.
func basePolicy() *bluemonday.Policy {
	return bluemonday.UGCPolicy().AllowRelativeURLs(false).AllowURLSchemes("cid").RequireNoFollowOnLinks(false)
}
