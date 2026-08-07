// SPDX-License-Identifier: Elastic-2.0

package sdk

import (
	"errors"
	"testing"
)

func TestGraphErrorReportsAndUnwrapsTheUnderlyingError(t *testing.T) {
	t.Parallel()

	underlying := errors.New("conversation not found")
	coded := GraphError{Code: "NOT_FOUND", Err: underlying}

	if coded.Error() != "conversation not found" {
		t.Errorf("Error() = %q, want the underlying message", coded.Error())
	}
	if !errors.Is(coded, underlying) {
		t.Error("errors.Is() = false, want the underlying error to match")
	}
}
