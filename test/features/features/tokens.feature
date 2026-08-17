Feature: API tokens are scoped and mortal
  A token carries a set of scopes narrowing its owner's authority and
  an expiry ending it. The gate checks every operation's root fields
  against the caller's token, sessions pass every check, and token
  management itself always needs a session.

  Background:
    Given a running AlphOne holding a user with an API token

  Scenario: A read token reads but cannot write
    Given the user holds a token scoped to "contacts:read tasks:read"
    When that token lists the contacts
    Then the list is answered
    When that token creates a contact named "Maria Perez"
    Then the operation is refused as unauthorized naming "contacts:write"

  Scenario: A token stays inside its granted areas
    Given the user holds a token scoped to "tasks:write"
    When that token creates a task titled "Call the supplier"
    Then the task is answered
    When that token disables a user
    Then the operation is refused as unauthorized naming "users:write"

  Scenario: A token cannot manage tokens whatever its scope
    Given the user holds a token scoped to "*"
    When that token lists the api tokens
    Then the operation is refused as unauthorized for token management

  Scenario: An expired token is refused
    Given the user holds a token that expired yesterday
    When that token asks for the version
    Then the request is refused as an invalid token

  Scenario: Revoking over the graph stops the token immediately
    Given the user holds a token scoped to "contacts:read"
    When the user's session revokes that token
    And that token lists the contacts
    Then the request is refused as an invalid token

  Scenario: Minting answers the secret once and the list shows scope and expiry
    When the user's session mints a token named "automation" scoped to "tasks:write" lasting 30 days
    Then the answer carries a secret beginning with "a1_"
    And the session's token list shows "automation" scoped to "tasks:write" expiring in 30 days
    And the list never shows a secret

  Scenario: A legacy token holds full scope and no expiry
    Given a token minted before scopes existed
    When that token creates a contact named "Maria Perez"
    Then the contact is answered
    And the session's token list shows it scoped to "*" never expiring

  @wip
  Scenario: An MCP tool call is refused by a withheld read scope
    Given the user holds a token scoped to "contacts:read"
    And an MCP session connected with that token
    When the agent calls the tool listing today's tasks
    Then the tool answers the refusal naming "tasks:read"
