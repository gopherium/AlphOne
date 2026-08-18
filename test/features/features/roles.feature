Feature: A role narrows what a user may do
  A role narrows the user where a scope narrows the token, and what an
  operation gets is the intersection of both. Admins manage users,
  members work the product, and every user from before roles stands as
  an admin so no deployment locks itself out.

  Background:
    Given a running AlphOne holding an admin and a member

  Scenario: A user created by an admin arrives a member
    When the admin's session creates a user named "Grace Hopper"
    Then the user list shows "Grace Hopper" as a "member"

  @wip
  Scenario: A member is refused user management
    When the member's session disables another user
    Then the operation is refused as admin only naming "users:write"

  Scenario: A member keeps working the contacts and tasks
    When the member's session creates a contact named "Maria Perez"
    Then the contact is answered
    When the member's session creates a task titled "Call the supplier"
    Then the task is answered

  @wip
  Scenario: A member's full scope token is held to the member's role
    Given the member holds a token scoped to "*"
    When that token disables a user
    Then the operation is refused as admin only naming "users:write"

  @wip
  Scenario: An admin's narrow token stays narrow
    Given the admin holds a token scoped to "contacts:read"
    When that token creates a contact named "Maria Perez"
    Then the operation is refused as unauthorized naming "contacts:write"

  Scenario: A user from before roles stands as admin
    Given a user provisioned before roles existed
    Then that user's session may disable another user

  @wip
  Scenario: A session sees its own role, and promotion changes it
    Then the member's session sees its role as "member"
    When the admin's session promotes the member to "admin"
    Then the member's session sees its role as "admin"

  @wip
  Scenario: The last admin cannot be demoted
    When the admin's session demotes itself to "member"
    Then the operation is refused as the last admin
