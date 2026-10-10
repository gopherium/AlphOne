Feature: A list pages through the contact directory
  The contact page answers one page of contacts and the total it pages
  through. It searches, filters by channel and sorts by name or by
  creation time.

  Background:
    Given a running AlphOne holding a user with an API token

  Scenario: A page answers its contacts in name order and the total
    Given these contacts, created in this order:
      | name         |
      | Maria Perez  |
      | Ada Lovelace |
      | Grace Hopper |
    When the caller reads the contact page 2 at a time from offset 0
    Then the page lists:
      | name         |
      | Ada Lovelace |
      | Grace Hopper |
    And the page counts 3 contacts
    And the page holds at most 2 contacts

  Scenario: The next offset answers the rest
    Given these contacts, created in this order:
      | name         |
      | Maria Perez  |
      | Ada Lovelace |
      | Grace Hopper |
    When the caller reads the contact page 2 at a time from offset 2
    Then the page lists:
      | name        |
      | Maria Perez |
    And the page counts 3 contacts

  Scenario: A page past the end still counts the contacts
    Given these contacts, created in this order:
      | name         |
      | Maria Perez  |
      | Ada Lovelace |
    When the caller reads the contact page 2 at a time from offset 10
    Then the page lists no contacts
    And the page counts 2 contacts

  Scenario: A search narrows the page and its total
    Given a contact "Maria Perez" reachable on whatsapp as "184467235"
    And a contact "Ada Lovelace" reachable on email as "ada@example.com"
    When the caller searches the contact page for "184 467"
    Then the page lists:
      | name        |
      | Maria Perez |
    And the page counts 1 contact

  Scenario: A percent sign in a search matches only itself
    Given these contacts, created in this order:
      | name          |
      | Promo 50% off |
      | Promo 500 off |
    When the caller searches the contact page for "50%"
    Then the page lists:
      | name          |
      | Promo 50% off |
    And the page counts 1 contact

  Scenario: An underscore in a search matches only itself
    Given these contacts, created in this order:
      | name      |
      | team_lead |
      | team lead |
    When the caller searches the contact page for "m_l"
    Then the page lists:
      | name      |
      | team_lead |
    And the page counts 1 contact

  Scenario: A channel filter keeps the contacts reachable on it
    Given a contact "Maria Perez" reachable on whatsapp as "184467235"
    And a contact "Ada Lovelace" reachable on email as "ada@example.com"
    When the caller filters the contact page to the whatsapp channel
    Then the page lists:
      | name        |
      | Maria Perez |
    And the page counts 1 contact

  Scenario: The newest contacts come first when sorted by creation time descending
    Given these contacts, created in this order:
      | name         |
      | Maria Perez  |
      | Ada Lovelace |
      | Grace Hopper |
    When the caller sorts the contact page by CREATED_AT DESC
    Then the page lists:
      | name         |
      | Grace Hopper |
      | Ada Lovelace |
      | Maria Perez  |

  Scenario: A name sort descending reverses the directory
    Given these contacts, created in this order:
      | name         |
      | Ada Lovelace |
      | Maria Perez  |
      | Grace Hopper |
    When the caller sorts the contact page by NAME DESC
    Then the page lists:
      | name         |
      | Maria Perez  |
      | Grace Hopper |
      | Ada Lovelace |

  Scenario: A page asked past the cap is refused naming the cap
    Given a contact "Maria Perez" reachable on whatsapp as "184467235"
    When the caller reads the contact page 201 at a time from offset 0
    Then the read is refused as out of range up to 200

  Scenario: Each listed contact carries the channels it is reachable on
    Given a contact "Maria Perez" reachable on whatsapp as "184467235"
    When the caller reads the contact page
    Then "Maria Perez" is listed reachable on whatsapp

  Scenario: Another tenant's contacts stay out of the page
    Given a contact "Maria Perez" reachable on whatsapp as "184467235"
    And the tenant "Acme" exists
    And a user "grace.hopper@example.com" holding a token is placed in the tenant "Acme"
    When that token reads the contact page
    Then the page lists no contacts
    And the page counts 0 contacts
