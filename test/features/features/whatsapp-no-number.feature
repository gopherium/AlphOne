Feature: A WhatsApp sender who shares no phone number
  Meta can leave out the phone number of a person who uses a WhatsApp
  username, and send a user ID such as "US.184467235" in its place. The
  person still reaches the inbox, and a reply still reaches the person. A
  user ID always holds a period, so a value without one is skipped and never
  joins the conversation of a phone number.

  Background:
    Given the database is migrated
    And the workspace "Acme" exists
    And the workspace "Globex" exists
    And the server holds an encryption key
    And AlphOne runs its plugins
    And the workspace "Acme" connects the WhatsApp number "5550001" as "Sales"

  Scenario: A sender with no phone number reaches the inbox under their profile name
    When the person "US.184467235" named "Maria Perez" writes with no phone number to "5550001"
    Then the workspace "Acme" holds 1 conversation
    And the workspace "Acme" holds 1 contact
    And the workspace "Acme" holds a contact named "Maria Perez" on "US.184467235"

  Scenario: Two messages from one user ID stay in one conversation
    When the person "US.184467235" named "Maria Perez" writes with no phone number to "5550001"
    And the person "US.184467235" named "Maria Perez" writes with no phone number to "5550001"
    Then the workspace "Acme" holds 1 conversation
    And the workspace "Acme" holds 1 contact
    And the workspace "Acme" holds 2 messages in the conversation with "US.184467235" on "5550001"

  Scenario: A sender with no name at all is named after the user ID
    When the person "US.184467235" with no name writes with no phone number to "5550001"
    Then the workspace "Acme" holds a contact named "US.184467235" on "US.184467235"

  Scenario: A reply to a user ID leaves through the recipient field
    Given the person "US.184467235" named "Maria Perez" writes with no phone number to "5550001"
    When the workspace "Acme" replies to "US.184467235" on "5550001"
    Then the reply names "US.184467235" as its recipient and has no to field
    And the reply leaves from "5550001" with its token

  Scenario: A status moves a reply sent to a user ID
    Given the person "US.184467235" named "Maria Perez" writes with no phone number to "5550001"
    When the workspace "Acme" replies to "US.184467235" on "5550001"
    And a status for the newest reply to the user ID "US.184467235" arrives for "5550001"
    Then the newest reply is marked delivered

  Scenario: A message carrying both a phone number and a user ID stays under the phone number
    When the person "US.184467235" with the phone number "184467235" writes to "5550001"
    Then the workspace "Acme" holds a contact named "Maria Perez" on "184467235"
    When the workspace "Acme" replies to "184467235" on "5550001"
    Then the reply names "184467235" in its to field and has no recipient

  Scenario: A person known by phone who later writes with only a user ID gets a second contact
    Given the person "US.184467235" with the phone number "184467235" writes to "5550001"
    When the person "US.184467235" named "Maria Perez" writes with no phone number to "5550001"
    Then the workspace "Acme" holds 2 contacts
    And the workspace "Acme" holds 2 conversations

  Scenario: A person known by phone on one number who writes to another with only a user ID gets a second contact
    Given the workspace "Acme" connects the WhatsApp number "5550002" as "Support"
    And the person "US.184467235" with the phone number "184467235" writes to "5550001"
    When the person "US.184467235" named "Maria Perez" writes with no phone number to "5550002"
    Then the workspace "Acme" holds 2 contacts
    And the workspace "Acme" holds 2 conversations

  Scenario: A person who moves between a user ID and a phone number keeps one conversation for each
    Given the person "US.184467235" named "Maria Perez" writes with no phone number to "5550001"
    And the person "US.184467235" with the phone number "184467235" writes to "5550001"
    When the person "US.184467235" named "Maria Perez" writes with no phone number to "5550001"
    Then the workspace "Acme" holds 2 contacts
    And the workspace "Acme" holds 2 conversations
    And the workspace "Acme" holds 2 messages in the conversation with "US.184467235" on "5550001"
    When the workspace "Acme" replies to "US.184467235" on "5550001"
    Then the reply names "US.184467235" as its recipient and has no to field

  Scenario: One user ID reaching two workspaces stays apart
    Given the workspace "Globex" connects the WhatsApp number "5550002" as "Front desk"
    When the person "US.184467235" named "Maria Perez" writes with no phone number to "5550001"
    And the person "US.184467235" named "Maria Perez" writes with no phone number to "5550002"
    Then the workspace "Acme" holds 1 contact
    And the workspace "Acme" holds 1 conversation
    And the workspace "Globex" holds 1 contact
    And the workspace "Globex" holds 1 conversation

  Scenario: A message with no sender stores nothing and the delivery still succeeds
    When a message with no sender arrives for "5550001"
    Then the delivery is answered with 200
    And the workspace "Acme" holds 0 conversations

  Scenario: A user ID with no period never joins a phone number's conversation
    Given the customer "184467235" writes to the WhatsApp number "5550001"
    When the person "184467235" named "Maria Perez" writes with no phone number to "5550001"
    Then the delivery is answered with 200
    And the workspace "Acme" holds 1 conversation
    And the workspace "Acme" holds 1 message in the conversation with "184467235" on "5550001"
