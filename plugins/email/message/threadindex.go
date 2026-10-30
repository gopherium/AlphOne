// SPDX-License-Identifier: Elastic-2.0

package message

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrBadThreadIndex reports a Thread-Index value Outlook would not write.
var ErrBadThreadIndex = errors.New("message: bad Thread-Index")

// threadHeaderSize is the length of a Thread-Index header block.
const threadHeaderSize = 22

// threadChildSize is the length of each Thread-Index child block.
const threadChildSize = 5

// threadReserved is the first byte of every Thread-Index.
const threadReserved = 0x01

// fileTimeEpoch is the Unix time of the FILETIME epoch, in seconds.
const fileTimeEpoch = -11644473600

// fileTimeTick is the length of one FILETIME tick.
const fileTimeTick = 100 * time.Nanosecond

// ticksPerSecond counts the FILETIME ticks in one second.
const ticksPerSecond = uint64(time.Second / fileTimeTick)

// ThreadIndex is what an Outlook Thread-Index value holds.
type ThreadIndex struct {
	// GUID is the key every message of one conversation shares.
	GUID [16]byte
	// Started is when the conversation began.
	Started time.Time
	// Replies counts the replies and forwards since the conversation began.
	Replies int
}

// DecodeThreadIndex reads an Outlook Thread-Index value, or answers [ErrBadThreadIndex].
func DecodeThreadIndex(value string) (ThreadIndex, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(value), ""))
	if err != nil {
		return ThreadIndex{}, fmt.Errorf("%w: %w", ErrBadThreadIndex, err)
	}
	childBytes := len(raw) - threadHeaderSize
	wellFormed := childBytes >= 0 && childBytes%threadChildSize == 0 && raw[0] == threadReserved
	if !wellFormed {
		return ThreadIndex{}, fmt.Errorf("%w: %d bytes", ErrBadThreadIndex, len(raw))
	}
	index := ThreadIndex{Started: fileTime(raw[1:6]), Replies: childBytes / threadChildSize}
	copy(index.GUID[:], raw[6:threadHeaderSize])
	return index, nil
}

// fileTime answers the time the five header time bytes of a Thread-Index hold.
func fileTime(b []byte) time.Time {
	ticks := (uint64(b[0])<<32 | uint64(b[1])<<24 | uint64(b[2])<<16 | uint64(b[3])<<8 | uint64(b[4])) << 24
	return time.Unix(int64(ticks/ticksPerSecond)+fileTimeEpoch, int64(ticks%ticksPerSecond*uint64(fileTimeTick))).UTC()
}
