// SPDX-License-Identifier: Elastic-2.0

package sdk

import (
	"errors"
	"fmt"
	"testing"
)

func TestFieldTextErrorMatchesTheSentinelAndTheProviderError(t *testing.T) {
	t.Parallel()

	provider := errors.New("birthDate expects DATE")
	named := FieldTextError{Reason: FieldTextKindMismatch, Fields: []string{"birthDate"}, Kind: "DATE", Err: provider}

	for name, err := range map[string]error{"bare": named, "wrapped": fmt.Errorf("checking the row: %w", named)} {
		if !errors.Is(err, ErrInvalidFieldText) {
			t.Errorf("%s: errors.Is(ErrInvalidFieldText) = false, want the sentinel to match", name)
		}
		if !errors.Is(err, provider) {
			t.Errorf("%s: errors.Is(provider) = false, want the provider error to match", name)
		}
		var read FieldTextError
		if !errors.As(err, &read) || read.Reason != FieldTextKindMismatch {
			t.Errorf("%s: errors.As() = %+v, want the reason read back", name, read)
		}
	}
}

func TestFieldTextErrorReadsAsTheSentinelBesideTheProviderMessage(t *testing.T) {
	t.Parallel()

	named := FieldTextError{Reason: FieldTextFieldUnknown, Fields: []string{"shoeSize"},
		Err: errors.New("no live field holds shoeSize")}

	if got, want := named.Error(), "sdk: invalid field text: no live field holds shoeSize"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
