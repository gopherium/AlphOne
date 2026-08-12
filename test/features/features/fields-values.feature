Feature: Field values travel the graph by name
  A defined field is a real graph field on Contact. Clients query it by its
  own name with no rebuild. Writes go through one mutation that checks every
  value against the kind its definition declares.

  Background:
    Given a running AlphOne holding a user with an API token
    And a contact named "Maria Perez"

  @wip
  Scenario: A defined field answers its written value by name
    Given the field "birthDate" labelled "Birth date" of kind DATE is defined
    When the operator writes "1990-04-17" into "birthDate" of the contact
    Then querying the contact for "birthDate" answers "1990-04-17"

  @wip
  Scenario: An undefined field is refused by the graph
    When the contact is queried for the field "neverDefined"
    Then the graph refuses the query as an unknown field

  @wip
  Scenario: A write naming an undefined field is refused
    When the operator writes "1990-04-17" into "neverDefined" of the contact
    Then the write is refused naming "neverDefined" as the bad key

  @wip
  Scenario: A write of the wrong kind is refused
    Given the field "birthDate" labelled "Birth date" of kind DATE is defined
    When the operator writes "not a date" into "birthDate" of the contact
    Then the write is refused for a value of the wrong kind

  @wip
  Scenario: An archived field stops answering and its value survives
    Given the field "birthDate" labelled "Birth date" of kind DATE is defined
    And the operator writes "1990-04-17" into "birthDate" of the contact
    When the operator archives the field "birthDate"
    Then querying the contact for "birthDate" is refused as an unknown field
    When the operator defines the field "birthDate" labelled "Birth date" of kind DATE
    Then querying the contact for "birthDate" answers "1990-04-17"
