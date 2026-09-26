Feature: A repeater keeps a list of entries on a contact
  A repeater field holds rows of sub fields, such as a history log where each
  entry has a date and a comment. The definition names its sub fields, each
  with a label and a kind, and every written row is checked cell by cell
  against them.

  Background:
    Given a running AlphOne holding a user with an API token
    And a contact named "Maria Perez"

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

  Scenario: Sub fields sharing a label keep the distinct names the client sends
    When the operator defines the repeater "calls" labelled "Calls" with sub fields:
      | name  | label | kind |
      | note  | Note  | TEXT |
      | note2 | Note  | TEXT |
    Then the repeater "calls" lists the sub fields:
      | name  | label | kind |
      | note  | Note  | TEXT |
      | note2 | Note  | TEXT |

  Scenario: Two sub fields with one name are refused
    When the operator defines the repeater "calls" labelled "Calls" with sub fields:
      | name | label | kind |
      | note | Note  | TEXT |
      | note | Other | TEXT |
    Then the definition is refused with the reason "field_sub_field_name_taken"

  Scenario: A repeater with no sub fields is refused
    When the operator defines the repeater "history" labelled "History" with no sub fields
    Then the definition is refused with the reason "field_sub_fields_required"

  Scenario: Only a repeater holds sub fields
    When the operator defines the field "birthDate" labelled "Birth date" of kind DATE with sub fields:
      | name | label | kind |
      | day  | Day   | TEXT |
    Then the definition is refused with the reason "field_sub_fields_unexpected"

  Scenario: A repeater inside a repeater is refused
    When the operator defines the repeater "history" labelled "History" with sub fields:
      | name    | label   | kind     |
      | entries | Entries | REPEATER |
    Then the definition is refused with the reason "field_sub_field_nested"

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

  Scenario: A row naming an unknown sub field is refused
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | date    | Date    | DATE     |
      | comment | Comment | LONGTEXT |
    When the operator writes the rows into "history" of the contact:
      | date       | mood  |
      | 2026-09-01 | happy |
    Then the write is refused naming "history[0].mood" as the bad key

  Scenario: A row holding a value of the wrong kind is refused
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | date    | Date    | DATE     |
      | comment | Comment | LONGTEXT |
    When the operator writes the rows into "history" of the contact:
      | date       | comment    |
      | not a date | First call |
    Then the write is refused for a value of the wrong kind

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

  Scenario: Introspection lists a repeater typed JSON
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | date    | Date    | DATE     |
      | comment | Comment | LONGTEXT |
    When the Contact type is introspected
    Then the introspection lists "history" answering the scalar "JSON"

  Scenario: A repeater is not offered as an import column
    When the operator defines the repeater "history" labelled "History" with sub fields:
      | name    | label   | kind     |
      | date    | Date    | DATE     |
      | comment | Comment | LONGTEXT |
    Then the catalogue lists "history" with label "History" and kind REPEATER
    And the mapping registry does not list "history"

  Scenario: A sub field named id is refused
    When the operator defines the repeater "history" labelled "History" with sub fields:
      | name | label | kind |
      | id   | ID    | TEXT |
    Then the definition is refused with the reason "field_sub_field_name_reserved"

  @wip
  Scenario: Adding an entry stores it with an id
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | date    | Date    | DATE     |
      | comment | Comment | LONGTEXT |
    When the operator adds an entry to "history" of the contact:
      | date       | comment    |
      | 2026-09-01 | First call |
    Then the add answers the entry with an id
    And the contact's "history" answers the entries, newest first:
      | date       | comment    |
      | 2026-09-01 | First call |

  @wip
  Scenario: Entries answer newest first
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | date    | Date    | DATE     |
      | comment | Comment | LONGTEXT |
    When the operator adds the entries to "history" of the contact, oldest first:
      | date       | comment        |
      | 2026-09-01 | First call     |
      | 2026-09-10 | Sent the offer |
    Then the contact's "history" answers the entries, newest first:
      | date       | comment        |
      | 2026-09-10 | Sent the offer |
      | 2026-09-01 | First call     |
    And every entry keeps the id its add answered

  @wip
  Scenario: An entry naming an unknown sub field is refused
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | date    | Date    | DATE     |
      | comment | Comment | LONGTEXT |
    When the operator adds an entry to "history" of the contact:
      | date       | mood  |
      | 2026-09-01 | happy |
    Then the write is refused naming "history.mood" as the bad key

  @wip
  Scenario: An entry naming its own id on an add is refused
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | comment | Comment | LONGTEXT |
    When the operator adds the raw entry {"id": "0199a3c4-0000-7000-8000-000000000001", "comment": "First call"} to "history" of the contact
    Then the write is refused naming "history.id" as the bad key

  @wip
  Scenario: An entry holding a value of the wrong kind is refused
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | date    | Date    | DATE     |
      | comment | Comment | LONGTEXT |
    When the operator adds an entry to "history" of the contact:
      | date       | comment    |
      | not a date | First call |
    Then the write is refused for a value of the wrong kind

  @wip
  Scenario Outline: An entry with every cell blank is refused
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | date    | Date    | DATE     |
      | comment | Comment | LONGTEXT |
    When the operator adds the raw entry <entry> to "history" of the contact
    Then the change is refused with the reason "field_entry_empty"

    Examples:
      | entry                         |
      | {}                            |
      | {"date": null, "comment": ""} |
      | {"comment": "   "}            |

  @wip
  Scenario: Adding to a field that is not a repeater is refused
    Given the field "birthDate" labelled "Birth date" of kind DATE is defined
    When the operator adds an entry to "birthDate" of the contact:
      | date       |
      | 2026-09-01 |
    Then the change is refused with the reason "field_not_a_repeater"

  @wip
  Scenario: Adding to an archived repeater is refused
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | comment | Comment | LONGTEXT |
    And the operator archives the field "history"
    When the operator adds an entry to "history" of the contact:
      | comment    |
      | First call |
    Then the write is refused naming "history" as the bad key

  @wip
  Scenario: Editing an entry changes only its cells and keeps its id and its place
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | date    | Date    | DATE     |
      | comment | Comment | LONGTEXT |
    And the operator added the entries to "history" of the contact, oldest first:
      | date       | comment        |
      | 2026-09-01 | First call     |
      | 2026-09-10 | Sent the offer |
    When the operator edits the entry "First call" of "history" to:
      | date       | comment               |
      | 2026-09-02 | First call, moved out |
    Then the contact's "history" answers the entries, newest first:
      | date       | comment               |
      | 2026-09-10 | Sent the offer        |
      | 2026-09-02 | First call, moved out |
    And every entry keeps the id its add answered

  @wip
  Scenario: Editing an entry after another add still changes that entry
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | comment | Comment | LONGTEXT |
    And the operator added the entries to "history" of the contact, oldest first:
      | comment    |
      | First call |
    And the operator added the entries to "history" of the contact, oldest first:
      | comment        |
      | Sent the offer |
    When the operator edits the entry "First call" of "history" to:
      | comment             |
      | First call, amended |
    Then the contact's "history" answers the entries, newest first:
      | comment             |
      | Sent the offer      |
      | First call, amended |

  @wip
  Scenario: An edit that sends the entry's own id back is accepted
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | comment | Comment | LONGTEXT |
    And the operator added the entries to "history" of the contact, oldest first:
      | comment    |
      | First call |
    When the operator edits the entry "First call" of "history" sending its own id with:
      | comment             |
      | First call, amended |
    Then the contact's "history" answers the entries, newest first:
      | comment             |
      | First call, amended |
    And every entry keeps the id its add answered

  @wip
  Scenario: An edit of the wrong kind is refused and leaves the entry unchanged
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | date    | Date    | DATE     |
      | comment | Comment | LONGTEXT |
    And the operator added the entries to "history" of the contact, oldest first:
      | date       | comment    |
      | 2026-09-01 | First call |
    When the operator edits the entry "First call" of "history" to:
      | date       | comment    |
      | not a date | First call |
    Then the write is refused for a value of the wrong kind
    And the contact's "history" answers the entries, newest first:
      | date       | comment    |
      | 2026-09-01 | First call |

  @wip
  Scenario: An edit with every cell blank is refused and leaves the entry unchanged
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | comment | Comment | LONGTEXT |
    And the operator added the entries to "history" of the contact, oldest first:
      | comment    |
      | First call |
    When the operator edits the entry "First call" of "history" to the raw entry {"comment": "  "}
    Then the change is refused with the reason "field_entry_empty"
    And the contact's "history" answers the entries, newest first:
      | comment    |
      | First call |

  @wip
  Scenario: Editing an entry that was removed is refused
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | comment | Comment | LONGTEXT |
    And the operator added the entries to "history" of the contact, oldest first:
      | comment    |
      | First call |
    And the operator removes the entry "First call" from "history"
    When the operator edits the entry "First call" of "history" to:
      | comment             |
      | First call, amended |
    Then the change is refused with the reason "field_entry_not_found"

  @wip
  Scenario: Deleting an entry drops only that entry
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | comment | Comment | LONGTEXT |
    And the operator added the entries to "history" of the contact, oldest first:
      | comment        |
      | First call     |
      | Sent the offer |
      | Signed         |
    When the operator removes the entry "Sent the offer" from "history"
    Then the contact's "history" answers the entries, newest first:
      | comment    |
      | Signed     |
      | First call |
    And every entry keeps the id its add answered

  @wip
  Scenario: Deleting the last entry clears the repeater
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | comment | Comment | LONGTEXT |
    And the operator added the entries to "history" of the contact, oldest first:
      | comment    |
      | First call |
    When the operator removes the entry "First call" from "history"
    Then the contact "Maria Perez" answers null for the field "history"

  @wip
  Scenario: Deleting an unknown entry is refused
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | comment | Comment | LONGTEXT |
    And the operator added the entries to "history" of the contact, oldest first:
      | comment    |
      | First call |
    When the operator removes an unknown entry from "history"
    Then the change is refused with the reason "field_entry_not_found"
    And the contact's "history" answers the entries, newest first:
      | comment    |
      | First call |

  @wip
  Scenario: Entry ids survive archiving the repeater and defining it again
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | comment | Comment | LONGTEXT |
    And the operator added the entries to "history" of the contact, oldest first:
      | comment    |
      | First call |
    When the operator archives the field "history"
    And the operator defines the repeater "history" labelled "History" with sub fields:
      | name    | label   | kind     |
      | comment | Comment | LONGTEXT |
    Then the contact's "history" answers the entries, newest first:
      | comment    |
      | First call |
    And every entry keeps the id its add answered

  @wip
  Scenario: Saving other fields keeps the entries
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | comment | Comment | LONGTEXT |
    And the field "birthDate" labelled "Birth date" of kind DATE is defined
    And the operator added the entries to "history" of the contact, oldest first:
      | comment    |
      | First call |
    When the operator writes "1990-04-17" into "birthDate" of the contact
    Then the contact's "history" answers the entries, newest first:
      | comment    |
      | First call |
    And every entry keeps the id its add answered

  @wip
  Scenario: Writing a repeater through the field values is refused
    Given the repeater "history" labelled "History" is defined with sub fields:
      | name    | label   | kind     |
      | comment | Comment | LONGTEXT |
    When the operator writes the rows into "history" of the contact:
      | comment    |
      | First call |
    Then the change is refused with the reason "field_repeater_entries_only"
