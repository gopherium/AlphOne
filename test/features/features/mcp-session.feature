Feature: An agent opens an MCP session
  AlphOne answers agents over MCP at /api/mcp. A session carries the same
  API token every integration uses, and every tool it advertises is read only.

  Background:
    Given a running AlphOne holding a user with an API token

  Scenario: The tools are advertised to a connected agent
    When the agent connects with the token
    Then the session advertises exactly these tools
      | workload_summary |
      | list_my_tasks    |
      | find_contacts    |
      | get_contact      |
    And every advertised tool is marked read only

  Scenario: A connection without a credential is refused
    When the agent connects without a token
    Then the connection is refused as unauthorized

  Scenario: A connection with a revoked token is refused
    Given the token is revoked
    When the agent connects with the token
    Then the connection is refused as unauthorized
