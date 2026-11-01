// SPDX-License-Identifier: Elastic-2.0

package message

import (
	"regexp"
	"strings"
)

// subjectTag matches one Re, Fw or Fwd tag opening a subject whose blanks are collapsed.
var subjectTag = regexp.MustCompile(`^(?i:re|fwd?)(?:\[\d+\]|\^\d+|\(\d+\))? ?: ?`)

// CarriesReplyTag reports whether subject opens with a Re, Fw or Fwd tag.
func CarriesReplyTag(subject string) bool {
	return subjectTag.MatchString(collapsed(subject))
}

// NormalizeSubject answers subject with its blanks collapsed and without its leading Re, Fw and Fwd tags.
func NormalizeSubject(subject string) string {
	s := collapsed(subject)
	for {
		tag := subjectTag.FindStringIndex(s)
		if tag == nil {
			return s
		}
		s = s[tag[1]:]
	}
}

// collapsed answers subject with each run of blanks turned into one and none around it.
func collapsed(subject string) string {
	return strings.Join(strings.Fields(subject), " ")
}
