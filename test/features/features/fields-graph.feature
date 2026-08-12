Feature: The widened graph keeps the compiled schema's promises
  The mechanism widens the compiled schema with the defined fields and
  leaves everything else alone. Introspection tells the truth about what a
  field answers, and a core query answers exactly what it answered before
  any field existed.

  Background:
    Given a running AlphOne holding a user with an API token

  @wip
  Scenario: Introspection lists a defined field with its scalar
    Given the field "birthDate" labelled "Birth date" of kind DATE is defined
    When the Contact type is introspected
    Then the introspection lists "birthDate" answering the scalar "Date"

  Scenario: A core query answers the same with and without fields
    Given a contact named "Maria Perez"
    And the core contact listing is captured
    When the field "birthDate" labelled "Birth date" of kind DATE is defined
    Then the core contact listing answers byte identical to the capture
