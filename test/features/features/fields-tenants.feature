Feature: Each tenant keeps its own contact fields
  Every tenant defines its own contact fields. The graph checks a write
  against the caller's own fields and shows each caller only the fields
  its own tenant defined. A caller with no session learns no field name.

  Background:
    Given a running AlphOne holding a user with an API token
    And the tenant "Acme" exists
    And a user "acme.member@example.com" holding a token is placed in the tenant "Acme"

  Scenario: A tenant writes its own field after another tenant defined one
    Given a contact named "Maria Perez"
    And the field "birthDate" labelled "Birth date" of kind DATE is defined
    And the tenant "Acme" defines the field "shoeSize" labelled "Shoe size" of kind TEXT
    When the operator writes "1990-04-17" into "birthDate" of the contact
    Then the write is accepted
    And querying the contact for "birthDate" answers "1990-04-17"

  Scenario: A tenant cannot store a value under another tenant's field name
    Given a contact named "Maria Perez"
    And the tenant "Acme" defines the field "shoeSize" labelled "Shoe size" of kind TEXT
    When the operator writes "44" into "shoeSize" of the contact
    Then the write is refused naming "shoeSize" as the bad key

  Scenario: A tenant cannot read another tenant's field by name
    Given a contact named "Maria Perez"
    And the field "birthDate" labelled "Birth date" of kind DATE is defined
    When the tenant "Acme" queries the contact for the field "birthDate"
    Then the graph refuses the query as an unknown field

  Scenario: Introspection shows each tenant only its own fields
    Given the field "birthDate" labelled "Birth date" of kind DATE is defined
    And the tenant "Acme" defines the field "shoeSize" labelled "Shoe size" of kind TEXT
    When the Contact type is introspected
    Then the introspection lists "birthDate" answering the scalar "Date"
    And the introspection does not list "shoeSize"
    When the tenant "Acme" introspects the Contact type
    Then the introspection lists "shoeSize" answering the scalar "String"
    And the introspection does not list "birthDate"

  Scenario: A caller with no session learns no field name
    Given the field "birthDate" labelled "Birth date" of kind DATE is defined
    When an anonymous caller asks a contact for the field "birthDat"
    Then the answer does not name "birthDate"

  Scenario: The import mapping offers only the caller's own fields
    Given the tenant "Acme" defines the field "shoeSize" labelled "Shoe size" of kind TEXT
    When the operator defines the field "birthDate" labelled "Birth date" of kind DATE
    Then the mapping registry lists "birthDate" labelled "Birth date" beside the core columns
    And the mapping registry does not list "shoeSize"

  Scenario: An import fills the caller's own field after another tenant defined one
    Given the field "birthDate" labelled "Birth date" of kind DATE is defined
    And an uploaded spreadsheet holding the row "Maria Perez,maria@example.com,1990-04-17"
    And the columns are mapped onto name, email and the field "birthDate"
    And the tenant "Acme" defines the field "shoeSize" labelled "Shoe size" of kind TEXT
    When the import is committed
    Then the commit answers 1 imported
    And the contact "Maria Perez" answers "1990-04-17" for the field "birthDate"
