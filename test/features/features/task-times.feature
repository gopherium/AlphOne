Feature: A task keeps a time of day
  A task can carry a wall clock time beside its due date. The time reads the
  same for everyone. Leaving it out of an update keeps it, and only an explicit
  null takes it away.

  Background:
    Given a running AlphOne holding a user with an API token

  Scenario: A new task keeps the time it was given
    When the owner creates the task "Call Ada" due on 2030-05-20 at 11:00
    Then the task "Call Ada" is due on 2030-05-20 at 11:00

  Scenario: A new task given no time carries none
    When the owner creates the task "Send the quote" due on 2030-05-20
    Then the task "Send the quote" is due on 2030-05-20 with no time

  Scenario: Moving a task to another day keeps its time
    Given the owner created the task "Call Ada" due on 2030-05-20 at 11:00
    When the owner moves the task "Call Ada" to 2030-05-21
    Then the task "Call Ada" is due on 2030-05-21 at 11:00

  Scenario: A new time replaces the old one
    Given the owner created the task "Call Ada" due on 2030-05-20 at 11:00
    When the owner sets the time of the task "Call Ada" to 09:30
    Then the task "Call Ada" is due on 2030-05-20 at 09:30

  Scenario: An explicit null clears the time
    Given the owner created the task "Call Ada" due on 2030-05-20 at 11:00
    When the owner clears the time of the task "Call Ada"
    Then the task "Call Ada" is due on 2030-05-20 with no time

  Scenario Outline: A time that is not a wall clock minute is refused
    When the owner creates the task "Call Ada" due on 2030-05-20 at "<time>"
    Then the graph refuses the value as malformed

    Examples:
      | time     |
      | 24:00    |
      | 9:00     |
      | 09:00:30 |
      | 11:60    |
