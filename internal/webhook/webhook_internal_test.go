// SPDX-License-Identifier: Elastic-2.0

package webhook

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/event"
)

var errEntropy = errors.New("entropy source failed")

type failingReader struct{}

// Read returns the entropy failure instead of any bytes.
func (failingReader) Read([]byte) (int, error) {
	return 0, errEntropy
}

func TestNewSubscriptionReportsAFailingIdentifierSource(t *testing.T) {
	uuid.SetRand(failingReader{})
	defer uuid.SetRand(nil)

	_, err := NewSubscription(uuid.Nil, "https://example.com/h", []event.Name{event.TaskCreated})

	if !errors.Is(err, errEntropy) {
		t.Errorf("NewSubscription() error = %v, want %v", err, errEntropy)
	}
}

func TestNewSubscriptionReportsAFailingSecretSource(t *testing.T) {
	randRead = failingReader{}.Read
	defer func() { randRead = defaultRandRead }()

	_, err := NewSubscription(uuid.Nil, "https://example.com/h", []event.Name{event.TaskCreated})

	if !errors.Is(err, errEntropy) {
		t.Errorf("NewSubscription() error = %v, want %v", err, errEntropy)
	}
}

func TestNewDeliveryReportsAFailingIdentifierSource(t *testing.T) {
	sub, err := NewSubscription(uuid.Nil, "https://example.com/h", []event.Name{event.TaskCreated})
	if err != nil {
		t.Fatalf("NewSubscription() error = %v, want nil", err)
	}
	occurred, err := event.New(event.TaskCreated, nil)
	if err != nil {
		t.Fatalf("event.New() error = %v, want nil", err)
	}
	uuid.SetRand(failingReader{})
	defer uuid.SetRand(nil)

	if _, err := NewDelivery(sub, occurred); !errors.Is(err, errEntropy) {
		t.Errorf("NewDelivery() error = %v, want %v", err, errEntropy)
	}
}
