Feature: The graph answers the reader's locale
  AlphOne serves one locale per reader. A stored choice wins, the
  Accept-Language header narrows an anonymous ask, and the default
  answers when nothing else does.

  Background:
    Given a running AlphOne holding a user with an API token

  Scenario: An anonymous ask is answered the default
    When an anonymous caller asks for the locale
    Then the locale answered is "en-US"

  Scenario: The Accept-Language header narrows an anonymous ask
    When an anonymous caller asks for the locale speaking "es"
    Then the locale answered is "es-ES"

  Scenario: A stored choice wins over the header
    Given the caller stored the locale "es-ES"
    When the caller asks for the locale speaking "en"
    Then the locale answered is "es-ES"

  Scenario: A choice outside the list is refused naming its reason
    When the caller stores the locale "de-DE"
    Then the ask is refused naming the reason "locale_unknown"
