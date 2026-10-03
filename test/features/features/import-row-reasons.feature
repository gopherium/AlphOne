Feature: Every staged row says why it settled as it did
  A staged row that is skipped, fails or needed a repair carries its
  reason as a stable code beside the values the reason names, so every
  screen can say it in the reader's own language.

  Background:
    Given a running AlphOne holding a user with an API token
    And the field "birthDate" labelled "Birth date" of kind DATE is defined

  Scenario: A row whose address another contact holds names that contact
    Given a contact named "Maria Perez" reachable at "maria@example.com"
    And an uploaded spreadsheet holding the row "M. Perez,maria@example.com,1990-04-17"
    And the columns are mapped onto name, email and the field "birthDate"
    When the import is committed
    Then the commit answers 1 skipped
    And a row settles skipped with the reason "identity_taken_by" and the meta:
      """
      {"ownerName": "Maria Perez"}
      """

  Scenario: A row shorter than the header imports and keeps its note
    Given an uploaded spreadsheet holding the row "Maria Perez,maria@example.com"
    And the columns are mapped onto name, email and the field "birthDate"
    When the import is committed
    Then the commit answers 1 imported
    And a row settles imported with the reason "row_cell_count_mismatch" and the meta:
      """
      {"cells": 2, "columns": 3}
      """

  Scenario: A row with a name and no address fails as incomplete
    Given an uploaded spreadsheet holding the row "Maria Perez,,"
    And the columns are mapped onto name, email and the field "birthDate"
    When the import is committed
    Then the commit answers 1 failed
    And a row settles failed with the reason "row_incomplete" and the meta:
      """
      {}
      """
