Feature: A workspace holds many WhatsApp numbers
  A workspace can connect any number of WhatsApp numbers. Each number keeps
  its own label, access token, app secret and verify token. A customer who
  writes to a number talks to that number, and a reply leaves from it. A
  delivery is accepted for a number only when its signature matches that
  number's own app secret, so one workspace can never write into the inbox
  of another. The webhook address stays one for everybody.

  Background:
    Given the database is migrated
    And the workspace "Acme" exists
    And the workspace "Globex" exists
    And the server holds an encryption key

  Scenario: A workspace holds two numbers and each answers
    Given AlphOne runs its plugins
    And the workspace "Acme" connects the WhatsApp number "5550001" as "Sales"
    And the workspace "Acme" connects the WhatsApp number "5550002" as "Support"
    When the customer "184467235" writes to the WhatsApp number "5550001"
    And the customer "184467236" writes to the WhatsApp number "5550002"
    Then the workspace "Acme" lists the WhatsApp numbers "5550001 Sales, 5550002 Support"
    When the workspace "Acme" replies to "184467235" on "5550001"
    Then the reply leaves from "5550001" with its token
    When the workspace "Acme" replies to "184467236" on "5550002"
    Then the reply leaves from "5550002" with its token

  Scenario: Reconnecting a number the workspace holds replaces its token and keeps the rest
    Given AlphOne runs its plugins
    And the workspace "Acme" connects the WhatsApp number "5550001" as "Sales"
    And the customer "184467235" writes to the WhatsApp number "5550001"
    When the workspace "Acme" replaces the token of the WhatsApp number "5550001"
    Then the workspace "Acme" lists the WhatsApp numbers "5550001 Sales"
    And the handshake with the verify token of "5550001" passes
    When the customer "184467236" writes to the WhatsApp number "5550001"
    Then the workspace "Acme" holds 2 conversations
    When the workspace "Acme" replies to "184467235" on "5550001"
    Then the reply leaves from "5550001" with its replaced token

  Scenario: A number another workspace holds is turned down without naming that workspace
    Given AlphOne runs its plugins
    And the workspace "Acme" connects the WhatsApp number "5550001" as "Sales"
    When the workspace "Globex" tries to connect the WhatsApp number "5550001"
    Then the attempt is turned down as "channel_number_taken"
    And the attempt names no workspace
    And the workspace "Globex" lists no WhatsApp numbers
    And the workspace "Acme" lists the WhatsApp numbers "5550001 Sales"

  Scenario: A customer who writes to two numbers gets two conversations
    Given AlphOne runs its plugins
    And the workspace "Acme" connects the WhatsApp number "5550001" as "Sales"
    And the workspace "Acme" connects the WhatsApp number "5550002" as "Support"
    When the customer "184467235" writes to the WhatsApp number "5550001"
    And the customer "184467235" writes to the WhatsApp number "5550002"
    Then the workspace "Acme" holds 2 conversations
    And the workspace "Acme" holds 1 contact
    When the workspace "Acme" replies to "184467235" on "5550001"
    Then the reply leaves from "5550001" with its token
    When the workspace "Acme" replies to "184467235" on "5550002"
    Then the reply leaves from "5550002" with its token

  Scenario: A delivery signed with one number's secret writes nothing to another workspace
    Given AlphOne runs its plugins
    And the workspace "Acme" connects the WhatsApp number "5550001" as "Sales"
    And the workspace "Globex" connects the WhatsApp number "5550002" as "Front desk"
    And the customer "184467235" writes to the WhatsApp number "5550001"
    When a delivery for "5550002" arrives signed with the app secret of "5550001"
    Then the delivery is answered with 403
    And the workspace "Acme" holds 1 conversation
    And the workspace "Globex" holds 0 conversations
    And the workspace "Globex" holds 0 contacts

  Scenario: A mixed delivery stores only the items whose own number is proven
    Given AlphOne runs its plugins
    And the workspace "Acme" connects the WhatsApp number "5550001" as "Sales"
    And the workspace "Globex" connects the WhatsApp number "5550002" as "Front desk"
    When one delivery for "5550001" and "5550002" arrives signed with the app secret of "5550001"
    Then the delivery is answered with 200
    And the workspace "Acme" holds 1 conversation
    And the workspace "Globex" holds 0 conversations
    And the workspace "Globex" holds 0 contacts

  Scenario: A status for one number leaves a message on another number alone
    Given AlphOne runs its plugins
    And the workspace "Acme" connects the WhatsApp number "5550001" as "Sales"
    And the workspace "Acme" connects the WhatsApp number "5550002" as "Support"
    And the customer "184467235" writes to the WhatsApp number "5550002"
    When the workspace "Acme" replies to "184467235" on "5550002"
    And a status for the newest reply arrives for "5550001" signed with the app secret of "5550001"
    Then the delivery is answered with 200
    And the newest reply holds no delivery status
    When a status for the newest reply arrives for "5550002" signed with the app secret of "5550002"
    Then the delivery is answered with 200
    And the newest reply is marked delivered

  Scenario: The environment secret does not prove a number with its own secret
    Given the environment file names the app secret "env-app-secret"
    And AlphOne runs its plugins
    And the workspace "Acme" connects the WhatsApp number "5550001" as "Sales"
    When a delivery for "5550001" arrives signed with the environment secret
    Then the delivery is answered with 200
    And the workspace "Acme" holds 0 conversations

  Scenario: A number's own secret opens neither the environment number nor a change naming no number
    Given the environment file names the WhatsApp number "5550009" with its token
    And the environment file names the app secret "env-app-secret"
    And AlphOne runs its plugins
    And the workspace "Acme" connects the WhatsApp number "5550001" as "Sales"
    When one delivery for "5550001", the environment number "5550009" and no number arrives signed with the app secret of "5550001"
    Then the delivery is answered with 200
    And the workspace "Acme" holds 1 conversation
    And the default workspace holds 0 conversations
    And the default workspace holds 0 contacts

  Scenario: The environment number cannot be connected to another workspace
    Given the environment file names the WhatsApp number "5550009" with its token
    And the environment file names the app secret "env-app-secret"
    And AlphOne runs its plugins
    When the workspace "Acme" tries to connect the WhatsApp number "5550009"
    Then the attempt is turned down as "channel_number_taken"
    And the attempt names no workspace
    And the workspace "Acme" lists no WhatsApp numbers
    When the customer "184467235" writes to the environment number "5550009"
    Then the default workspace holds 1 conversation

  Scenario: The handshake passes with a number's verify token and fails with any other
    Given AlphOne runs its plugins
    And the workspace "Acme" connects the WhatsApp number "5550001" as "Sales"
    Then the handshake with the verify token of "5550001" passes
    And the handshake with the verify token "a guessed token" fails
    And the handshake with the verify token "" fails

  Scenario: Removing a number keeps its conversations readable and a reply says the number is gone
    Given AlphOne runs its plugins
    And the workspace "Acme" connects the WhatsApp number "5550001" as "Sales"
    And the customer "184467235" writes to the WhatsApp number "5550001"
    When the workspace "Acme" removes the WhatsApp number "5550001"
    Then the workspace "Acme" lists no WhatsApp numbers
    And the workspace "Acme" still reads the conversation with "184467235" on "5550001"
    When the workspace "Acme" tries to reply to "184467235" on "5550001"
    Then the attempt is turned down as "number_removed"
    And no reply left for WhatsApp

  Scenario: Another workspace cannot remove a number
    Given AlphOne runs its plugins
    And the workspace "Acme" connects the WhatsApp number "5550001" as "Sales"
    When the workspace "Globex" removes the WhatsApp number "5550001"
    Then the workspace "Acme" lists the WhatsApp numbers "5550001 Sales"

  Scenario: The environment number keeps one conversation across the upgrade
    Given the environment file names the WhatsApp number "5550009" with its token
    And the environment file names the app secret "env-app-secret"
    And an install from before many numbers holds a conversation with "184467235"
    And AlphOne runs its plugins
    When the customer "184467235" writes to the environment number "5550009"
    Then the default workspace holds 1 conversation
    When the default workspace replies to "184467235"
    Then the reply leaves from "5550009" with the environment token

  Scenario: An upgrade beside another stored number reads older conversations as that number
    Given the environment file names the WhatsApp number "5550009" with its token
    And the environment file names the app secret "env-app-secret"
    And an install from before many numbers holds a conversation with "184467235"
    And that install stored the WhatsApp number "5550001" for the default workspace
    And AlphOne runs its plugins
    And the default workspace connects the WhatsApp number "5550001" as "Sales"
    When the default workspace replies to "184467235" on "5550001"
    Then the reply leaves from "5550001" with its token
    When the customer "184467235" writes to the environment number "5550009"
    Then the default workspace holds 2 conversations
    When the default workspace replies to "184467235"
    Then the reply leaves from "5550009" with the environment token

  Scenario: A reply on an environment number with no token leaves from no other number
    Given the environment file names the WhatsApp number "5550009" without a token
    And the environment file names the app secret "env-app-secret"
    And AlphOne runs its plugins
    And the default workspace connects the WhatsApp number "5550001" as "Sales"
    And the customer "184467235" writes to the environment number "5550009"
    When the default workspace tries to reply to "184467235"
    Then the attempt is turned down as "credentials_missing"
    And no reply left for WhatsApp

  Scenario: The environment number reads the conversations that store no number
    Given the environment file names the WhatsApp number "5550009" with its token
    And the environment file names the app secret "env-app-secret"
    And AlphOne runs its plugins
    When the customer "184467235" writes to the environment number "5550009"
    Then the default workspace reads 1 conversation on "5550009"

  Scenario: The default workspace lists the environment number beside a connected number
    Given the environment file names the WhatsApp number "5550009" with its token
    And the environment file names the app secret "env-app-secret"
    And AlphOne runs its plugins
    And the default workspace connects the WhatsApp number "5550001" as "Sales"
    When the customer "184467235" writes to the environment number "5550009"
    Then the default workspace lists the WhatsApp numbers "5550009, 5550001 Sales"
    And the default workspace reads the conversation with "184467235" as on "5550009"
    And the workspace "Acme" lists no WhatsApp numbers

  Scenario: A reply on the environment number leaves from it while the default workspace holds another number
    Given the environment file names the WhatsApp number "5550009" with its token
    And the environment file names the app secret "env-app-secret"
    And AlphOne runs its plugins
    And the customer "184467235" writes to the environment number "5550009"
    And the default workspace connects the WhatsApp number "5550001" as "Sales"
    When the default workspace replies to "184467235"
    Then the reply leaves from "5550009" with the environment token

  Scenario: A reply on the environment number still leaves from it after the default workspace connected that number and removed it
    Given the environment file names the WhatsApp number "5550009" with its token
    And the environment file names the app secret "env-app-secret"
    And AlphOne runs its plugins
    And the customer "184467235" writes to the environment number "5550009"
    And the default workspace connects the WhatsApp number "5550009" as "Sales"
    And the customer "184467235" writes to the WhatsApp number "5550009"
    Then the default workspace holds 2 conversations
    When the default workspace replies to "184467235"
    Then the reply leaves from "5550009" with its token
    When the default workspace removes the WhatsApp number "5550009"
    And the default workspace replies to "184467235"
    Then the reply leaves from "5550009" with the environment token
    When the default workspace replies to "184467235" on "5550009"
    Then the reply leaves from "5550009" with the environment token
