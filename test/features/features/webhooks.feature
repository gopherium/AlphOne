Feature: A webhook names an address AlphOne may reach
  A webhook carries workspace events to the address it names. An address
  written as an internal IP is refused when the webhook is created unless
  the operator allows it, and no name is looked up then. A host name is
  stored as written and checked again each time a delivery connects.

  Background:
    Given a running AlphOne holding a user with an API token
    And the user holds a token scoped to "webhooks:write"

  Scenario Outline: A webhook to an internal address is refused
    When that token registers a webhook to "<address>"
    Then the webhook is refused as an internal address
    And that token lists no webhook

    Examples:
      | address                        |
      | http://127.0.0.1:5678/hook     |
      | http://10.0.0.5/hook           |
      | http://192.168.1.20:8080/hook  |
      | http://169.254.169.254/latest  |
      | http://[::1]:5678/hook         |
      | http://[::ffff:127.0.0.1]/hook |
      | http://[fd00:ec2::254]/hook    |
      | http://168.63.129.16/machine   |

  Scenario: A webhook to a host name is registered without a lookup
    When that token registers a webhook to "https://example.com/hook"
    Then the webhook is registered for "https://example.com/hook"

  Scenario: A webhook to a public address is registered on any port
    When that token registers a webhook to "https://192.0.2.10:8443/hook"
    Then the webhook is registered for "https://192.0.2.10:8443/hook"

  Scenario: A webhook to an internal address the operator allowed is registered
    Given the operator allows webhook deliveries to "127.0.0.1/32"
    When that token registers a webhook to "http://127.0.0.1:5678/hook"
    Then the webhook is registered for "http://127.0.0.1:5678/hook"

  Scenario Outline: A metadata address stays refused whatever the operator allows
    Given the operator allows webhook deliveries to "<allowed>"
    When that token registers a webhook to "<address>"
    Then the webhook is refused as an internal address

    Examples:
      | allowed        | address                                  |
      | 169.254.0.0/16 | http://169.254.169.254/latest            |
      | 100.64.0.0/10  | http://100.100.100.200/latest            |
      | fc00::/7       | http://[fd20:ce::254]/computeMetadata/v1 |

  Scenario: A neighbour of a metadata address is registered once its range is allowed
    Given the operator allows webhook deliveries to "100.64.0.0/10"
    When that token registers a webhook to "http://100.100.100.201/hook"
    Then the webhook is registered for "http://100.100.100.201/hook"
