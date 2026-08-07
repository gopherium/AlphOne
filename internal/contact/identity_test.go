// SPDX-License-Identifier: Elastic-2.0

package contact_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/contact"
)

func TestNewIdentity(t *testing.T) {
	t.Parallel()

	ownerID := uuid.Must(uuid.NewV7())

	tests := map[string]struct {
		contactID       uuid.UUID
		channel         contact.Channel
		identifier      string
		displayName     string
		wantChannel     contact.Channel
		wantIdentifier  string
		wantDisplayName string
		wantErr         error
	}{
		"valid whatsapp identity": {
			contactID:       ownerID,
			channel:         "whatsapp",
			identifier:      "184467235@lid",
			displayName:     "María",
			wantChannel:     "whatsapp",
			wantIdentifier:  "184467235@lid",
			wantDisplayName: "María",
		},
		"channel is lowercased and trimmed": {
			contactID:      ownerID,
			channel:        "  WhatsApp ",
			identifier:     "184467235@lid",
			wantChannel:    "whatsapp",
			wantIdentifier: "184467235@lid",
		},
		"identifier keeps its case": {
			contactID:      ownerID,
			channel:        "whatsapp",
			identifier:     "AbC123@Lid",
			wantChannel:    "whatsapp",
			wantIdentifier: "AbC123@Lid",
		},
		"identifier is trimmed": {
			contactID:      ownerID,
			channel:        "phone",
			identifier:     "  +34600111222  ",
			wantChannel:    "phone",
			wantIdentifier: "+34600111222",
		},
		"display name is trimmed": {
			contactID:       ownerID,
			channel:         "email",
			identifier:      "maria@acme.com",
			displayName:     "  María Pérez  ",
			wantChannel:     "email",
			wantIdentifier:  "maria@acme.com",
			wantDisplayName: "María Pérez",
		},
		"nil contact id": {
			contactID:  uuid.Nil,
			channel:    "whatsapp",
			identifier: "184467235@lid",
			wantErr:    contact.ErrNilContactID,
		},
		"empty channel": {
			contactID:  ownerID,
			channel:    " \t ",
			identifier: "184467235@lid",
			wantErr:    contact.ErrEmptyChannel,
		},
		"empty identifier": {
			contactID:  ownerID,
			channel:    "whatsapp",
			identifier: " \t ",
			wantErr:    contact.ErrEmptyIdentifier,
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			got, err := contact.NewIdentity(tc.contactID, tc.channel, tc.identifier, tc.displayName)

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("NewIdentity() error = %v, want %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			if got.ContactID != tc.contactID {
				t.Errorf("NewIdentity().ContactID = %s, want %s", got.ContactID, tc.contactID)
			}
			if got.Channel != tc.wantChannel {
				t.Errorf("NewIdentity().Channel = %q, want %q", got.Channel, tc.wantChannel)
			}
			if got.Identifier != tc.wantIdentifier {
				t.Errorf("NewIdentity().Identifier = %q, want %q", got.Identifier, tc.wantIdentifier)
			}
			if got.DisplayName != tc.wantDisplayName {
				t.Errorf("NewIdentity().DisplayName = %q, want %q", got.DisplayName, tc.wantDisplayName)
			}
		})
	}
}

func TestNewIdentityNormalizesPerChannel(t *testing.T) {
	t.Parallel()

	ownerID := uuid.Must(uuid.NewV7())

	tests := map[string]struct {
		channel    contact.Channel
		identifier string
		want       string
		wantErr    error
	}{
		"email is lowercased": {
			channel:    "email",
			identifier: " Maria.Perez@Example.COM ",
			want:       "maria.perez@example.com",
		},
		"phone keeps digits and a leading plus": {
			channel:    "phone",
			identifier: "+184 467-235",
			want:       "+184467235",
		},
		"phone drops separators and inner plus signs": {
			channel:    "phone",
			identifier: "(184) 467+235",
			want:       "184467235",
		},
		"phone with no digits is empty": {
			channel:    "phone",
			identifier: "no digits here",
			wantErr:    contact.ErrEmptyIdentifier,
		},
		"other channels stay verbatim": {
			channel:    "whatsapp",
			identifier: "AbC123@Lid",
			want:       "AbC123@Lid",
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			got, err := contact.NewIdentity(ownerID, tc.channel, tc.identifier, "")

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("NewIdentity() error = %v, want %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			if got.Identifier != tc.want {
				t.Errorf("NewIdentity().Identifier = %q, want %q", got.Identifier, tc.want)
			}
		})
	}
}

