Feature: An invitation arrives by mail and lets a person in
  An administrator never types somebody else's password. The server mails
  an activation link, or shows it for hand delivery when no relay is
  configured, and every answer reads the same whatever the address is so
  a listing cannot be used to discover who holds an account.

  Background:
    Given a running AlphOne holding an admin and a mail relay

  Scenario: An invitation carries a working activation link
    When the admin invites "grace@example.com" as "Grace Hopper"
    Then the relay holds one mail addressed to "grace@example.com"
    And the mail carries an activation link

  Scenario: The activation link signs the invited person in
    Given the admin invited "grace@example.com" as "Grace Hopper"
    When the invited person activates with the password "correct horse battery"
    Then the invited person holds a session
    And the account shows as confirmed

  Scenario: A taken address answers the same way as a fresh one
    Given the admin invited "grace@example.com" as "Grace Hopper"
    And the invited person activated with the password "correct horse battery"
    When the admin invites "grace@example.com" as "Grace Hopper"
    Then the invitation answers delivered

  Scenario: A spent activation link is refused
    Given the admin invited "grace@example.com" as "Grace Hopper"
    And the invited person activated with the password "correct horse battery"
    When the invited person activates with the password "another good password"
    Then the activation is refused as an invalid link

  Scenario: A reset ends every session the account holds
    Given the admin invited "grace@example.com" as "Grace Hopper"
    And the invited person activated with the password "correct horse battery"
    When the invited person resets to the password "brand new password"
    Then the earlier session is gone
    And the invited person signs in with "brand new password"

  Scenario: A reset request answers nothing about an unknown address
    When a reset is asked for "nobody@example.com"
    Then the request is answered
    And the relay holds no mail
