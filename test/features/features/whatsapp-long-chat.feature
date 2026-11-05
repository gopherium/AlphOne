Feature: A long WhatsApp chat opens on its newest messages
  A chat longer than a page opens on its newest messages, listed oldest
  first. The older messages stay reachable one page at a time, and the
  conversation names its oldest message, so a reader knows when the whole
  chat is loaded.

  Background:
    Given the database is migrated
    And the workspace "Acme" exists
    And the server holds an encryption key
    And AlphOne runs its plugins
    And the workspace "Acme" connects the WhatsApp number "5550001" as "Sales"
    And the customer "184467235" wrote 5 messages to the WhatsApp number "5550001"

  Scenario: A chat shorter than the page reads every message, oldest first
    When the workspace "Acme" reads 10 messages of the conversation with "184467235" on "5550001"
    Then the messages read are "message 1, message 2, message 3, message 4, message 5"

  @wip
  Scenario: Opening a long chat reads its newest messages, oldest first
    When the workspace "Acme" reads 2 messages of the conversation with "184467235" on "5550001"
    Then the messages read are "message 4, message 5"

  @wip
  Scenario: Reading back from a message answers the messages just before it
    When the workspace "Acme" reads 2 messages before "message 4" of the conversation with "184467235" on "5550001"
    Then the messages read are "message 2, message 3"

  @wip
  Scenario: Reading back far enough reaches the first message of the chat
    When the workspace "Acme" reads 2 messages before "message 2" of the conversation with "184467235" on "5550001"
    Then the messages read are "message 1"
    And the workspace "Acme" names "message 1" as the oldest message of the conversation with "184467235" on "5550001"
