// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gopherium/alphone/plugins/email/message"
)

// dedupeKey returns the SHA-256 naming mail in its mailbox, from its id, sender, date and content, or its reference.
func dedupeKey(mail parsedMail) []byte {
	m := mail.message
	switch {
	case m.MessageID != "":
		return hashParts("id", slices.Concat(
			[][]byte{[]byte(m.MessageID), []byte(m.From.Email), dateText(m.Date)}, contentParts(mail))...)
	case m.From.Email != "" && !m.Date.IsZero():
		return hashParts("noid", slices.Concat([][]byte{[]byte(m.From.Email), dateText(m.Date)}, contentParts(mail))...)
	}
	return hashParts("ref", []byte(mail.in.ProviderRef))
}

// hashParts returns the SHA-256 of a leading word and parts, each one prefixed with its length.
func hashParts(word string, parts ...[]byte) []byte {
	hash := sha256.New()
	for _, part := range slices.Concat([][]byte{[]byte(word)}, parts) {
		_, _ = hash.Write(binary.BigEndian.AppendUint64(nil, uint64(len(part))))
		_, _ = hash.Write(part)
	}
	return hash.Sum(nil)
}

// dateText returns date in Unix seconds as text, empty when date is zero.
func dateText(date time.Time) []byte {
	if date.IsZero() {
		return nil
	}
	return []byte(strconv.FormatInt(date.Unix(), 10))
}

// contentParts returns the subject, text and file hashes of mail, and its raw hash when part of it went unread.
func contentParts(mail parsedMail) [][]byte {
	m := mail.message
	text := sha256.Sum256([]byte(strings.Join(strings.Fields(m.Text), " ")))
	var files [][]byte
	for _, file := range slices.Concat(m.Attachments, m.InlineImages) {
		hash := sha256.Sum256(file.Content)
		files = append(files, hash[:])
	}
	slices.SortFunc(files, bytes.Compare)
	parts := slices.Concat([][]byte{[]byte(normalizedSubject(m.Subject)), text[:]}, files)
	if bodyStateOf(mail) != bodyWhole || m.SkippedParts > 0 {
		parts = append(parts, mail.raw.sha256)
	}
	return parts
}

// normalizedSubject returns subject lowercased, without its leading reply tags and with its blanks collapsed.
func normalizedSubject(subject string) string {
	return strings.ToLower(message.NormalizeSubject(subject))
}
