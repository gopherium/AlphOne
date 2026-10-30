// SPDX-License-Identifier: Elastic-2.0

package message

import (
	"bytes"
	"cmp"
	"strings"

	"github.com/jhillyerd/enmime/v2"
	"github.com/jhillyerd/enmime/v2/mediatype"
)

// Marks is what the parts of a message mark it as.
type Marks struct {
	// Encrypted reports an S/MIME or PGP encrypted part.
	Encrypted bool
	// Signed reports an S/MIME or PGP signed part.
	Signed bool
	// InviteMethod is the uppercased method of the first calendar part naming one, such as REQUEST, empty for none.
	InviteMethod string
	// Winmail reports a winmail.dat part in the TNEF format of Outlook.
	Winmail bool
}

// Mark answers what the parts of m mark it as, leaving every part where Parse put it.
func Mark(m *Message) Marks {
	var marks Marks
	everyPart := func(*enmime.Part) bool { return true }
	for _, part := range rootOf(m).DepthMatchAll(everyPart) {
		_, params, _, _ := mediatype.Parse(part.Header.Get("Content-Type"))
		smimeType := strings.ToLower(params["smime-type"])
		marks.Encrypted = marks.Encrypted || isEncrypted(part, smimeType)
		marks.Signed = marks.Signed || isSigned(part, smimeType)
		marks.InviteMethod = cmp.Or(marks.InviteMethod, inviteMethod(part, params["method"]))
		marks.Winmail = marks.Winmail || isWinmail(part)
	}
	return marks
}

// isEncrypted reports whether part holds S/MIME or PGP encrypted content.
func isEncrypted(part *enmime.Part, smimeType string) bool {
	switch part.ContentType {
	case "multipart/encrypted":
		return true
	case "application/pkcs7-mime", "application/x-pkcs7-mime":
		return smimeType == "enveloped-data" || smimeType == "authenveloped-data"
	case "text/plain":
		return opensWith(part, "-----BEGIN PGP MESSAGE-----")
	}
	return false
}

// isSigned reports whether part holds S/MIME or PGP signed content.
func isSigned(part *enmime.Part, smimeType string) bool {
	switch part.ContentType {
	case "multipart/signed":
		return true
	case "application/pkcs7-mime", "application/x-pkcs7-mime":
		return smimeType == "signed-data"
	case "text/plain":
		return opensWith(part, "-----BEGIN PGP SIGNED MESSAGE-----")
	}
	return false
}

// inviteMethod answers method uppercased when part is a calendar part, empty otherwise.
func inviteMethod(part *enmime.Part, method string) string {
	if part.ContentType != "text/calendar" {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(method))
}

// isWinmail reports whether part is a TNEF part by its type or its file name.
func isWinmail(part *enmime.Part) bool {
	return part.ContentType == "application/ms-tnef" || part.ContentType == "application/vnd.ms-tnef" ||
		strings.EqualFold(part.FileName, "winmail.dat") || strings.EqualFold(part.FileName, "win.dat")
}

// opensWith reports whether the content of part opens with line after any blank lines.
func opensWith(part *enmime.Part, line string) bool {
	return bytes.HasPrefix(bytes.TrimLeft(part.Content, " \t\r\n"), []byte(line))
}
