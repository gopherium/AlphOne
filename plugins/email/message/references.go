// SPDX-License-Identifier: Elastic-2.0

package message

import "slices"

// referencesField opens the References header line.
const referencesField = "References: "

// lineLimit is the longest header line the mail standard allows.
const lineLimit = 998

// ReplyReferences answers the References a reply carries, given its parent's References, In-Reply-To and Message-ID.
func ReplyReferences(references, inReplyTo []string, messageID string) []string {
	ids := slices.Clone(references)
	if len(ids) == 0 && len(inReplyTo) == 1 {
		ids = []string{inReplyTo[0]}
	}
	if messageID != "" && !slices.Contains(ids, messageID) {
		ids = append(ids, messageID)
	}
	return ids
}

// FormatReferences answers the clean ids among ids as a References value trimmed to fit one header line.
func FormatReferences(ids []string) string {
	ids = cleanIDs(ids)
	length := len(referencesField) + len(FormatIDs(ids))
	cut := 1
	for length > lineLimit && len(ids)-cut > 2 {
		length -= len("<> ") + len(ids[cut])
		cut++
	}
	if cut == 1 {
		return FormatIDs(ids)
	}
	return FormatIDs(append([]string{ids[0]}, ids[cut:]...))
}
