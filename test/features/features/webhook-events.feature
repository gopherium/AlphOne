Feature: Plugins declare the events a webhook can carry
  A webhook carries the events it names. Core publishes its own events and
  every plugin declares the events it publishes, so a webhook accepts a name
  only when core or a running plugin declared it. Clients read the names
  from AlphOne instead of keeping their own list.

  Background:
    Given a running AlphOne holding a user with an API token
    And the user holds a token scoped to "webhooks:write"

  Scenario: A webhook carries an event a plugin declared
    When that token registers a webhook for the event "whatsapp.message.received"
    Then the webhook is registered for the event "whatsapp.message.received"

  Scenario: A webhook for an event nobody declared is refused
    When that token registers a webhook for the event "invoice.paid"
    Then the webhook is refused as an unknown event
    And that token lists no webhook

  Scenario: The event list names the core events and every plugin event
    When that token lists the webhook events
    Then the event list names
      | task.created              |
      | task.completed            |
      | contact.created           |
      | import.completed          |
      | whatsapp.message.received |
