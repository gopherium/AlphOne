Feature: Every caller stands in a tenant
  A fresh install holds one default tenant and every user belongs to it
  without a row saying so. Membership rows exist only for users placed
  somewhere else, and the graph answers each caller its own tenant.

  Background:
    Given a running AlphOne holding a user with an API token

  @wip
  Scenario: A fresh install holds the default tenant
    Then the store holds the tenant "Default"

  @wip
  Scenario: A caller without a membership row is answered the default tenant
    When the caller asks for its tenant
    Then the tenant answered is "Default"

  @wip
  Scenario: A member of another tenant is answered that tenant
    Given the tenant "Acme" exists
    And the caller is placed in the tenant "Acme"
    When the caller asks for its tenant
    Then the tenant answered is "Acme"

  @wip
  Scenario: An API token caller is answered its own user's tenant
    Given the tenant "Globex" exists
    And a user "grace.hopper@example.com" holding a token is placed in the tenant "Globex"
    When that token asks for its tenant
    Then the tenant answered is "Globex"

  @wip
  Scenario: An anonymous caller is refused before the field
    When an anonymous caller asks for a tenant
    Then the ask is refused as unauthenticated
