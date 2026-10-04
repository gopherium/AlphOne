Feature: Operators run AlphOne from one command line
  An operator looks after an install through the alphone command. A run
  with no command lists what the program offers. Seeding, a change to an
  existing account and a token revoke are only previews until the
  operator confirms them. A change to an existing account names the
  administrator who makes it and is kept on record. Tokens are minted,
  listed and revoked for the account that owns them.

  Background:
    Given the settings point at an empty database

  Scenario: A run with no command lists every command and leaves the database without a schema
    When the operator runs alphone with no command
    Then the command succeeds
    And the answer lists the commands "serve", "migrate", "seed" and "check"
    And the database holds no schema

  Scenario Outline: An account command name from before the command line is refused
    When the operator runs "<name>"
    Then the command exits with code 2
    And the answer says "<name>" is an unknown command
    And the answer names the command "alphone list"

    Examples:
      | name        |
      | createadmin |
      | grantrole   |

  Scenario: A help page answers without a database
    Given the settings name no database
    When the operator asks for the help page of "migrate"
    Then the command succeeds
    And the answer describes "migrate"

  Scenario: Seeding only previews until it is confirmed with -yes
    When the operator runs "seed"
    Then the command succeeds
    And the answer says nothing changed until it is confirmed with "-yes"
    And the database holds no demo data
    When the operator runs "seed -yes"
    Then the command succeeds
    And the database holds the demo data

  Scenario: The first administrator is created from the command line
    When the operator creates the administrator "admin@example.com" with the password "correct horse battery"
    Then the command succeeds
    And the account "admin@example.com" holds the role "admin"

  Scenario Outline: An account change is refused when <case>
    Given the administrator "admin@example.com"
    And the member "maria.perez@example.com"
    When the operator gives "maria.perez@example.com" the role "admin" acting as "<actor>"
    Then the command exits with code <code>
    And the error says "<error>"
    And the account "maria.perez@example.com" still holds the role "member"
    And no account change is on record

    Examples:
      | case                                 | actor                   | code | error                    |
      | it names no acting account           |                         | 2    | wants -as                |
      | the acting account is a member       | maria.perez@example.com | 1    | which lacks manage_users |
      | nobody answers to the acting address | nobody@example.com      | 1    | no account answers to    |

  Scenario: An applied account change is kept on record
    Given the administrator "admin@example.com"
    And the member "maria.perez@example.com"
    When the operator gives "maria.perez@example.com" the role "admin" acting as "admin@example.com"
    Then the command succeeds
    And the account "maria.perez@example.com" holds the role "admin"
    And the command "account:records" lists "account:role" applied by "admin@example.com"

  Scenario: A preview of an account change records nothing
    Given the administrator "admin@example.com"
    And the member "maria.perez@example.com"
    When the operator previews giving "maria.perez@example.com" the role "admin" acting as "admin@example.com"
    Then the command succeeds
    And the answer says nothing changed until it is confirmed with "-yes"
    And the account "maria.perez@example.com" still holds the role "member"
    And no account change is on record

  Scenario: A token is minted and its secret is shown once
    Given the administrator "admin@example.com"
    When the operator mints a token named "automation" for "admin@example.com"
    Then the command succeeds
    And the answer carries a secret beginning with "a1_"
    And the token list of "admin@example.com" shows "automation"
    And the token list of "admin@example.com" shows no secret

  Scenario: The old two word token spelling still works and says it is deprecated
    Given the administrator "admin@example.com"
    When the operator mints a token named "automation" for "admin@example.com" with the old spelling "token create"
    Then the command succeeds
    And the answer carries a secret beginning with "a1_"
    And the answer says "token create" is deprecated and names "token:create"
    When the operator lists the tokens of "admin@example.com" with the old spelling "token list"
    Then the command succeeds
    And the answer lists the token "automation"
    And the answer says "token list" is deprecated and names "token:list"

  Scenario: The old revoke spelling is refused
    Given the administrator "admin@example.com"
    And the account "admin@example.com" holds a token named "automation"
    When the operator revokes the token "automation" of "admin@example.com" with the old spelling "token revoke"
    Then the command exits with code 2
    And the answer names the command "token:revoke"
    And the token list of "admin@example.com" shows "automation"

  Scenario: Revoking a token only previews until it is confirmed with -yes
    Given the administrator "admin@example.com"
    And the account "admin@example.com" holds a token named "automation"
    When the operator previews revoking the token "automation" of "admin@example.com"
    Then the command succeeds
    And the answer says nothing changed until it is confirmed with "-yes"
    And the token list of "admin@example.com" shows "automation"
    When the operator revokes the token "automation" of "admin@example.com"
    Then the command succeeds
    And the token list of "admin@example.com" shows no token

  Scenario: The check command names a malformed setting
    Given the setting "ALPHONE_INVITE_TTL" holds "a week"
    When the operator runs "check"
    Then the command exits with code 1
    And the answer names the setting "ALPHONE_INVITE_TTL"

  Scenario: A token belongs to its owner's workspace
    Given the member "maria.perez@example.com"
    And the workspace "Acme" exists
    And the account "maria.perez@example.com" is placed in the workspace "Acme"
    When the operator mints a token named "automation" for "maria.perez@example.com"
    Then the command succeeds
    And the token "automation" is kept in the workspace "Acme"

  Scenario: The token list answers as JSON
    Given the administrator "admin@example.com"
    And the account "admin@example.com" holds a token named "automation"
    When the operator lists the tokens of "admin@example.com" as JSON
    Then the command succeeds
    And the answer is one JSON document listing the token "automation"
    And the JSON document holds no secret

  Scenario: The token list covers every account with -all
    Given the administrator "admin@example.com"
    And the member "maria.perez@example.com"
    And the workspace "Acme" exists
    And the account "maria.perez@example.com" is placed in the workspace "Acme"
    And the account "admin@example.com" holds a token named "automation"
    And the account "maria.perez@example.com" holds a token named "reporting"
    When the operator lists the tokens of every account with "-all"
    Then the command succeeds
    And the answer lists the token "automation" of "admin@example.com" in the workspace "Default"
    And the answer lists the token "reporting" of "maria.perez@example.com" in the workspace "Acme"

  Scenario: An acting account cannot disable itself
    Given the administrator "admin@example.com"
    And the administrator "maria.perez@example.com"
    When the operator disables "admin@example.com" acting as "admin@example.com"
    Then the command exits with code 1
    And the account "admin@example.com" is still enabled
    And no account change is on record

  Scenario: An account change beyond the acting account's reach is refused
    Given a plugin declares the role "steward" with a capability the role "admin" lacks
    And the administrator "admin@example.com"
    And the member "maria.perez@example.com"
    When the operator gives "maria.perez@example.com" the role "steward" acting as "admin@example.com"
    Then the command exits with code 1
    And the account "maria.perez@example.com" still holds the role "member"
    And no account change is on record
