// SPDX-License-Identifier: Elastic-2.0

package message

import (
	"regexp"
	"strings"
)

// subjectTag matches one Re, Fw or Fwd tag opening a subject whose blanks are collapsed.
var subjectTag = regexp.MustCompile(`^(?i:re|fwd?)(?:\[\d+\]|\^\d+|\(\d+\))? ?: ?`)

// NormalizeSubject answers subject with its blanks collapsed and without its leading Re, Fw and Fwd tags.
func NormalizeSubject(subject string) string {
	s := strings.Join(strings.Fields(subject), " ")
	for {
		tag := subjectTag.FindStringIndex(s)
		if tag == nil {
			return s
		}
		s = s[tag[1]:]
	}
}
