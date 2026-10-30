// SPDX-License-Identifier: Elastic-2.0

package message_test

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/gopherium/alphone/plugins/email/message"
)

// threadStart is the time the test conversations begin.
var threadStart = time.Date(2026, time.October, 26, 10, 15, 0, 0, time.UTC)

// threadGUID is the conversation key the test headers carry.
var threadGUID = [16]byte{
	0x5a, 0x0e, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66,
}

// headerPrecision is the span one unit of a Thread-Index header time covers.
const headerPrecision = (1 << 24) * 100 * time.Nanosecond

func TestDecodeThreadIndex(t *testing.T) {
	t.Parallel()

	header := threadHeader(0x01, threadStart, threadGUID)
	cases := []struct {
		name    string
		value   string
		replies int
	}{
		{"a header block only", encode(header), 0},
		{"a header and two child blocks", encode(header, childBlock(1), childBlock(2)), 2},
		{"the value folded with blanks", fold(encode(header, childBlock(1))), 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got, err := message.DecodeThreadIndex(c.value)

			if err != nil {
				t.Fatalf("DecodeThreadIndex(%q) error = %v, want nil", c.value, err)
			}
			if got.GUID != threadGUID {
				t.Errorf("DecodeThreadIndex(%q).GUID = %x, want %x", c.value, got.GUID, threadGUID)
			}
			if got.Replies != c.replies {
				t.Errorf("DecodeThreadIndex(%q).Replies = %d, want %d", c.value, got.Replies, c.replies)
			}
			if lag := threadStart.Sub(got.Started); lag < 0 || lag >= headerPrecision {
				t.Errorf("DecodeThreadIndex(%q).Started = %v, want %v within the header precision", c.value, got.Started,
					threadStart)
			}
		})
	}
}

func TestDecodeThreadIndexRefusesWhatOutlookNeverWrites(t *testing.T) {
	t.Parallel()

	header := threadHeader(0x01, threadStart, threadGUID)
	cases := []struct {
		name  string
		value string
	}{
		{"a first byte other than 0x01", encode(threadHeader(0x02, threadStart, threadGUID))},
		{"21 bytes", encode(header[:21])},
		{"23 bytes", encode(header, []byte{0x01})},
		{"25 bytes", encode(header, []byte{0x01, 0x02, 0x03})},
		{"not Base64", "not a thread index"},
		{"empty", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if _, err := message.DecodeThreadIndex(c.value); !errors.Is(err, message.ErrBadThreadIndex) {
				t.Errorf("DecodeThreadIndex(%q) error = %v, want ErrBadThreadIndex", c.value, err)
			}
		})
	}
}

func TestAReplySharesItsParentsConversation(t *testing.T) {
	t.Parallel()

	parentBytes := threadHeader(0x01, threadStart, threadGUID)
	parent, err := message.DecodeThreadIndex(encode(parentBytes))
	if err != nil {
		t.Fatalf("DecodeThreadIndex(parent) error = %v, want nil", err)
	}

	reply, err := message.DecodeThreadIndex(encode(parentBytes, childBlock(7)))

	if err != nil {
		t.Fatalf("DecodeThreadIndex(reply) error = %v, want nil", err)
	}
	if reply.GUID != parent.GUID || !reply.Started.Equal(parent.Started) {
		t.Errorf("reply = %x started %v, want the parent's %x started %v", reply.GUID, reply.Started, parent.GUID,
			parent.Started)
	}
	if reply.Replies != parent.Replies+1 {
		t.Errorf("reply.Replies = %d, want %d", reply.Replies, parent.Replies+1)
	}
}

// threadHeader builds a Thread-Index header block from its first byte, start time and GUID.
func threadHeader(first byte, started time.Time, guid [16]byte) []byte {
	ticks := uint64(started.Unix()+11644473600)*10_000_000 + uint64(started.Nanosecond()/100)
	block := []byte{first, byte(ticks >> 56), byte(ticks >> 48), byte(ticks >> 40), byte(ticks >> 32), byte(ticks >> 24)}
	return append(block, guid[:]...)
}

// childBlock builds one five byte Thread-Index child block.
func childBlock(seed byte) []byte {
	return []byte{0x00, 0x00, 0x10, seed, seed}
}

// encode answers the blocks joined and Base64 encoded.
func encode(blocks ...[]byte) string {
	var joined []byte
	for _, block := range blocks {
		joined = append(joined, block...)
	}
	return base64.StdEncoding.EncodeToString(joined)
}

// fold answers value broken over a line break, a tab and a blank.
func fold(value string) string {
	return value[:8] + "\r\n\t" + value[8:16] + " " + value[16:]
}
