Feature: Admins manage every webhook in their workspace
  A webhook sends workspace events to an outside server for as long as it
  lives. Creating one needs the manage_webhooks capability, which admins
  hold. Holders list and delete every webhook in their workspace, each with
  its owner. Everyone else lists and deletes only their own.

  Background:
    Given a running AlphOne holding an admin and a member

  Scenario: A member's token cannot register a webhook
    Given the member holds a token scoped to "webhooks:write"
    When that token registers a webhook to "https://example.com/hook"
    Then the operation is refused as admin only naming "webhooks:write"
    And the refusal names the capability "manage_webhooks"

  Scenario: An admin's token registers a webhook
    Given the admin holds a token scoped to "webhooks:write"
    When that token registers a webhook to "https://example.com/hook"
    Then the webhook is registered for "https://example.com/hook"

  Scenario: An admin lists a member's webhook with its owner
    Given the member owns a webhook to "https://example.com/member-hook"
    When the admin's session lists the webhooks
    Then the list shows "https://example.com/member-hook" owned by "Maria Perez" at "member@example.com"

  Scenario: An admin deletes a member's webhook
    Given the member owns a webhook to "https://example.com/member-hook"
    When the admin's session deletes the member's webhook
    Then the deletion is answered
    And the member's session lists no webhook

  Scenario: A member lists and deletes only its own webhooks
    Given the member owns a webhook to "https://example.com/member-hook"
    And the admin owns a webhook to "https://example.com/admin-hook"
    When the member's session lists the webhooks
    Then the list holds only "https://example.com/member-hook"
    When the member's session deletes the admin's webhook
    Then the deletion is refused as not found
    And the admin's session still lists "https://example.com/admin-hook"
    When the member's session deletes the member's webhook
    Then the deletion is answered
