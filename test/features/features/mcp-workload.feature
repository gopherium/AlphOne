Feature: An agent reads the day's workload
  The workload_summary tool answers the headline numbers, how many open
  tasks the token's owner holds for today and from before. It answers data
  only, the phrasing belongs to the agent reading it.

  Background:
    Given a running AlphOne holding a user with an API token
    And the agent is connected with the token

  Scenario: The counts cover today and the backlog
    Given 10 open tasks due today
    And 3 open tasks due yesterday
    When the agent calls workload_summary
    Then the structured answer counts 10 due today and 3 overdue

  Scenario: A day without work counts zero
    When the agent calls workload_summary
    Then the structured answer counts 0 due today and 0 overdue

  Scenario: Another user's tasks never count
    Given 10 open tasks due today
    And a second user holding 5 open tasks due today
    When the agent calls workload_summary
    Then the structured answer counts 10 due today and 0 overdue

  Scenario: Done tasks never count
    Given 4 open tasks due today
    And 2 done tasks due today
    When the agent calls workload_summary
    Then the structured answer counts 4 due today and 0 overdue

  Scenario: A very long day is marked capped
    Given 201 open tasks due today
    When the agent calls workload_summary
    Then the structured answer counts 200 due today and 0 overdue
    And the structured answer is marked capped
