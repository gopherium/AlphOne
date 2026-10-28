Feature: WhatsApp keeps each workspace's token inside that workspace
  A workspace can send WhatsApp messages from a number of its own. The
  WhatsApp plugin stores that number with its access token, and encrypts
  the token with the encryption key the server is given, tied to the
  workspace that stored it. A token copied into another workspace does
  not open there. The number in the environment file still serves the
  default workspace, with or without a key.

  Background:
    Given the database is migrated
    And the workspace "Acme" exists
    And the workspace "Globex" exists

  Scenario: A workspace's token opens in that workspace
    Given the server holds an encryption key
    And AlphOne runs its plugins
    When the workspace "Acme" stores the WhatsApp number "5550001" with its token
    Then the workspace "Acme" answers the WhatsApp number "5550001"
    And the token stored for "Acme" is not readable at rest

  Scenario: A token copied into another workspace does not open there
    Given the server holds an encryption key
    And AlphOne runs its plugins
    And the workspace "Acme" stores the WhatsApp number "5550001" with its token
    When the token of "Acme" is copied to "Globex" under another number
    Then the workspace "Globex" cannot open its WhatsApp token
    And the workspace "Acme" answers the WhatsApp number "5550001"

  Scenario: Without an encryption key a workspace's token is not stored
    Given AlphOne runs its plugins
    When the workspace "Acme" tries to store the WhatsApp number "5550001" with its token
    Then the attempt answers an error that names "ALPHONE_ENCRYPTION_KEY"
    And no WhatsApp token is stored

  Scenario: Without an encryption key the default workspace keeps the number from the environment file
    Given the environment file names the WhatsApp number "5550009" with its token
    And AlphOne runs its plugins
    Then the default workspace answers the WhatsApp number "5550009"
