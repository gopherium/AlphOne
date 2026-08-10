Feature: An agent reads the day's workload
  The workload_summary tool answers the headline question, how much work
  the token's owner holds right now.

  Background:
    Given a running AlphOne holding a user with an API token
    And the agent is connected with the token

  @wip
  Scenario: The summary counts today and the backlog
    Given 10 open tasks due today
    And 3 open tasks due yesterday
    When the agent calls workload_summary
    Then the summary reads "You have 10 tasks due today and 3 overdue."
    And the structured answer counts 10 due today and 3 overdue

  @wip
  Scenario: A day without work reads as free
    When the agent calls workload_summary
    Then the summary reads "You have no tasks due today and none overdue."

  @wip
  Scenario: Another user's tasks never count
    Given 10 open tasks due today
    And a second user holding 5 open tasks due today
    When the agent calls workload_summary
    Then the structured answer counts 10 due today and 0 overdue

  @wip
  Scenario: Done tasks never count
    Given 4 open tasks due today
    And 2 done tasks due today
    When the agent calls workload_summary
    Then the structured answer counts 4 due today and 0 overdue

  @wip
  Scenario: A very long day counts as at least the page
    Given 201 open tasks due today
    When the agent calls workload_summary
    Then the summary reads "You have at least 200 tasks due today and none overdue."
    And the structured answer is marked capped
