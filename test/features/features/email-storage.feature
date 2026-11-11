Feature: Stored mail keeps its files inside the workspace limit
  The raw message and each attachment of a stored mail go to the file
  store. A file over its cap, or one that would pass the workspace limit,
  keeps a named row with no file, and the text of the mail is always kept.
  The limit counts the text as well as the files. A workspace can hold a
  limit of its own in its plugin settings. A background sweep removes the
  files that no stored mail holds, such as those a stopped run left behind,
  in every workspace.

  Background:
    Given the database is migrated
    And the administrator "admin@example.com"
    And the workspace "Acme" exists
    And the workspace "Acme" knows "Maria Perez" at "maria.perez@example.com"

  Scenario: A raw message over the file cap keeps its attachments and text
    Given the setting "ALPHONE_STORAGE_MAX_BYTES" holds "200"
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Quote attached
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <quote-attached@example.com>
      MIME-Version: 1.0
      Content-Type: multipart/mixed; boundary="files"

      --files
      Content-Type: text/plain

      The quote is attached.

      --files
      Content-Type: application/pdf; name="quote.pdf"
      Content-Disposition: attachment; filename="quote.pdf"
      Content-Transfer-Encoding: base64

      JVBERi0xLjQKJcOkw7zDtsOfCg==

      --files--
      """
    Then the command answers "stored"
    And the mail "Quote attached" keeps no raw copy as it is too large
    And the mail "Quote attached" keeps the file "quote.pdf" as "application/pdf"
    And the mail "Quote attached" keeps the text "The quote is attached."

  Scenario: An attachment over the attachment cap keeps a named row with no file
    Given the setting "ALPHONE_EMAIL_ATTACHMENT_MAX_BYTES" holds "8"
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Quote attached
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <quote-attached@example.com>
      MIME-Version: 1.0
      Content-Type: multipart/mixed; boundary="files"

      --files
      Content-Type: text/plain

      The quote is attached.

      --files
      Content-Type: application/pdf; name="quote.pdf"
      Content-Disposition: attachment; filename="quote.pdf"
      Content-Transfer-Encoding: base64

      JVBERi0xLjQKJcOkw7zDtsOfCg==

      --files--
      """
    Then the command answers "stored"
    And the mail "Quote attached" names the file "quote.pdf" with no copy as it is too large
    And the mail "Quote attached" keeps its raw copy
    And the mail "Quote attached" keeps the text "The quote is attached."

  Scenario: A mail over the workspace limit keeps its text and skips its files, and each skipped file keeps its name
    Given the setting "ALPHONE_EMAIL_WORKSPACE_MAX_BYTES" holds "100"
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Quote attached
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <quote-attached@example.com>
      MIME-Version: 1.0
      Content-Type: multipart/mixed; boundary="files"

      --files
      Content-Type: text/plain

      The quote is attached.

      --files
      Content-Type: application/pdf; name="quote.pdf"
      Content-Disposition: attachment; filename="quote.pdf"
      Content-Transfer-Encoding: base64

      JVBERi0xLjQKJcOkw7zDtsOfCg==

      --files--
      """
    Then the command answers "stored"
    And the mail "Quote attached" keeps the text "The quote is attached."
    And the mail "Quote attached" keeps no raw copy as it is over the workspace limit
    And the mail "Quote attached" names the file "quote.pdf" with no copy as it is over the workspace limit
    And the workspace "Acme" keeps no file

  Scenario: The workspace limit counts text as well as files
    Given the setting "ALPHONE_EMAIL_WORKSPACE_MAX_BYTES" holds "1450"
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Lamp care guide
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <lamp-care-guide@example.com>
      Content-Type: text/plain

      Dust the brass lamp once a week with a dry cloth, and never use water
      on the shade. Turn the lamp off and let the bulb cool before you touch
      it. Keep the cord away from the radiator, and check the plug for cracks
      every season. A warm white bulb of eight watts suits the white shade,
      and a brighter bulb can brown the fabric over a long winter.
      """
    And the mail "Lamp care guide" keeps its raw copy
    And the usage of "Acme" counts its stored files and its stored text
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Lamp order
      Date: Mon, 02 Nov 2026 09:05:00 +0000
      Message-ID: <lamp-order@example.com>
      Content-Type: text/plain

      Please send one brass lamp with a white shade to the shop. The lamp
      should arrive before the end of the month, and the driver can take the
      old lamp back on the same trip. We will pay the invoice when the lamp
      arrives. If the brass finish is out of stock, a black finish will do,
      but please ask us first. The shop opens at nine on weekdays and at ten
      on Saturday.
      """
    Then the mail "Lamp order" keeps its whole text
    And the mail "Lamp order" keeps no raw copy as it is over the workspace limit

  Scenario: A workspace's own limit replaces the default
    Given the workspace "Globex" exists
    And the workspace "Globex" knows "Maria Perez" at "maria.perez@example.com"
    And the workspace "Acme" holds its own mail limit "100"
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Quote attached
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <quote-attached@example.com>
      MIME-Version: 1.0
      Content-Type: multipart/mixed; boundary="files"

      --files
      Content-Type: text/plain

      The quote is attached.

      --files
      Content-Type: application/pdf; name="quote.pdf"
      Content-Disposition: attachment; filename="quote.pdf"
      Content-Transfer-Encoding: base64

      JVBERi0xLjQKJcOkw7zDtsOfCg==

      --files--
      """
    And the same mail reaches "Globex"
    Then the workspace "Acme" holds 1 mail
    And the workspace "Acme" keeps no file
    And the workspace "Globex" holds 1 mail
    And the workspace "Globex" keeps 2 files

  Scenario: A workspace's own limit that does not read as a number keeps the default
    Given the workspace "Acme" holds its own mail limit "a lot"
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Quote attached
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <quote-attached@example.com>
      MIME-Version: 1.0
      Content-Type: multipart/mixed; boundary="files"

      --files
      Content-Type: text/plain

      The quote is attached.

      --files
      Content-Type: application/pdf; name="quote.pdf"
      Content-Disposition: attachment; filename="quote.pdf"
      Content-Transfer-Encoding: base64

      JVBERi0xLjQKJcOkw7zDtsOfCg==

      --files--
      """
    Then the command answers "stored"
    And the command warns about "workspace_max_bytes"
    And the workspace "Acme" keeps 2 files

  Scenario: A file delete in the sweep reaches the right workspace
    Given the workspace "Globex" exists
    And the workspace "Globex" knows "Maria Perez" at "maria.perez@example.com"
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Opening hours
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <opening-hours@example.com>
      Content-Type: text/plain

      When does the shop open on Saturday?
      """
    And an ingest of this mail into "Globex" stopped two days ago after it kept 2 files:
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Quote attached
      Date: Mon, 02 Nov 2026 09:05:00 +0000
      Message-ID: <quote-attached@example.com>
      MIME-Version: 1.0
      Content-Type: multipart/mixed; boundary="files"

      --files
      Content-Type: text/plain

      The quote is attached.

      --files
      Content-Type: application/pdf; name="quote.pdf"
      Content-Disposition: attachment; filename="quote.pdf"
      Content-Transfer-Encoding: base64

      JVBERi0xLjQKJcOkw7zDtsOfCg==

      --files--
      """
    When AlphOne serves with an email sweep every 2 seconds
    Then the file queue of "Globex" is empty after one sweep
    And the workspace "Globex" keeps no file
    And the workspace "Acme" keeps 1 file

  Scenario: Files left by a stopped ingest are swept
    Given an ingest of this mail into "Acme" stopped two days ago after it kept 2 files:
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Quote attached
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <quote-attached@example.com>
      MIME-Version: 1.0
      Content-Type: multipart/mixed; boundary="files"

      --files
      Content-Type: text/plain

      The quote is attached.

      --files
      Content-Type: application/pdf; name="quote.pdf"
      Content-Disposition: attachment; filename="quote.pdf"
      Content-Transfer-Encoding: base64

      JVBERi0xLjQKJcOkw7zDtsOfCg==

      --files--
      """
    When AlphOne serves with an email sweep every 2 seconds
    Then the file queue of "Acme" is empty after one sweep
    And the workspace "Acme" keeps no file

  Scenario: A queue larger than one batch drains in one sweep
    Given the setting "ALPHONE_EMAIL_SWEEP_BATCH" holds "2"
    And an ingest of this mail into "Acme" stopped two days ago after it kept 5 files:
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Site plans
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <site-plans@example.com>
      MIME-Version: 1.0
      Content-Type: multipart/mixed; boundary="files"

      --files
      Content-Type: text/plain

      The four plans of the site are attached.

      --files
      Content-Type: application/pdf; name="plan-1.pdf"
      Content-Disposition: attachment; filename="plan-1.pdf"
      Content-Transfer-Encoding: base64

      UGxhbiBvbmU=

      --files
      Content-Type: application/pdf; name="plan-2.pdf"
      Content-Disposition: attachment; filename="plan-2.pdf"
      Content-Transfer-Encoding: base64

      UGxhbiB0d28=

      --files
      Content-Type: application/pdf; name="plan-3.pdf"
      Content-Disposition: attachment; filename="plan-3.pdf"
      Content-Transfer-Encoding: base64

      UGxhbiB0aHJlZQ==

      --files
      Content-Type: application/pdf; name="plan-4.pdf"
      Content-Disposition: attachment; filename="plan-4.pdf"
      Content-Transfer-Encoding: base64

      UGxhbiBmb3Vy

      --files--
      """
    When AlphOne serves with an email sweep every 2 seconds
    Then the file queue of "Acme" is empty after one sweep
    And the workspace "Acme" keeps no file
