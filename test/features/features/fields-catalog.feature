Feature: An operator shapes the field catalogue
  Contact fields are defined at runtime by an operator. A definition carries
  a machine name, a human label and a kind. The catalogue refuses names the
  compiled schema already owns, so a runtime field can never shadow a real
  one. It also refuses the names every JavaScript object already holds, so a
  browser reads each field as it was stored.

  Background:
    Given a running AlphOne holding a user with an API token

  Scenario: Defining a field lists it in the catalogue
    When the operator defines the field "birthDate" labelled "Birth date" of kind DATE
    Then the catalogue lists "birthDate" with label "Birth date" and kind DATE

  Scenario: A duplicate name is refused
    Given the field "birthDate" labelled "Birth date" of kind DATE is defined
    When the operator defines the field "birthDate" labelled "Another" of kind TEXT
    Then the definition is refused for a taken name

  Scenario: A name the schema already owns is refused
    When the operator defines the field "name" labelled "Name" of kind TEXT
    Then the definition is refused for a reserved name

  Scenario Outline: A name every JavaScript object holds is refused
    When the operator defines the field "<name>" labelled "Points" of kind NUMBER
    Then the definition is refused with the reason "field_name_reserved"
    And the catalogue does not list "<name>"

    Examples:
      | name                 |
      | constructor          |
      | hasOwnProperty       |
      | isPrototypeOf        |
      | propertyIsEnumerable |
      | toLocaleString       |
      | toString             |
      | valueOf              |

  Scenario: The catalogue lists every name a field cannot take
    When the operator asks which names a field cannot take
    Then the names a field cannot take are:
      | name                  |
      | constructor           |
      | createdAt             |
      | field                 |
      | hasOwnProperty        |
      | id                    |
      | identities            |
      | isPrototypeOf         |
      | name                  |
      | propertyIsEnumerable  |
      | tasks                 |
      | toLocaleString        |
      | toString              |
      | valueOf               |
      | whatsAppConversations |

  Scenario: A malformed name is refused
    When the operator defines the field "birth date" labelled "Birth date" of kind DATE
    Then the definition is refused for a malformed name

  Scenario: Archiving a field hides it from the catalogue
    Given the field "birthDate" labelled "Birth date" of kind DATE is defined
    When the operator archives the field "birthDate"
    Then the catalogue does not list "birthDate"
    And the catalogue lists "birthDate" among archived definitions

  Scenario: Defining an archived field again answers the field it brings back and lists it last
    Given the field "birthDate" labelled "Birth date" of kind DATE is defined
    And the field "shoeSize" labelled "Shoe size" of kind TEXT is defined
    And the operator archives the field "birthDate"
    When the operator defines the field "birthDate" labelled "Date of birth" of kind DATE
    Then the definition answers the id the catalogue lists for "birthDate"
    And the catalogue lists "birthDate" with label "Date of birth" and kind DATE
    And the catalogue lists the fields in order:
      | name      |
      | shoeSize  |
      | birthDate |

  Scenario: The operator orders the fields
    Given the field "birthDate" labelled "Birth date" of kind DATE is defined
    And the field "shoeSize" labelled "Shoe size" of kind TEXT is defined
    And the field "nickname" labelled "Nickname" of kind TEXT is defined
    When the operator orders the fields:
      | name      |
      | nickname  |
      | birthDate |
      | shoeSize  |
    Then the order is accepted
    And the catalogue lists the fields in order:
      | name      |
      | nickname  |
      | birthDate |
      | shoeSize  |

  Scenario: A new field goes to the end of the order
    Given the field "birthDate" labelled "Birth date" of kind DATE is defined
    And the field "shoeSize" labelled "Shoe size" of kind TEXT is defined
    And the operator orders the fields:
      | name      |
      | shoeSize  |
      | birthDate |
    When the operator defines the field "nickname" labelled "Nickname" of kind TEXT
    Then the catalogue lists the fields in order:
      | name      |
      | shoeSize  |
      | birthDate |
      | nickname  |

  Scenario: An order that leaves a field out is refused
    Given the field "birthDate" labelled "Birth date" of kind DATE is defined
    And the field "shoeSize" labelled "Shoe size" of kind TEXT is defined
    When the operator orders the fields:
      | name     |
      | shoeSize |
    Then the change is refused with the reason "field_order_incomplete"
    And the catalogue lists the fields in order:
      | name      |
      | birthDate |
      | shoeSize  |
