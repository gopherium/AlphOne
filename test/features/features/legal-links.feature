Feature: The sign-in screens read the legal links
  An operator names the install's privacy policy and terms of service.
  The screens a person reaches before signing in show a link to each one,
  so those two addresses are the one thing besides the locale a caller
  with no identity can read.

  Background:
    Given a running AlphOne holding a user with an API token

  Scenario: An anonymous caller reads the addresses the operator set
    Given the operator set the privacy address "https://example.com/legal/privacy"
    And the operator set the terms address "https://example.com/legal/terms"
    When an anonymous caller reads the legal links
    Then the privacy address answered is "https://example.com/legal/privacy"
    And the terms address answered is "https://example.com/legal/terms"

  Scenario: An install that set neither answers none
    When an anonymous caller reads the legal links
    Then no privacy address is answered
    And no terms address is answered

  Scenario: One address set answers only that one
    Given the operator set the terms address "https://example.com/legal/terms"
    When an anonymous caller reads the legal links
    Then no privacy address is answered
    And the terms address answered is "https://example.com/legal/terms"

  Scenario: The boot read takes the locale and the links together
    Given the operator set the privacy address "https://example.com/legal/privacy"
    When an anonymous caller reads the locale and the legal links together
    Then the locale answered is "en-US"
    And the privacy address answered is "https://example.com/legal/privacy"

  Scenario: A signed in caller reads them too
    Given the operator set the privacy address "https://example.com/legal/privacy"
    When the caller reads the legal links
    Then the privacy address answered is "https://example.com/legal/privacy"

  Scenario: A private field beside the links is refused
    When an anonymous caller reads the legal links beside the version
    Then the ask is refused as unauthenticated

  Scenario: An alias does not reach a private field
    When an anonymous caller aliases the version as the legal links
    Then the ask is refused as unauthenticated
