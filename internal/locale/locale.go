// SPDX-License-Identifier: Elastic-2.0

package locale

import (
	"errors"

	"golang.org/x/text/language"
)

// Default is the locale AlphOne answers when nothing narrows the choice.
const Default = "en-US"

// ErrUnknown reports a locale outside the supported list.
var ErrUnknown = errors.New("locale: not a supported locale")

// supported lists every locale AlphOne serves, the default first.
var supported = []string{Default, "es-ES"}

// tags holds the supported list parsed once for the matcher.
var tags = func() []language.Tag {
	parsed := make([]language.Tag, 0, len(supported))
	for _, held := range supported {
		parsed = append(parsed, language.MustParse(held))
	}
	return parsed
}()

// matcher picks the closest supported locale for an Accept-Language header.
var matcher = language.NewMatcher(tags)

// Supported returns every locale AlphOne serves, the default first.
func Supported() []string {
	held := make([]string, len(supported))
	copy(held, supported)
	return held
}

// Validate reports whether the locale is one AlphOne serves.
func Validate(candidate string) error {
	for _, held := range supported {
		if held == candidate {
			return nil
		}
	}
	return ErrUnknown
}

// Resolve returns the locale to serve from the stored choice and the Accept-Language header.
func Resolve(stored, acceptLanguage string) string {
	if Validate(stored) == nil {
		return stored
	}
	asked, _, err := language.ParseAcceptLanguage(acceptLanguage)
	if err != nil || len(asked) == 0 {
		return Default
	}
	_, index, confidence := matcher.Match(asked...)
	if confidence == language.No {
		return Default
	}
	return supported[index]
}
