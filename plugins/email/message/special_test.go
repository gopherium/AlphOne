// SPDX-License-Identifier: Elastic-2.0

package message_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/gopherium/alphone/plugins/email/message"
)

func TestMark(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
		want message.Marks
	}{
		{"smime-encrypted.eml", string(readFixture(t, "smime-encrypted.eml")), message.Marks{Encrypted: true}},
		{
			"S/MIME authEnveloped-data", onePart("application/pkcs7-mime; smime-type=authEnveloped-data", "AAAA"),
			message.Marks{Encrypted: true},
		},
		{
			"the legacy S/MIME type", onePart("application/x-pkcs7-mime; smime-type=enveloped-data", "AAAA"),
			message.Marks{Encrypted: true},
		},
		{
			"S/MIME signed-data", onePart("application/pkcs7-mime; smime-type=signed-data", "AAAA"),
			message.Marks{Signed: true},
		},
		{"S/MIME certs-only", onePart("application/pkcs7-mime; smime-type=certs-only", "AAAA"), message.Marks{}},
		{"S/MIME with no smime-type", onePart("application/pkcs7-mime", "AAAA"), message.Marks{}},
		{"smime-signed.eml", string(readFixture(t, "smime-signed.eml")), message.Marks{Signed: true}},
		{
			"a signature of the legacy S/MIME type",
			twoParts(`multipart/signed; protocol="application/x-pkcs7-signature"`, "application/x-pkcs7-signature"),
			message.Marks{Signed: true},
		},
		{"pgp-encrypted.eml", string(readFixture(t, "pgp-encrypted.eml")), message.Marks{Encrypted: true}},
		{"pgp-signed.eml", string(readFixture(t, "pgp-signed.eml")), message.Marks{Signed: true}},
		{
			"a text part of PGP armor",
			onePart("text/plain", "-----BEGIN PGP MESSAGE-----\r\n\r\nhF4DAAAA\r\n-----END PGP MESSAGE-----"),
			message.Marks{Encrypted: true},
		},
		{
			"a text part of signed PGP armor after blank lines",
			onePart("text/plain", "\r\n\r\n-----BEGIN PGP SIGNED MESSAGE-----\r\nHash: SHA256\r\n\r\nI confirm the visit."),
			message.Marks{Signed: true},
		},
		{
			"a text part quoting PGP armor after other words",
			onePart("text/plain", "The key starts -----BEGIN PGP MESSAGE-----"),
			message.Marks{},
		},
		{"calendar-invite.eml", string(readFixture(t, "calendar-invite.eml")), message.Marks{InviteMethod: "REQUEST"}},
		{
			"a calendar part with method CANCEL", twoParts("multipart/alternative", "text/calendar; method=CANCEL"),
			message.Marks{InviteMethod: "CANCEL"},
		},
		{
			"a calendar part with a lowercase method", twoParts("multipart/alternative", "text/calendar; method=reply"),
			message.Marks{InviteMethod: "REPLY"},
		},
		{"a calendar part with no method", twoParts("multipart/mixed", "text/calendar"), message.Marks{}},
		{"winmail-dat.eml", string(readFixture(t, "winmail-dat.eml")), message.Marks{Winmail: true}},
		{"the IANA TNEF type", twoParts("multipart/mixed", "application/vnd.ms-tnef"), message.Marks{Winmail: true}},
		{
			"an octet stream named winmail.dat", twoParts("multipart/mixed", `application/octet-stream; name="winmail.dat"`),
			message.Marks{Winmail: true},
		},
		{
			"an octet stream named WIN.DAT", twoParts("multipart/mixed", `application/octet-stream; name="WIN.DAT"`),
			message.Marks{Winmail: true},
		},
		{"gmail-reply.eml", string(readFixture(t, "gmail-reply.eml")), message.Marks{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got := message.Mark(parseRaw(t, c.raw)); got != c.want {
				t.Errorf("Mark() = %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestMarkFindsNothingInAMessageParseDidNotBuild(t *testing.T) {
	t.Parallel()

	if got := message.Mark(&message.Message{}); got != (message.Marks{}) {
		t.Errorf("Mark() of a zero message = %+v, want nothing", got)
	}
}

func TestMarkedPartsStayAttachments(t *testing.T) {
	t.Parallel()

	cases := []struct {
		file        string
		contentType string
	}{
		{"smime-encrypted.eml", "application/pkcs7-mime"},
		{"smime-signed.eml", "application/pkcs7-signature"},
		{"pgp-encrypted.eml", "application/octet-stream"},
		{"pgp-signed.eml", "application/pgp-signature"},
		{"calendar-invite.eml", "text/calendar"},
		{"winmail-dat.eml", "application/ms-tnef"},
	}

	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			t.Parallel()

			m := parseFixture(t, c.file)
			before := slices.Clone(m.Attachments)

			marks := message.Mark(m)

			hasType := func(a message.Attachment) bool { return a.ContentType == c.contentType }
			if marks == (message.Marks{}) {
				t.Errorf("Mark(%s) = no mark, want one", c.file)
			}
			if !reflect.DeepEqual(m.Attachments, before) {
				t.Errorf("Mark(%s) changed the attachments to %+v, want %+v", c.file, m.Attachments, before)
			}
			if !slices.ContainsFunc(m.Attachments, hasType) {
				t.Errorf("Mark(%s) attachments = %+v, want a %s attachment", c.file, m.Attachments, c.contentType)
			}
		})
	}
}

// onePart answers a raw message of one part of contentType holding body.
func onePart(contentType, body string) string {
	return "Content-Type: " + contentType + "\r\n\r\n" + body + "\r\n"
}

// twoParts answers a raw multipart message of contentType holding a note and then a part of partType.
func twoParts(contentType, partType string) string {
	return "Content-Type: " + contentType + "; boundary=\"parts\"\r\n\r\n" +
		"--parts\r\nContent-Type: text/plain\r\n\r\nA short note.\r\n" +
		"--parts\r\nContent-Type: " + partType + "\r\n\r\nAAAA\r\n--parts--\r\n"
}
