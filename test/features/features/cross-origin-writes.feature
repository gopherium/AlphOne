Feature: A browser write from another origin is refused
  A page on another site can make a signed in user's browser post a form
  to AlphOne with that user's session. AlphOne refuses such a write before
  any route sees it. Its own pages, API tokens and agents keep writing.

  Background:
    Given a running AlphOne holding a user with an API token

  Scenario: A form from another site creating a contact is refused
    Given the user is signed in through the browser
    When a page on another site posts a form creating the contact "Maria Perez" through the user's browser
    Then the request is refused with the code "request_cross_origin"
    And the user's session finds no contact named "Maria Perez"

  Scenario: A form from a sibling site creating a contact is refused
    Given the user is signed in through the browser
    When a page on a sibling site posts a form creating the contact "Maria Perez" through the user's browser
    Then the request is refused with the code "request_cross_origin"
    And the user's session finds no contact named "Maria Perez"

  Scenario: An older browser posting a form from another site is refused
    Given the user is signed in through the browser
    When an older browser posts a form creating the contact "Maria Perez" from a page on another site
    Then the request is refused with the code "request_cross_origin"
    And the user's session finds no contact named "Maria Perez"

  Scenario: An older browser posting through a proxy that renames the host is refused
    Given the user is signed in through the browser
    When an older browser posts a form creating the contact "Maria Perez" from a page on this site through a proxy that renames the host
    Then the request is refused with the code "request_cross_origin"
    And the user's session finds no contact named "Maria Perez"

  Scenario: A sign in posted from another site is refused
    When a page on another site signs the user in through the visitor's browser
    Then the request is refused with the code "request_cross_origin"
    And no session cookie is answered

  Scenario: A sign out posted from another site leaves the session alive
    Given the user is signed in through the browser
    When a page on another site signs the user out through the user's browser
    Then the request is refused with the code "request_cross_origin"
    And the user's session still answers

  Scenario: A form from AlphOne's own page creates the contact
    Given the user is signed in through the browser
    When a page on this site posts a form creating the contact "Maria Perez" through the user's browser
    Then the form is answered
    And the user's session finds the contact named "Maria Perez"

  Scenario: An older browser posting from AlphOne's own page creates the contact
    Given the user is signed in through the browser
    When an older browser posts a form creating the contact "Maria Perez" from a page on this site
    Then the form is answered
    And the user's session finds the contact named "Maria Perez"

  Scenario: A token writes with no browser headers
    Given the user holds a token scoped to "contacts:write"
    When that token creates a contact named "Maria Perez"
    Then the contact is answered

  Scenario: An agent posts to the MCP endpoint with the token
    When an agent posts an MCP handshake with the token
    Then the handshake is answered
