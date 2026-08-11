Feature: An agent reads contacts
  The find_contacts tool searches the directory and marks who holds open
  work, and get_contact reads one contact in full.

  Background:
    Given a running AlphOne holding a user with an API token
    And the agent is connected with the token

  Scenario: A search marks which matches hold open work
    Given a contact "Maria Perez" holding an open task
    And a contact "Ada Lovelace" holding no tasks
    When the agent calls find_contacts with query "a"
    Then the answer lists "Maria Perez" marked holding open work
    And the answer lists "Ada Lovelace" marked free

  Scenario: Phone digits find the contact
    Given a contact "Maria Perez" reachable on whatsapp as "184467235"
    When the agent calls find_contacts with query "184 467"
    Then the answer lists "Maria Perez"

  Scenario: One contact answers in full
    Given a contact "Maria Perez" reachable on whatsapp as "184467235"
    And an open task "Call Maria Perez back" due today linked to that contact
    When the agent calls get_contact for that contact
    Then the answer names "Maria Perez"
    And the answer carries the whatsapp identity "184467235"
    And the answer lists the open task "Call Maria Perez back"

  Scenario: An unknown contact id is refused as not found
    When the agent calls get_contact with an id no contact holds
    Then the tool fails with a not found error
