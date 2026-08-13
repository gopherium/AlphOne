Feature: A spreadsheet fills the fields
  A CSV column maps onto a runtime field the same way it maps onto name,
  email and phone. Values are checked before a contact is created, an
  import only ever creates, and a mapping naming a vanished field refuses
  the commit while the import is still editable.

  Background:
    Given a running AlphOne holding a user with an API token

  Scenario: A live field joins the mapping registry beside the core columns
    When the operator defines the field "birthDate" labelled "Birth date" of kind DATE
    Then the mapping registry lists "birthDate" labelled "Birth date" beside the core columns

  Scenario: A committed import writes a mapped cell into its field
    Given the field "birthDate" labelled "Birth date" of kind DATE is defined
    And an uploaded spreadsheet holding the row "Maria Perez,maria@example.com,1990-04-17"
    And the columns are mapped onto name, email and the field "birthDate"
    When the import is committed
    Then the commit answers 1 imported
    And the contact "Maria Perez" answers "1990-04-17" for the field "birthDate"

  Scenario: An empty cell writes nothing
    Given the field "birthDate" labelled "Birth date" of kind DATE is defined
    And an uploaded spreadsheet holding the row "Maria Perez,maria@example.com,"
    And the columns are mapped onto name, email and the field "birthDate"
    When the import is committed
    Then the commit answers 1 imported
    And the contact "Maria Perez" answers null for the field "birthDate"

  Scenario: A cell that does not fit its kind fails the row and creates no contact
    Given the field "birthDate" labelled "Birth date" of kind DATE is defined
    And an uploaded spreadsheet holding the row "Maria Perez,maria@example.com,not a date"
    And the columns are mapped onto name, email and the field "birthDate"
    When the import is committed
    Then the commit answers 1 failed
    And a row settles failed naming "birthDate" and kind DATE
    And no contact named "Maria Perez" exists

  Scenario: Archiving a mapped field refuses the commit and keeps the import ready
    Given the field "birthDate" labelled "Birth date" of kind DATE is defined
    And an uploaded spreadsheet holding the row "Maria Perez,maria@example.com,1990-04-17"
    And the columns are mapped onto name, email and the field "birthDate"
    When the operator archives the field "birthDate"
    And the import is committed
    Then the commit is refused naming "birthDate"
    And the import stays ready for a new mapping

  Scenario: A field named after a core column stays out of the registry
    When the operator defines the field "email" labelled "Second email" of kind TEXT
    Then the mapping registry lists "email" exactly once

  Scenario: A skipped row leaves the existing contact's fields untouched
    Given the field "birthDate" labelled "Birth date" of kind DATE is defined
    And a contact named "Maria Perez" reachable at "maria@example.com"
    And an uploaded spreadsheet holding the row "M. Perez,maria@example.com,1990-04-17"
    And the columns are mapped onto name, email and the field "birthDate"
    When the import is committed
    Then the commit answers 1 skipped
    And the contact "Maria Perez" answers null for the field "birthDate"
