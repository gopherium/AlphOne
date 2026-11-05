// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/cucumber/godog"
)

// whatsAppNoNumberFeature is the feature a WhatsApp sender who shares no phone number answers to.
const whatsAppNoNumberFeature = "../../test/features/features/whatsapp-no-number.feature"

// contactOnIdentity reads the name of the contact a workspace links to a WhatsApp identifier.
const contactOnIdentity = `SELECT c.name FROM core.contacts c
	JOIN core.contact_identities i ON i.contact_id = c.id
	WHERE c.tenant_id = $1 AND i.channel = 'whatsapp' AND i.identifier = $2`

// messagesInConversation counts the messages of one conversation.
const messagesInConversation = "SELECT count(*) FROM plugin_whatsapp.messages WHERE conversation_id = $1"

// bindSenders binds the steps that deliver messages from senders with and without a phone number.
func (s *numbersScenario) bindSenders(sc *godog.ScenarioContext) {
	sc.Step(`^the person "([^"]*)" named "([^"]*)" writes with no phone number to "([^"]*)"$`, s.writeWithNoNumber)
	sc.Step(`^the person "([^"]*)" with no name writes with no phone number to "([^"]*)"$`, s.writeWithNoName)
	sc.Step(`^the person "([^"]*)" with the phone number "([^"]*)" writes to "([^"]*)"$`, s.writeWithBoth)
	sc.When(`^a message with no sender arrives for "([^"]*)"$`, s.deliverWithNoSender)
	sc.Then(`^the workspace "([^"]*)" holds a contact named "([^"]*)" on "([^"]*)"$`, s.holdsContactOn)
	sc.Then(`^the workspace "([^"]*)" holds (\d+) messages? in the conversation with "([^"]*)" on "([^"]*)"$`,
		s.holdsMessages)
	sc.Then(`^the reply names "([^"]*)" as its recipient and has no to field$`, s.replyNamesRecipient)
	sc.Then(`^the reply names "([^"]*)" in its to field and has no recipient$`, s.replyNamesTo)
}

// senderChange answers one change on number carrying the given contacts and one message.
func senderChange(number, contacts, message string) string {
	return fmt.Sprintf(`{"field":"messages","value":{"metadata":{"phone_number_id":%q},`+
		`"contacts":[%s],"messages":[%s]}}`, number, contacts, message)
}

// textMessage answers a text message element whose sender fields open it.
func (s *numbersScenario) textMessage(sender string) string {
	return fmt.Sprintf(`{%s"id":%q,"timestamp":"1751791000","type":"text","text":{"body":"hello"}}`,
		sender, s.nextMessageID())
}

// userIDSender answers the sender field of a message from userID that carries no phone number.
func userIDSender(userID string) string {
	return fmt.Sprintf(`"from_user_id":%q,`, userID)
}

// writeWithNoNumber delivers a message from userID named name to number, carrying no phone number.
func (s *numbersScenario) writeWithNoNumber(userID, name, number string) error {
	contact := fmt.Sprintf(`{"user_id":%q,"profile":{"name":%q,"username":"mariaperez"}}`, userID, name)
	s.post(numberAppSecret(number), senderChange(number, contact, s.textMessage(userIDSender(userID))))
	return s.wantStored()
}

// writeWithNoName delivers a message from userID to number, carrying no phone number and no name.
func (s *numbersScenario) writeWithNoName(userID, number string) error {
	contact := fmt.Sprintf(`{"user_id":%q}`, userID)
	s.post(numberAppSecret(number), senderChange(number, contact, s.textMessage(userIDSender(userID))))
	return s.wantStored()
}

// writeWithBoth delivers a message to number carrying both the phone number and the user ID of its sender.
func (s *numbersScenario) writeWithBoth(userID, phone, number string) error {
	contact := fmt.Sprintf(`{"wa_id":%q,"user_id":%q,"profile":{"name":"Maria Perez"}}`, phone, userID)
	sender := fmt.Sprintf(`"from":%q,"from_user_id":%q,`, phone, userID)
	s.post(numberAppSecret(number), senderChange(number, contact, s.textMessage(sender)))
	return s.wantStored()
}

// deliverWithNoSender delivers a message to number naming no sender at all.
func (s *numbersScenario) deliverWithNoSender(number string) {
	s.post(numberAppSecret(number), senderChange(number, "", s.textMessage("")))
}

// holdsContactOn fails unless the workspace called name links identifier to a contact named contact.
func (s *numbersScenario) holdsContactOn(ctx context.Context, name, contact, identifier string) error {
	var held string
	if err := s.pool.QueryRow(ctx, contactOnIdentity, s.workspaces[name], identifier).Scan(&held); err != nil {
		return fmt.Errorf("reading the contact on %s: %w", identifier, err)
	}
	if held != contact {
		return fmt.Errorf("the workspace %s names the contact on %s %q, want %q", name, identifier, held, contact)
	}
	return nil
}

// holdsMessages fails unless the conversation with customer on number holds want messages.
func (s *numbersScenario) holdsMessages(ctx context.Context, name string, want int, customer, number string) error {
	id, err := s.conversationID(ctx, conversationKey{workspace: name, customer: customer, number: number})
	if err != nil {
		return err
	}
	var held int
	if err := s.pool.QueryRow(ctx, messagesInConversation, id).Scan(&held); err != nil {
		return err
	}
	if held != want {
		return fmt.Errorf("the conversation with %s holds %d messages, want %d", customer, held, want)
	}
	return nil
}

// newestReplyFields answers the fields of the newest reply the stand-in for the Graph API received.
func (s *numbersScenario) newestReplyFields() (map[string]any, error) {
	got, sent := s.replies.last()
	if sent == 0 {
		return nil, errors.New("no reply left for WhatsApp")
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(got.body), &fields); err != nil {
		return nil, fmt.Errorf("reading the newest reply: %w", err)
	}
	return fields, nil
}

// wantAddressed fails unless the newest reply names address in field and leaves the other field out.
func (s *numbersScenario) wantAddressed(field, other, address string) error {
	fields, err := s.newestReplyFields()
	if err != nil {
		return err
	}
	if fields[field] != address {
		return fmt.Errorf("the reply names %v as %s, want %s", fields[field], field, address)
	}
	if left, named := fields[other]; named {
		return fmt.Errorf("the reply also names %v as %s, want no %s", left, other, other)
	}
	return nil
}

// replyNamesRecipient fails unless the newest reply names address as its recipient and carries no to field.
func (s *numbersScenario) replyNamesRecipient(address string) error {
	return s.wantAddressed("recipient", "to", address)
}

// replyNamesTo fails unless the newest reply names address in its to field and carries no recipient.
func (s *numbersScenario) replyNamesTo(address string) error {
	return s.wantAddressed("to", "recipient", address)
}

func TestWhatsAppNoNumber(t *testing.T) {
	runFeature(t, whatsAppNoNumberFeature, initializeWhatsAppNumbers(t))
}
