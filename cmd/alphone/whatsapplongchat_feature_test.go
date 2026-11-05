// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/google/uuid"
)

// whatsAppLongChatFeature is the feature a long WhatsApp chat answers to.
const whatsAppLongChatFeature = "../../test/features/features/whatsapp-long-chat.feature"

// firstChatTimestamp is the Unix time the first message of a long chat carries.
const firstChatTimestamp = 1751791000

// messageWithContent reads the message one conversation holds with the given content.
const messageWithContent = "SELECT id FROM plugin_whatsapp.messages WHERE conversation_id = $1 AND content = $2"

// bindLongChat binds the steps that read a long chat one page at a time.
func (s *numbersScenario) bindLongChat(sc *godog.ScenarioContext) {
	sc.Given(`^the customer "([^"]*)" wrote (\d+) messages to the WhatsApp number "([^"]*)"$`, s.writeMany)
	sc.When(`^the workspace "([^"]*)" reads (\d+) messages of the conversation with "([^"]*)" on "([^"]*)"$`,
		s.readNewest)
	sc.When(`^the workspace "([^"]*)" reads (\d+) messages before "([^"]*)" `+
		`of the conversation with "([^"]*)" on "([^"]*)"$`, s.readBefore)
	sc.Then(`^the messages read are "([^"]*)"$`, s.messagesRead)
	sc.Then(`^the workspace "([^"]*)" names "([^"]*)" as the oldest message `+
		`of the conversation with "([^"]*)" on "([^"]*)"$`, s.namesOldest)
}

// writeMany delivers count text messages from customer to number in one delivery, one second apart.
func (s *numbersScenario) writeMany(customer string, count int, number string) error {
	messages := make([]string, count)
	for i := range messages {
		messages[i] = fmt.Sprintf(`{"from":%q,"id":%q,"timestamp":"%d","type":"text","text":{"body":"message %d"}}`,
			customer, s.nextMessageID(), firstChatTimestamp+i, i+1)
	}
	contact := fmt.Sprintf(`{"wa_id":%q,"profile":{"name":"Maria Perez"}}`, customer)
	s.post(numberAppSecret(number), senderChange(number, contact, strings.Join(messages, ",")))
	return s.wantStored()
}

// messageID reads the id of the message with content in the conversation the key names.
func (s *numbersScenario) messageID(ctx context.Context, key conversationKey, content string) (uuid.UUID, error) {
	conversation, err := s.conversationID(ctx, key)
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	if err := s.pool.QueryRow(ctx, messageWithContent, conversation, content).Scan(&id); err != nil {
		return uuid.Nil, fmt.Errorf("reading the message %q: %w", content, err)
	}
	return id, nil
}

// readPage reads up to limit messages of the conversation the key names, before the given message when one is named.
func (s *numbersScenario) readPage(ctx context.Context, key conversationKey, limit int, before *uuid.UUID) error {
	conversation, err := s.readConversation(ctx, key)
	if err != nil {
		return err
	}
	if before != nil {
		return godog.ErrPending
	}
	messages, err := s.plugin.WhatsAppConversationResolvers().Messages(s.in(ctx, key.workspace), conversation, &limit)
	if err != nil {
		return err
	}
	s.read = make([]string, len(messages))
	for i, message := range messages {
		s.read[i] = message.Content
	}
	return nil
}

// readNewest reads up to limit messages of the conversation with customer on number, naming no starting message.
func (s *numbersScenario) readNewest(ctx context.Context, name string, limit int, customer, number string) error {
	return s.readPage(ctx, conversationKey{workspace: name, customer: customer, number: number}, limit, nil)
}

// readBefore reads up to limit messages of the conversation with customer on number that come before content.
func (s *numbersScenario) readBefore(
	ctx context.Context, name string, limit int, content, customer, number string,
) error {
	key := conversationKey{workspace: name, customer: customer, number: number}
	before, err := s.messageID(ctx, key, content)
	if err != nil {
		return err
	}
	return s.readPage(ctx, key, limit, &before)
}

// messagesRead fails unless the last read answered exactly the given messages in that order.
func (s *numbersScenario) messagesRead(want string) error {
	if got := strings.Join(s.read, ", "); got != want {
		return fmt.Errorf("the read answered %q, want %q", got, want)
	}
	return nil
}

// namesOldest fails unless the conversation with customer on number names content as its oldest message.
func (s *numbersScenario) namesOldest(ctx context.Context, name, content, customer, number string) error {
	key := conversationKey{workspace: name, customer: customer, number: number}
	if _, err := s.readConversation(ctx, key); err != nil {
		return err
	}
	if _, err := s.messageID(ctx, key, content); err != nil {
		return err
	}
	return godog.ErrPending
}

func TestWhatsAppLongChat(t *testing.T) {
	runFeature(t, whatsAppLongChatFeature, initializeWhatsAppNumbers(t))
}
