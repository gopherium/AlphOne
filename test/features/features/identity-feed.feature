Feature: A plugin follows the addresses its workspace gains
  A plugin reads the addresses added to contacts after a cursor it keeps.
  Each workspace reads only the addresses of its own contacts.

  Background:
    Given a running AlphOne holding a user with an API token
    And the tenant "Acme" exists
    And a user "acme.member@example.com" holding a token is placed in the tenant "Acme"

  Scenario: An address added to a contact appears in the feed of its own workspace only
    Given the tenant "Acme" holds the contact "Maria Perez"
    And a plugin reads the email feed of each tenant from its horizon
    When the tenant "Acme" adds the email address "Maria.Perez@Example.com" to that contact
    Then the email feed of the tenant "Acme" answers "maria.perez@example.com" for that contact
    And the email feed of the default tenant answers no address
