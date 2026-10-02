Feature: The admin screens read their settings
  One query answers the settings the admin screens read once: how long
  a confirmation toast stays, the page sizes a list offers and the one
  it opens on, the most contacts one contact page holds, and the locale
  dates, times, numbers and money are written in.

  Background:
    Given a running AlphOne holding a user with an API token

  Scenario: A fresh install answers the defaults
    When the caller reads the admin settings
    Then a toast stays 6000 milliseconds
    And a list offers the page sizes 10, 20, 50 and 100
    And a list opens on 20 rows
    And a contact page holds at most 200 contacts
    And dates, times, numbers and money are written in es-ES

  Scenario: An anonymous caller is refused the settings
    When an anonymous caller reads the admin settings
    Then the ask is refused as unauthenticated
