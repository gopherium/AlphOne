// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"context"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/plugins/email/message"
	"github.com/gopherium/alphone/sdk"
)

// eventMessageReceived is the event a newly stored mail from a person publishes.
const eventMessageReceived = "email.message.received"

// announcedMail is a newly stored mail as the event that announces it reads it.
type announcedMail struct {
	// id names the stored mail.
	id uuid.UUID
	// row is the mail as its row holds it.
	row messageRow
	// contacts are the contacts the mail is linked to.
	contacts []uuid.UUID
	// from is the mailbox the From field names.
	from message.Address
	// own lists the mailbox's address and every alias.
	own []string
	// backfill reports old mail a first import or a backfill brings in.
	backfill bool
}

// announces reports whether the mail publishes its event, which only an inbound mail from a person does.
func (m announcedMail) announces() bool {
	fromOwn := len(message.WithoutOwn([]message.Address{m.from}, m.own)) == 0
	return m.row.automation == message.NotAutomated && m.row.direction == inbound && !m.backfill && !fromOwn &&
		m.row.bodyState != bodyHeadersOnly
}

// eventData returns what the event of the mail carries, its text cut to at most textMaxBytes where a letter starts.
func (m announcedMail) eventData(textMaxBytes int) map[string]any {
	contacts := make([]string, 0, len(m.contacts))
	for _, contact := range m.contacts {
		contacts = append(contacts, contact.String())
	}
	text := ""
	if !m.row.encrypted {
		text = cutOnLetter(m.row.bodyText, textMaxBytes)
	}
	return map[string]any{
		"id":          m.id.String(),
		"thread_id":   m.row.threadID.String(),
		"contact_ids": contacts,
		"subject":     m.row.subject,
		"text":        text,
	}
}

// publishArrival publishes the event of the mail in the workspace tenant through publisher unless the mail is quiet.
func publishArrival(
	ctx context.Context, publisher sdk.Publisher, tenant uuid.UUID, m announcedMail, textMaxBytes int,
) {
	if m.announces() {
		publisher.Publish(sdk.WithTenant(ctx, tenant), eventMessageReceived, m.eventData(textMaxBytes))
	}
}

// cutOnLetter returns text cut to at most limit bytes, at the start of a letter.
func cutOnLetter(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	end := limit
	for end > 0 && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end]
}
