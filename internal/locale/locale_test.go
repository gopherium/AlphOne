// SPDX-License-Identifier: Elastic-2.0

package locale_test

import (
	"errors"
	"testing"

	"github.com/gopherium/alphone/internal/locale"
)

func TestResolvePrefersTheStoredChoice(t *testing.T) {
	t.Parallel()

	if got := locale.Resolve("es-ES", "en-US"); got != "es-ES" {
		t.Errorf("Resolve() = %q, want the stored choice over the header", got)
	}
}

func TestResolveMatchesTheClosestHeaderLanguage(t *testing.T) {
	t.Parallel()

	if got := locale.Resolve("", "es"); got != "es-ES" {
		t.Errorf("Resolve() = %q, want the bare language matched onto es-ES", got)
	}
	if got := locale.Resolve("", "es-MX, en;q=0.5"); got != "es-ES" {
		t.Errorf("Resolve() = %q, want the closest supported locale, not the tag the header named", got)
	}
}

func TestResolveFallsBackToTheDefault(t *testing.T) {
	t.Parallel()

	if got := locale.Resolve("", ""); got != "en-US" {
		t.Errorf("Resolve() = %q, want the default with nothing to go on", got)
	}
	if got := locale.Resolve("", "de-DE"); got != "en-US" {
		t.Errorf("Resolve() = %q, want the default for an unsupported language", got)
	}
	if got := locale.Resolve("", "not a header ;;;"); got != "en-US" {
		t.Errorf("Resolve() = %q, want the default for a header that does not parse", got)
	}
}

func TestResolveIgnoresAStoredChoiceNoLongerSupported(t *testing.T) {
	t.Parallel()

	if got := locale.Resolve("de-DE", "es"); got != "es-ES" {
		t.Errorf("Resolve() = %q, want an unsupported stored choice skipped", got)
	}
}

func TestValidateRefusesALocaleOutsideTheList(t *testing.T) {
	t.Parallel()

	if err := locale.Validate("es-ES"); err != nil {
		t.Errorf("Validate(es-ES) = %v, want a supported locale accepted", err)
	}
	if err := locale.Validate("de-DE"); !errors.Is(err, locale.ErrUnknown) {
		t.Errorf("Validate(de-DE) = %v, want %v", err, locale.ErrUnknown)
	}
	if err := locale.Validate(""); !errors.Is(err, locale.ErrUnknown) {
		t.Errorf("Validate(empty) = %v, want %v", err, locale.ErrUnknown)
	}
}

func TestSupportedStartsWithTheDefault(t *testing.T) {
	t.Parallel()

	supported := locale.Supported()

	if len(supported) == 0 || supported[0] != locale.Default {
		t.Errorf("Supported() = %v, want the default first", supported)
	}
}
