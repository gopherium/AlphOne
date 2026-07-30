// SPDX-License-Identifier: Elastic-2.0

package event

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

var errEntropy = errors.New("entropy source failed")

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errEntropy
}

func TestNewReportsAFailingIdentifierSource(t *testing.T) {
	uuid.SetRand(failingReader{})
	defer uuid.SetRand(nil)

	if _, err := New(TaskCreated, nil); !errors.Is(err, errEntropy) {
		t.Errorf("New() error = %v, want %v", err, errEntropy)
	}
}
