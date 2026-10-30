// SPDX-License-Identifier: Elastic-2.0

package message

import (
	"time"

	"github.com/jhillyerd/enmime/v2"
)

// Message is one raw email read into plain values.
type Message struct {
	// From is the first mailbox the From field names, zero when it names none.
	From Address
	// To lists the mailboxes the To field names.
	To []Address
	// Cc lists the mailboxes the Cc field names.
	Cc []Address
	// ReplyTo lists the mailboxes the Reply-To field names.
	ReplyTo []Address
	// Subject is the Subject field decoded to UTF-8.
	Subject string
	// Date is when the sender wrote the message, zero when the Date field is missing or unreadable.
	Date time.Time
	// MessageID is the id the Message-ID field holds, empty when it holds none.
	MessageID string
	// InReplyTo lists the ids the In-Reply-To field holds.
	InReplyTo []string
	// References lists the ids the References field holds.
	References []string
	// ThreadIndex is the decoded Thread-Index field, nil when it is missing or unreadable.
	ThreadIndex *ThreadIndex
	// Text is the plain text body in UTF-8, converted from the HTML body when the message has no text body.
	Text string
	// HTML is the HTML body in UTF-8 as sent, unsafe to show.
	HTML string
	// Attachments lists the parts that are neither a body nor an inline image, in message order.
	Attachments []Attachment
	// InlineImages lists the images the sanitized HTML body shows through a cid link, in message order.
	InlineImages []Attachment
	// Raw is the message exactly as read.
	Raw []byte
	// root is the top of the parsed part tree.
	root *enmime.Part
}

// Attachment is one part of a message kept as a file.
type Attachment struct {
	// Name is the file name the part gives, empty when it gives none.
	Name string
	// ContentType is the media type of the part without its parameters.
	ContentType string
	// ContentID is the Content-ID of the part without brackets and percent-decoded, empty when it has none.
	ContentID string
	// Content is the decoded content of the part.
	Content []byte
}
