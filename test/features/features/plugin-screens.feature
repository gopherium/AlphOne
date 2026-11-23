Feature: A plugin's screens hide until its workspace holds what they show
  A plugin can say whether its menu rows and screens show in the caller's
  workspace. The admin reads one list of the plugins that hide their
  screens and leaves their rows and panels out. The email screens show
  only once the workspace holds a mailbox. A plugin that fails, panics or
  answers after its time ran out keeps its screens shown, so a broken
  plugin never takes a screen away. Two settings bound how long the
  plugins together may take to answer and how many later calls time each
  plugin after one ran late, and each must stand above zero.

  Background:
    Given the database is migrated
    And the setting "ALPHONE_PLUGIN_SCREENS_BUDGET" holds "5s"
    And the administrator "admin@example.com"
    And the member "member@example.com"
    And the workspace "Acme" exists
    And the account "member@example.com" is placed in the workspace "Acme"
    And the member "member@example.com" holds a token scoped to "meta:read"

  Scenario: A workspace with no mailbox hides the email screens
    When the member "member@example.com" reads which plugins hide their screens
    Then the plugins that hide their screens are "email"
    And the server log holds no "plugin hook failed" line

  Scenario: A workspace that holds a mailbox shows the email screens
    Given the workspace "Acme" knows "Maria Perez" at "maria.perez@example.com"
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Opening hours
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <opening-hours@example.com>
      Content-Type: text/plain

      When does the shop open on Saturday?
      """
    When the member "member@example.com" reads which plugins hide their screens
    Then no plugin hides its screens
    And the server log holds no "plugin hook failed" line

  Scenario: An answer after the budget ran out keeps the email screens shown and is logged
    Given the setting "ALPHONE_PLUGIN_SCREENS_BUDGET" holds "1ns"
    When the member "member@example.com" reads which plugins hide their screens
    Then no plugin hides its screens
    And the server log holds one line with "plugin=email hook=admin.screens panicked=false context_ended=true"

  Scenario: A caller who is not signed in is rejected
    When a caller who is not signed in reads which plugins hide their screens
    Then the read is rejected as unauthenticated

  Scenario Outline: A hook setting that is not above zero names its key at start
    Given the setting "<key>" holds "<value>"
    When the operator starts AlphOne
    Then AlphOne stops before it serves, naming the setting "<key>"

    Examples:
      | key                               | value |
      | ALPHONE_PLUGIN_SCREENS_BUDGET     | 0s    |
      | ALPHONE_PLUGIN_HOOK_TIMED_FIRINGS | 0     |
