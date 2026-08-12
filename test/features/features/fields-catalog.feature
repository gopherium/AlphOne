Feature: An operator shapes the field catalogue
  Contact fields are defined at runtime by an operator. A definition carries
  a machine name, a human label and a kind. The catalogue refuses names the
  compiled schema already owns, so a runtime field can never shadow a real
  one.

  Background:
    Given a running AlphOne holding a user with an API token

  @wip
  Scenario: Defining a field lists it in the catalogue
    When the operator defines the field "birthDate" labelled "Birth date" of kind DATE
    Then the catalogue lists "birthDate" with label "Birth date" and kind DATE

  @wip
  Scenario: A duplicate name is refused
    Given the field "birthDate" labelled "Birth date" of kind DATE is defined
    When the operator defines the field "birthDate" labelled "Another" of kind TEXT
    Then the definition is refused for a taken name

  @wip
  Scenario: A name the schema already owns is refused
    When the operator defines the field "name" labelled "Name" of kind TEXT
    Then the definition is refused for a reserved name

  @wip
  Scenario: A malformed name is refused
    When the operator defines the field "birth date" labelled "Birth date" of kind DATE
    Then the definition is refused for a malformed name

  @wip
  Scenario: Archiving a field hides it from the catalogue
    Given the field "birthDate" labelled "Birth date" of kind DATE is defined
    When the operator archives the field "birthDate"
    Then the catalogue does not list "birthDate"
    And the catalogue lists "birthDate" among archived definitions
