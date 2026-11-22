Feature: A panic in a plugin route costs only that request
  A plugin's routes run inside AlphOne. When a route panics before it
  answered, the caller gets AlphOne's own internal error, which never
  names the plugin. When a route panics after it began its answer, the
  connection is cut, so a half answer never looks complete. The server
  log names the plugin, the method, the path and the stack of each
  panic. The plugin keeps serving its other routes.

  Background:
    Given the settings point at a migrated database
    And AlphOne runs the plugin "probe", whose public route "/ping" answers 200

  Scenario: A route that panics answers the internal error and the plugin keeps serving
    Given the public route "/boom" of "probe" panics with "boom"
    When AlphOne serves
    Then the route "/boom" of "probe" answers 500 with the code "internal", kept out of every cache
    And the route "/ping" of "probe" answers 200
    And the server log holds once that GET "/api/plugins/probe/boom" of "probe" panicked with "boom", answered
    And the server log holds no "http: panic serving" line

  Scenario: A route that panics after it began its answer is cut
    Given the public route "/boom" of "probe" answers 200, then panics with "boom"
    When AlphOne serves
    Then the answer of the route "/boom" of "probe" is cut
    And the route "/ping" of "probe" answers 200
    And the server log holds once that GET "/api/plugins/probe/boom" of "probe" panicked with "boom", not answered

  Scenario: A route behind a token that panics answers the internal error
    Given the route "/private" of "probe" needs a token and panics with "boom"
    And a member of a workspace holds an API token
    When AlphOne serves
    Then the route "/private" of "probe" answers that token 500 with the code "internal", kept out of every cache
    And the server log holds once that GET "/api/plugins/probe/private" of "probe" panicked with "boom", answered
