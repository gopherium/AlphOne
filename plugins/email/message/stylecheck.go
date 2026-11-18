// SPDX-License-Identifier: Elastic-2.0

package message

import (
	"strings"

	"github.com/microcosm-cc/bluemonday/css"
)

// styleChecks maps each allowed style property to the check bluemonday runs on its lower cased value.
var styleChecks = allowedStyleChecks()

// lengthStyles lists the allowed styles whose bluemonday check takes a length for the whole value before its words.
var lengthStyles = []string{
	"vertical-align", "font-size", "width", "height", "padding-top", "padding-right", "padding-bottom",
	"padding-left", "margin-top", "margin-right", "margin-bottom", "margin-left",
}

// allowedStyleChecks answers bluemonday's default check of each allowed style, read part by part.
func allowedStyleChecks() map[string]func(string) bool {
	checks := make(map[string]func(string) bool, len(allowedStyles))
	for _, property := range allowedStyles {
		checks[property] = wordsOf(css.GetDefaultHandler(property))
	}
	for _, property := range lengthStyles {
		checks[property] = lengthOrWordsOf(css.GetDefaultHandler(property))
	}
	checks["color"], checks["background-color"] = isColor, isColor
	checks["font-family"] = wordsOf(isFamilyName)
	return checks
}

// isEveryPart reports whether accepts takes every comma part of value, each trimmed and lower cased.
func isEveryPart(value string, accepts func(string) bool) bool {
	for part := range strings.SplitSeq(value, ",") {
		if !accepts(strings.ToLower(strings.TrimSpace(part))) {
			return false
		}
	}
	return true
}

// wordsOf answers a check that runs handler on each comma part.
func wordsOf(handler func(string) bool) func(string) bool {
	return func(value string) bool { return isEveryPart(value, handler) }
}

// lengthOrWordsOf answers a check that takes a whole length, or runs handler on each comma part that is no length.
func lengthOrWordsOf(handler func(string) bool) func(string) bool {
	word := func(part string) bool { return !css.Length.MatchString(part) && handler(part) }
	return func(value string) bool { return css.Length.MatchString(value) || isEveryPart(value, word) }
}

// isColor reports what bluemonday's ColorHandler reports.
func isColor(value string) bool {
	return isEveryPart(value, isColorName) || css.HexRGB.MatchString(value) || css.RGB.MatchString(value) ||
		css.RGBA.MatchString(value) || css.HSL.MatchString(value) || css.HSLA.MatchString(value)
}

// isColorName reports whether part is a colour name, the word half of bluemonday's ColorHandler.
func isColorName(part string) bool {
	return !css.HexRGB.MatchString(part) && css.ColorHandler(part)
}

// isFamilyName reports whether part is empty or a font family name, as bluemonday's FontFamilyHandler takes a part.
func isFamilyName(part string) bool {
	return part == "" || css.Font.MatchString(part)
}
