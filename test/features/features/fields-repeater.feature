Feature: A repeater keeps a list of entries on a contact
  A repeater field holds rows of sub fields, such as a history log where each
  entry has a date and a comment. The definition names its sub fields, each
  with a label and a kind, and every written row is checked cell by cell
  against them.

  Background:
    Given a running AlphOne holding a user with an API token
    And a contact named "Maria Perez"

  @wip
  Scenario: Defining a repeater lists it with its sub fields
    When the operator defines the repeater "history" labelled "History" with sub fields:
      | name    | label   | kind     |
      | date    | Date    | DATE     |
      | comment | Comment | LONGTEXT |
    Then the catalogue lists "history" with label "History" and kind REPEATER
    And the repeater "history" lists the sub fields:
      | name    | label   | kind     |
      | date    | Date    | DATE     |
      | comment | Comment | LONGTEXT |

  @wip
  Scenario: Sub fields sharing a label keep the distinct names the client sends
    When the operator defines the repeater "calls" labelled "Calls" with sub fields:
      | name  | label | kind |
      | note  | Note  | TEXT |
      | note2 | Note  | TEXT |
    Then the repeater "calls" lists the sub fields:
      | name  | label | kind |
      | note  | Note  | TEXT |
      | note2 | Note  | TEXT |

  @wip
  Scenario: Two sub fields with one name are refused
    When the operator defines the repeater "calls" labelled "Calls" with sub fields:
      | name | label | kind |
      | note | Note  | TEXT |
      | note | Other | TEXT |
    Then the definition is refused with the reason "field_sub_field_name_taken"

  @wip
  Scenario: A repeater with no sub fields is refused
    When the operator defines the repeater "history" labelled "History" with no sub fields
    Then the definition is refused with the reason "field_sub_fields_required"

  @wip
  Scenario: Only a repeater holds sub fields
    When the operator defines the field "birthDate" labelled "Birth date" of kind DATE with sub fields:
      | name | label | kind |
      | day  | Day   | TEXT |
    Then the definition is refused with the reason "field_sub_fields_unexpected"

  @wip
  Scenario: A repeater inside a repeater is refused
    When the operator defines the repeater "history" labelled "History" with sub fields:
      | name    | label   | kind     |
      | entries | Entries | REPEATER |
    Then the definition is refused with the reason "field_sub_field_nested"

  @wip
  Scenario: A repeater answers its rows in the order written
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | date    | Date    | DATE     |
      | comment | Comment | LONGTEXT |
    When the operator writes the rows into "history" of the contact:
      | date       | comment        |
      | 2026-09-10 | Sent the offer |
      | 2026-09-01 | First call     |
    Then querying the contact for "history" answers the rows:
      | date       | comment        |
      | 2026-09-10 | Sent the offer |
      | 2026-09-01 | First call     |

  @wip
  Scenario: A row naming an unknown sub field is refused
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | date    | Date    | DATE     |
      | comment | Comment | LONGTEXT |
    When the operator writes the rows into "history" of the contact:
      | date       | mood  |
      | 2026-09-01 | happy |
    Then the write is refused naming "history[0].mood" as the bad key

  @wip
  Scenario: A row holding a value of the wrong kind is refused
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | date    | Date    | DATE     |
      | comment | Comment | LONGTEXT |
    When the operator writes the rows into "history" of the contact:
      | date       | comment    |
      | not a date | First call |
    Then the write is refused for a value of the wrong kind

  @wip
  Scenario: Writing no rows clears the repeater
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | date    | Date    | DATE     |
      | comment | Comment | LONGTEXT |
    And the operator writes the rows into "history" of the contact:
      | date       | comment    |
      | 2026-09-01 | First call |
    When the operator writes no rows into "history" of the contact
    Then the contact "Maria Perez" answers null for the field "history"

  @wip
  Scenario: An archived repeater defined again with other sub fields is refused
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | date    | Date    | DATE     |
      | comment | Comment | LONGTEXT |
    When the operator archives the field "history"
    And the operator defines the repeater "history" labelled "History" with sub fields:
      | name | label | kind |
      | date | Date  | DATE |
    Then the definition is refused with the reason "field_kind_locked"

  @wip
  Scenario: An archived repeater defined again with the same sub fields answers its rows
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | date    | Date    | DATE     |
      | comment | Comment | LONGTEXT |
    And the operator writes the rows into "history" of the contact:
      | date       | comment    |
      | 2026-09-01 | First call |
    When the operator archives the field "history"
    And the operator defines the repeater "history" labelled "History" with sub fields:
      | name    | label   | kind     |
      | date    | Date    | DATE     |
      | comment | Comment | LONGTEXT |
    Then querying the contact for "history" answers the rows:
      | date       | comment    |
      | 2026-09-01 | First call |

  @wip
  Scenario: Introspection lists a repeater typed JSON
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | date    | Date    | DATE     |
      | comment | Comment | LONGTEXT |
    When the Contact type is introspected
    Then the introspection lists "history" answering the scalar "JSON"

  @wip
  Scenario: A repeater is not offered as an import column
    When the operator defines the repeater "history" labelled "History" with sub fields:
      | name    | label   | kind     |
      | date    | Date    | DATE     |
      | comment | Comment | LONGTEXT |
    Then the catalogue lists "history" with label "History" and kind REPEATER
    And the mapping registry does not list "history"
