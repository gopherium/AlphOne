Feature: An agent lists tasks
  The list_my_tasks tool reads the token owner's tasks for one day or the
  overdue backlog, with contact names resolved beside their ids.

  Background:
    Given a running AlphOne holding a user with an API token
    And the agent is connected with the token

  Scenario: Today's list resolves contact names
    Given a contact "Maria Perez"
    And an open task "Call Maria Perez back" due today linked to that contact
    And an open task "Send the quote" due today
    When the agent calls list_my_tasks
    Then the answer lists 2 tasks
    And the task "Call Maria Perez back" names its contact "Maria Perez"

  Scenario: A chosen day lists only that day
    Given an open task "Prepare the renewal" due on 2020-01-15
    And an open task "Send the quote" due today
    When the agent calls list_my_tasks for 2020-01-15
    Then the answer lists 1 task
    And the task "Prepare the renewal" is listed

  Scenario: The overdue backlog lists work due before today
    Given an open task "Chase the invoice" due yesterday
    And an open task "Send the quote" due today
    When the agent calls list_my_tasks for work due before today
    Then the answer lists 1 task
    And the task "Chase the invoice" is listed

  Scenario: Done work arrives on request
    Given a done task "Book the courier" due today
    And an open task "Send the quote" due today
    When the agent calls list_my_tasks with status "done"
    Then the answer lists 1 task
    And the task "Book the courier" is listed

  Scenario: A raised task lists above the normal work of its day
    Given an open task "Send the quote" due today
    And an open task "Approve the pricing" due today with priority 1
    When the agent calls list_my_tasks
    Then the answer lists the tasks in order:
      | title               |
      | Approve the pricing |
      | Send the quote      |

  Scenario: A higher priority lists above a lower one
    Given an open task "Renew the license" due today with priority 1
    And an open task "Restore the backup" due today with priority 9
    And an open task "Send the quote" due today
    When the agent calls list_my_tasks
    Then the answer lists the tasks in order:
      | title              |
      | Restore the backup |
      | Renew the license  |
      | Send the quote     |

  Scenario: The overdue backlog lists raised work first across every late day
    Given an open task "Chase the invoice" due on 2020-01-10
    And an open task "Renew the contract" due yesterday with priority 1
    And an open task "File the report" due on 2020-01-12
    When the agent calls list_my_tasks for work due before today
    Then the answer lists the tasks in order:
      | title              |
      | Renew the contract |
      | Chase the invoice  |
      | File the report    |

  Scenario: Done work keeps the order it was created in
    Given a done task "Book the courier" due today
    And a done task "File the receipt" due today with priority 1
    When the agent calls list_my_tasks with status "done"
    Then the answer lists the tasks in order:
      | title            |
      | Book the courier |
      | File the receipt |

  Scenario: Priority comes first and the earlier hour next
    Given an open task "Approve the pricing" due today at 11:00 with priority 1
    And an open task "Send the quote" due today at 09:00
    And an open task "Renew the license" due today at 09:00 with priority 1
    When the agent calls list_my_tasks
    Then the answer lists the tasks in order:
      | title               |
      | Renew the license   |
      | Approve the pricing |
      | Send the quote      |

  Scenario: Work with no time lists after the timed work of its priority
    Given an open task "Update the contact list" due today
    And an open task "Send the quote" due today at 10:30
    And an open task "Call Ada" due today at 09:00
    When the agent calls list_my_tasks
    Then the answer lists the tasks in order:
      | title                   |
      | Call Ada                |
      | Send the quote          |
      | Update the contact list |

  Scenario: The overdue backlog lists by priority, then day, then hour
    Given an open task "Approve the pricing" due on 2020-01-12 at 15:00 with priority 1
    And an open task "Renew the contract" due on 2020-01-12 at 09:00 with priority 1
    And an open task "Book the venue" due on 2020-01-11 at 18:00 with priority 1
    And an open task "Chase the invoice" due on 2020-01-10 at 17:00
    And an open task "File the report" due on 2020-01-10 at 08:00
    When the agent calls list_my_tasks for work due before today
    Then the answer lists the tasks in order:
      | title               |
      | Book the venue      |
      | Renew the contract  |
      | Approve the pricing |
      | File the report     |
      | Chase the invoice   |

  Scenario: Done work keeps the order it was created in whatever its hour
    Given a done task "Book the courier" due today at 11:00
    And a done task "File the receipt" due today at 09:00
    When the agent calls list_my_tasks with status "done"
    Then the answer lists the tasks in order:
      | title            |
      | Book the courier |
      | File the receipt |

  Scenario: The answer gives each task its time
    Given an open task "Send the quote" due today at 09:30
    And an open task "Update the contact list" due today
    When the agent calls list_my_tasks
    Then the answer gives "Send the quote" the time "09:30"
    And the answer gives "Update the contact list" no time

  Scenario: Naming a day and the backlog together is refused
    When the agent calls list_my_tasks with both a date and due_before
    Then the tool fails with a validation error
