Feature: A plugin keeps the settings of each workspace apart
  A plugin that needs the settings each workspace chooses, such as its
  mailbox address and password, keeps them through the settings store
  AlphOne hands it. A setting belongs to the workspace that saved it and
  to the plugin that saved it. A secret setting is encrypted with the key
  the server is given, and it opens only in the workspace and under the
  setting it was saved for.

  Background:
    Given the database is migrated
    And the server holds an encryption key
    And the workspace "Acme" exists
    And the workspace "Globex" exists
    And AlphOne runs a mail plugin and a calendar plugin that keep settings

  Scenario: Two workspaces keep a setting apart
    When the workspace "Acme" saves the mailbox host "imap.example.com"
    And the workspace "Globex" saves the mailbox host "mail.example.com"
    Then the workspace "Acme" reads the mailbox host "imap.example.com"
    And the workspace "Globex" reads the mailbox host "mail.example.com"

  Scenario: A secret copied to another workspace does not open
    Given the workspace "Acme" saves the mailbox password "correct horse battery"
    When the mailbox password of "Acme" is copied to "Globex"
    Then the workspace "Globex" cannot open its mailbox password
    And the workspace "Acme" still opens its mailbox password as "correct horse battery"

  Scenario: A plugin never reaches the settings of another plugin
    Given the workspace "Acme" saves the mailbox password "correct horse battery"
    When the calendar plugin asks its store for the store of the mail plugin
    Then the calendar plugin gets no store of the mail plugin
    And the calendar plugin reads no mailbox password in "Acme"
