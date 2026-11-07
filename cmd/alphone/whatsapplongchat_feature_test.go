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
	sc.Given(`^the workspace "([^"]*)" opens the conversation with "([^"]*)" on "([^"]*)"$`, s.opensConversation)
	sc.When(`^it reads (\d+) messages(?: before "([^"]*)")?$`, s.readMessages)
	sc.Then(`^the messages read are "([^"]*)"$`, s.messagesRead)
	sc.Then(`^it names "([^"]*)" as the oldest message$`, s.namesOldest)
}

// opensConversation keeps the conversation with customer on number in the workspace called name for the reads after it.
func (s *numbersScenario) opensConversation(name, customer, number string) {
	s.opened = conversationKey{workspace: name, customer: customer, number: number}
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

// pageRequest names up to limit messages of a conversation, before the message holding content when one is named.
type pageRequest struct {
	conversation conversationKey
	limit        int
	before       string
}

// pageStart answers the id of the message the request reads before, nil when it names none.
func (s *numbersScenario) pageStart(ctx context.Context, request pageRequest) (*uuid.UUID, error) {
	if request.before == "" {
		return nil, nil
	}
	id, err := s.messageID(ctx, request.conversation, request.before)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// readPage reads the messages the request names and keeps their content in order.
func (s *numbersScenario) readPage(ctx context.Context, request pageRequest) error {
	conversation, err := s.readConversation(ctx, request.conversation)
	if err != nil {
		return err
	}
	before, err := s.pageStart(ctx, request)
	if err != nil {
		return err
	}
	messages, err := s.plugin.WhatsAppConversationResolvers().Messages(
		s.in(ctx, request.conversation.workspace), conversation, &request.limit, before,
	)
	if err != nil {
		return err
	}
	s.read = make([]string, len(messages))
	for i, message := range messages {
		s.read[i] = message.Content
	}
	return nil
}

// readMessages reads up to limit messages of the opened conversation, before content when named.
func (s *numbersScenario) readMessages(ctx context.Context, limit int, content string) error {
	return s.readPage(ctx, pageRequest{conversation: s.opened, limit: limit, before: content})
}

// messagesRead fails unless the last read answered exactly the given messages in that order.
func (s *numbersScenario) messagesRead(want string) error {
	if got := strings.Join(s.read, ", "); got != want {
		return fmt.Errorf("the read answered %q, want %q", got, want)
	}
	return nil
}

// namesOldest fails unless the opened conversation names content as its oldest message.
func (s *numbersScenario) namesOldest(ctx context.Context, content string) error {
	key := s.opened
	conversation, err := s.readConversation(ctx, key)
	if err != nil {
		return err
	}
	want, err := s.messageID(ctx, key, content)
	if err != nil {
		return err
	}
	oldest, err := s.plugin.WhatsAppConversationResolvers().OldestMessageID(s.in(ctx, key.workspace), conversation)
	if err != nil {
		return err
	}
	if oldest == nil || *oldest != want {
		return fmt.Errorf("the conversation names %v as its oldest message, want %s", oldest, want)
	}
	return nil
}

func TestWhatsAppLongChat(t *testing.T) {
	runFeature(t, whatsAppLongChatFeature, initializeWhatsAppNumbers(t))
}
