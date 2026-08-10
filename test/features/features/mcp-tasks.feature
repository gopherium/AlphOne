Feature: An agent lists tasks
  The list_my_tasks tool reads the token owner's tasks for one day or the
  overdue backlog, with contact names resolved beside their ids.

  Background:
    Given a running AlphOne holding a user with an API token
    And the agent is connected with the token

  @wip
  Scenario: Today's list resolves contact names
    Given a contact "Maria Perez"
    And an open task "Call Maria Perez back" due today linked to that contact
    And an open task "Send the quote" due today
    When the agent calls list_my_tasks
    Then the answer lists 2 tasks
    And the task "Call Maria Perez back" names its contact "Maria Perez"

  @wip
  Scenario: A chosen day lists only that day
    Given an open task "Prepare the renewal" due on 2026-09-01
    And an open task "Send the quote" due today
    When the agent calls list_my_tasks for 2026-09-01
    Then the answer lists 1 task
    And the task "Prepare the renewal" is listed

  @wip
  Scenario: The overdue backlog lists work due before today
    Given an open task "Chase the invoice" due yesterday
    And an open task "Send the quote" due today
    When the agent calls list_my_tasks for work due before today
    Then the answer lists 1 task
    And the task "Chase the invoice" is listed

  @wip
  Scenario: Done work arrives on request
    Given a done task "Book the courier" due today
    And an open task "Send the quote" due today
    When the agent calls list_my_tasks with status "done"
    Then the answer lists 1 task
    And the task "Book the courier" is listed

  @wip
  Scenario: Naming a day and the backlog together is refused
    When the agent calls list_my_tasks with both a date and due_before
    Then the tool fails with a validation error