func TestIdentityExistsErrorMatchesTheSentinelAndNamesTheOwner(t *testing.T) {
	t.Parallel()

	ownerID := uuid.Must(uuid.NewV7())
	err := error(contact.IdentityExistsError{OwnerID: ownerID})

	if !errors.Is(err, contact.ErrIdentityExists) {
		t.Fatalf("errors.Is() = false, want the sentinel match")
	}
	if err.Error() != contact.ErrIdentityExists.Error() {
		t.Errorf("Error() = %q, want %q", err.Error(), contact.ErrIdentityExists.Error())
	}
	var exists contact.IdentityExistsError
	if !errors.As(err, &exists) || exists.OwnerID != ownerID {
		t.Errorf("errors.As() owner = %s, want %s", exists.OwnerID, ownerID)
	}
	if errors.Is(err, contact.ErrIdentityNotFound) {
		t.Error("errors.Is(ErrIdentityNotFound) = true, want false for an unrelated sentinel")
	}
}

func TestNewIdentityReportsIDGenerationFailure(t *testing.T) {
	ownerID := uuid.Must(uuid.NewV7())
	uuid.SetRand(failingReader{})
	defer uuid.SetRand(nil)

	_, err := contact.NewIdentity(ownerID, "whatsapp", "184467235@lid", "")

	if !errors.Is(err, errEntropy) {
		t.Fatalf("NewIdentity() error = %v, want the entropy failure in its chain", err)
	}
}

func TestNewIdentityAssignsUniqueV7IDs(t *testing.T) {
	t.Parallel()

	ownerID := uuid.Must(uuid.NewV7())

	first, err := contact.NewIdentity(ownerID, "whatsapp", "184467235@lid", "")
	if err != nil {
		t.Fatalf("NewIdentity() error = %v, want nil", err)
	}
	second, err := contact.NewIdentity(ownerID, "email", "maria@acme.com", "")
	if err != nil {
		t.Fatalf("NewIdentity() error = %v, want nil", err)
	}

	if first.ID == uuid.Nil {
		t.Error("NewIdentity().ID is uuid.Nil, want a generated UUID")
	}
	if first.ID.Version() != 7 {
		t.Errorf("NewIdentity().ID version = %d, want 7", first.ID.Version())
	}
	if first.ID == second.ID {
		t.Errorf("two identities share the ID %s, want unique IDs", first.ID)
	}
}

func TestNewIdentitySetsCreatedAtToCurrentUTCTime(t *testing.T) {
	t.Parallel()

	ownerID := uuid.Must(uuid.NewV7())

	before := time.Now().UTC()
	got, err := contact.NewIdentity(ownerID, "whatsapp", "184467235@lid", "")
	if err != nil {
		t.Fatalf("NewIdentity() error = %v, want nil", err)
	}
	after := time.Now().UTC()

	if got.CreatedAt.Before(before) || got.CreatedAt.After(after) {
		t.Errorf("NewIdentity().CreatedAt = %v, want between %v and %v", got.CreatedAt, before, after)
	}
	if got.CreatedAt.Location() != time.UTC {
		t.Errorf("NewIdentity().CreatedAt location = %v, want UTC", got.CreatedAt.Location())
	}
}

func TestNewWritableIdentity(t *testing.T) {
	t.Parallel()

	ownerID := uuid.Must(uuid.NewV7())

	tests := map[string]struct {
		channel    contact.Channel
		identifier string
		wantErr    error
	}{
		"writable email channel": {channel: "email", identifier: "maria@example.com"},
		"writable phone channel": {channel: "phone", identifier: "184467235"},
		"unwritable whatsapp channel": {
			channel: "whatsapp", identifier: "184467235@lid", wantErr: contact.ErrChannelNotWritable,
		},
		"invalid identity passes through": {
			channel: "email", identifier: "", wantErr: contact.ErrEmptyIdentifier,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := contact.NewWritableIdentity(ownerID, tc.channel, tc.identifier, "")

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("NewWritableIdentity() error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr == nil && got.Identifier != tc.identifier {
				t.Errorf("NewWritableIdentity().Identifier = %q, want %q", got.Identifier, tc.identifier)
			}
		})
	}
}
