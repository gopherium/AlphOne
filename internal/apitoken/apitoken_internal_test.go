// SPDX-License-Identifier: Elastic-2.0

package apitoken

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

func TestMintReportsAFailingIdentifierSource(t *testing.T) {
	uuid.SetRand(failingReader{})
	defer uuid.SetRand(nil)

	if _, err := Mint(uuid.Nil, "n8n", Full(), Never); !errors.Is(err, errEntropy) {
		t.Errorf("Mint() error = %v, want %v", err, errEntropy)
	}
}

func TestMintReportsAFailingSecretSource(t *testing.T) {
	randRead = failingReader{}.Read
	defer func() { randRead = defaultRandRead }()

	if _, err := Mint(uuid.Nil, "n8n", Full(), Never); !errors.Is(err, errEntropy) {
		t.Errorf("Mint() error = %v, want %v", err, errEntropy)
	}
}
