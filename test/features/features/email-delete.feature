Feature: Staff delete stored mail and free the space a workspace uses
  An admin deletes one mail, a whole thread, an attachment or the original
  of a mail through the API with a token that holds the email area for
  writing. A member cannot. Mail fed with email:ingest or stored by the
  seed deletes at once. A thread left with no mail goes, and a thread that
  keeps mail takes its subject and its times from the mail left. A deleted
  mail fed again is not stored again. A mail whose files were deleted still
  names each file it carried. Text frees its space at once, and a file
  frees its space once the sweep removes it. Every member reads how much
  space the workspace uses, its limit, the space still being freed and the
  mails that cost it most, each with its files.

  Background:
    Given the database is migrated
    And the administrator "admin@example.com"
    And the member "member@example.com"
    And the workspace "Acme" exists
    And the account "admin@example.com" is placed in the workspace "Acme"
    And the account "member@example.com" is placed in the workspace "Acme"
    And the workspace "Acme" knows "Maria Perez" at "maria.perez@example.com"

  Scenario: An admin deletes one freshly fed mail at once
    Given the next mail is received just now
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
    And the administrator "admin@example.com" holds a token scoped to "email:read email:write"
    And the member "member@example.com" holds a token scoped to "email:read"
    When the administrator "admin@example.com" deletes the mail "Opening hours"
    Then the delete answers 1 mail deleted and the thread deleted
    And the workspace "Acme" holds 0 mails
    When the member "member@example.com" reads the inbox
    Then the inbox lists no thread

  Scenario: An admin deletes one seeded mail at once
    Given the administrator "default.admin@example.com"
    And the member "default.member@example.com"
    And the sample mailbox is seeded in the default workspace
    And the administrator "default.admin@example.com" holds a token scoped to "email:read email:write"
    And the member "default.member@example.com" holds a token scoped to "email:read"
    When the administrator "default.admin@example.com" deletes the mail "Site visit on Friday"
    Then the delete answers 1 mail deleted and the thread deleted
    When the member "default.member@example.com" reads the inbox
    Then the inbox lists the threads "Delivery date, Quote for the new lamp" in that order

  Scenario: Deleting the last mail of a thread removes the thread
    Given this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Lamp quote
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <lamp-quote@example.com>
      Content-Type: text/plain

      Could you send me a quote for the new lamp?
      """
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Re: Lamp quote
      Date: Mon, 02 Nov 2026 09:05:00 +0000
      Message-ID: <lamp-quote-again@example.com>
      In-Reply-To: <lamp-quote@example.com>
      References: <lamp-quote@example.com>
      Content-Type: text/plain

      Could the lamp come in brass?
      """
    And the administrator "admin@example.com" holds a token scoped to "email:read email:write"
    And the member "member@example.com" holds a token scoped to "email:read"
    And the member "member@example.com" shares the thread "Lamp quote"
    When the administrator "admin@example.com" deletes the mail "Re: Lamp quote"
    Then the delete answers 1 mail deleted and the thread kept
    When the administrator "admin@example.com" deletes the mail "Lamp quote"
    Then the delete answers 1 mail deleted and the thread deleted
    When the member "member@example.com" opens the shared thread
    Then no thread is found

  Scenario: A thread that keeps mail takes its subject and times from the mail left
    Given this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Lamp quote
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <lamp-quote@example.com>
      Content-Type: text/plain

      Could you send me a quote for the new lamp?
      """
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Lamp colours
      Date: Mon, 02 Nov 2026 09:05:00 +0000
      Message-ID: <lamp-colours@example.com>
      In-Reply-To: <lamp-quote@example.com>
      References: <lamp-quote@example.com>
      Content-Type: text/plain

      Does the lamp come in brass or in black?
      """
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Lamp delivery
      Date: Mon, 02 Nov 2026 09:10:00 +0000
      Message-ID: <lamp-delivery@example.com>
      In-Reply-To: <lamp-colours@example.com>
      References: <lamp-quote@example.com> <lamp-colours@example.com>
      Content-Type: text/plain

      When will the brass lamp arrive?
      """
    And the administrator "admin@example.com" holds a token scoped to "email:read email:write"
    And the member "member@example.com" holds a token scoped to "email:read"
    And the administrator "admin@example.com" deleted the mail "Lamp quote"
    And the administrator "admin@example.com" deleted the mail "Lamp delivery"
    When the member "member@example.com" reads the inbox
    Then the inbox lists the threads "Lamp colours" in that order
    When the member "member@example.com" opens the thread "Lamp colours" with its times
    Then the thread starts and ends when its mail "Lamp colours" was received

  Scenario: An admin deletes a whole seeded thread at once
    Given the administrator "default.admin@example.com"
    And the member "default.member@example.com"
    And the sample mailbox is seeded in the default workspace
    And the administrator "default.admin@example.com" holds a token scoped to "email:read email:write"
    And the member "default.member@example.com" holds a token scoped to "email:read"
    When the administrator "default.admin@example.com" deletes the thread "Quote for the new lamp"
    Then the delete answers 3 mails deleted and the thread deleted
    When the member "default.member@example.com" reads the inbox
    Then the inbox lists the threads "Delivery date, Site visit on Friday" in that order
    When the member "default.member@example.com" reads the usage
    Then the usage counts the bytes of every stored mail and frees each deleted file once

  Scenario: A member cannot delete mail
    Given this mail reached "Acme":
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
    And the member "member@example.com" holds a token scoped to "email:read email:write"
    When the member "member@example.com" deletes the mail "Quote attached"
    Then the delete is rejected for want of the capability "email.delete_messages"
    When the member "member@example.com" deletes the file "quote.pdf" of the mail "Quote attached"
    Then the delete is rejected for want of the capability "email.delete_files"
    And the workspace "Acme" holds 1 mail
    When the member "member@example.com" reads the mail "Quote attached"
    Then the mail offers the file "quote.pdf" for download

  Scenario: A token that only reads cannot delete mail
    Given this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Opening hours
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <opening-hours@example.com>
      Content-Type: text/plain

      When does the shop open on Saturday?
      """
    And the administrator "admin@example.com" holds a token scoped to "email:read"
    When the administrator "admin@example.com" deletes the mail "Opening hours"
    Then the delete is rejected for want of the scope "email:write"
    And the workspace "Acme" holds 1 mail

  Scenario: A deactivated workspace cannot delete mail
    Given this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Opening hours
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <opening-hours@example.com>
      Content-Type: text/plain

      When does the shop open on Saturday?
      """
    And the administrator "admin@example.com" holds a token scoped to "email:read email:write"
    And the workspace "Acme" was deactivated a day ago
    When the administrator "admin@example.com" deletes the mail "Opening hours"
    Then the delete is rejected as the workspace is deactivated
    And the workspace "Acme" holds 1 mail

  Scenario: A mail of another workspace is not found
    Given the workspace "Globex" exists
    And the workspace "Globex" knows "Maria Perez" at "maria.perez@example.com"
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Lamp quote
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <lamp-quote@example.com>
      Content-Type: text/plain

      Could you send me a quote for the new lamp?
      """
    And the same mail reaches "Globex"
    And the administrator "other.admin@example.com"
    And the account "other.admin@example.com" is placed in the workspace "Globex"
    And the member "member@example.com" holds a token scoped to "email:read"
    And the administrator "other.admin@example.com" holds a token scoped to "email:read email:write"
    And the member "member@example.com" shares the mail "Lamp quote"
    When the administrator "other.admin@example.com" deletes the shared mail
    Then the delete is rejected with the reason "thread_not_found"
    When the administrator "other.admin@example.com" deletes the original of the shared mail
    Then the delete is rejected with the reason "message_not_found"
    When the member "member@example.com" reads the mail "Lamp quote"
    Then the mail offers its original

  Scenario: A deleted mail fed again answers already_deleted and stores nothing
    Given this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Opening hours
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <opening-hours@example.com>
      Content-Type: text/plain

      When does the shop open on Saturday?
      """
    And the administrator "admin@example.com" holds a token scoped to "email:read email:write"
    And the member "member@example.com" holds a token scoped to "email:read"
    And the administrator "admin@example.com" deleted the mail "Opening hours"
    When the same mail reaches "Acme" again
    Then the command answers "already_deleted"
    And the workspace "Acme" holds 0 mails
    And the workspace "Acme" keeps no file but the ones waiting for the sweep
    When the member "member@example.com" reads the inbox
    Then the inbox lists no thread

  Scenario: A deleted mail previewed again answers already_deleted
    Given this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Opening hours
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <opening-hours@example.com>
      Content-Type: text/plain

      When does the shop open on Saturday?
      """
    And the administrator "admin@example.com" holds a token scoped to "email:read email:write"
    And the administrator "admin@example.com" deleted the mail "Opening hours"
    When the same mail is previewed for "Acme"
    Then the command answers "already_deleted"
    And the answer says nothing changed until it is confirmed with "-yes"
    And the workspace "Acme" holds 0 mails

  Scenario: A deleted mail fed again as a Sent copy answers already_deleted
    Given this mail reached "Acme":
      """
      From: Info <info@example.com>
      To: Maria Perez <maria.perez@example.com>
      Cc: Info <info@example.com>
      Subject: Your quote
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <your-quote@example.com>
      Content-Type: text/plain

      The quote for the new lamp is ready.
      """
    And the administrator "admin@example.com" holds a token scoped to "email:read email:write"
    And the administrator "admin@example.com" deleted the mail "Your quote"
    When the same mail reaches "Acme" as a Sent copy
    Then the command answers "already_deleted"
    And the workspace "Acme" holds 0 mails

  Scenario: The files of a deleted mail go at the next sweep and the workspace frees their bytes
    Given this mail reached "Acme":
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
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Opening hours
      Date: Mon, 02 Nov 2026 09:05:00 +0000
      Message-ID: <opening-hours@example.com>
      Content-Type: text/plain

      When does the shop open on Saturday?
      """
    And the administrator "admin@example.com" holds a token scoped to "email:read email:write"
    And the member "member@example.com" holds a token scoped to "email:read"
    When AlphOne serves with an email sweep every 10 seconds
    And the administrator "admin@example.com" deletes the mail "Quote attached"
    Then the delete answers 1 mail deleted and the thread deleted
    And the file queue of "Acme" is empty after one sweep
    And the workspace "Acme" keeps 1 file
    When the member "member@example.com" reads the usage
    Then the usage counts the bytes of every stored mail and frees nothing

  Scenario: An admin deletes one attachment and the mail keeps its text and the attachment's name
    Given this mail reached "Acme":
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
    And the administrator "admin@example.com" holds a token scoped to "email:read email:write"
    And the member "member@example.com" holds a token scoped to "email:read"
    When the administrator "admin@example.com" deletes the file "quote.pdf" of the mail "Quote attached"
    Then the delete answers the mail with the file "quote.pdf" deleted
    When the member "member@example.com" reads what is kept of the mail "Quote attached"
    Then the mail reads the text "The quote is attached."
    And the mail names the file "quote.pdf" as "application/pdf" of its old size, deleted with no download
    And the mail offers its original

  Scenario: Deleting an attachment twice changes nothing
    Given this mail reached "Acme":
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
    And the administrator "admin@example.com" holds a token scoped to "email:read email:write"
    And the member "member@example.com" holds a token scoped to "email:read"
    And the administrator "admin@example.com" deleted the file "quote.pdf" of the mail "Quote attached"
    When the administrator "admin@example.com" deletes the file "quote.pdf" of the mail "Quote attached"
    Then the delete answers the mail with the file "quote.pdf" deleted
    When the member "member@example.com" reads the usage
    Then the usage counts the bytes of every stored mail and frees each deleted file once

  Scenario: An admin deletes the original of a mail and its text and attachments stay
    Given this mail reached "Acme":
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
    And the administrator "admin@example.com" holds a token scoped to "email:read email:write"
    And the member "member@example.com" holds a token scoped to "email:read"
    When the administrator "admin@example.com" deletes the original of the mail "Quote attached"
    Then the delete answers the mail with its original deleted
    When the member "member@example.com" reads what is kept of the mail "Quote attached"
    Then the mail reads the text "The quote is attached."
    And the mail offers the file "quote.pdf" for download
    And the mail names its original of its old size as deleted with no download

  Scenario: A deleted attachment and a deleted original no longer download
    Given this mail reached "Acme":
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
    And the administrator "admin@example.com" holds a token scoped to "email:read email:write"
    And the member "member@example.com" holds a token scoped to "email:read"
    And the member "member@example.com" shares the file "quote.pdf" of the mail "Quote attached"
    When the administrator "admin@example.com" deletes both "quote.pdf" and the original of the mail "Quote attached"
    Then the delete answers the mail with the file "quote.pdf" deleted
    And the delete answers the mail with its original deleted
    When the member "member@example.com" downloads the shared file
    Then the download answers not found
    When the member "member@example.com" downloads the original of the mail "Quote attached"
    Then the download answers not found

  Scenario: Deleting an attachment and then its whole mail frees each byte once
    Given this mail reached "Acme":
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
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Opening hours
      Date: Mon, 02 Nov 2026 09:05:00 +0000
      Message-ID: <opening-hours@example.com>
      Content-Type: text/plain

      When does the shop open on Saturday?
      """
    And the administrator "admin@example.com" holds a token scoped to "email:read email:write"
    And the member "member@example.com" holds a token scoped to "email:read"
    And the administrator "admin@example.com" deleted the file "quote.pdf" of the mail "Quote attached"
    When the administrator "admin@example.com" deletes the mail "Quote attached"
    Then the delete answers 1 mail deleted and the thread deleted
    When the member "member@example.com" reads the usage
    Then the usage counts the bytes of every stored mail and frees each deleted file once

  Scenario: The same mail fed again after "Delete attachment" and "Delete original message" adds no file and no bytes
    Given this mail reached "Acme":
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
    And the administrator "admin@example.com" holds a token scoped to "email:read email:write"
    And the member "member@example.com" holds a token scoped to "email:read"
    And the administrator "admin@example.com" deleted the file "quote.pdf" of the mail "Quote attached"
    And the administrator "admin@example.com" deleted the original of the mail "Quote attached"
    And the member "member@example.com" reads the usage
    When the same mail reaches "Acme" again
    Then the command answers "duplicate"
    And the workspace "Acme" holds 1 mail
    And the workspace "Acme" keeps no file but the ones waiting for the sweep
    When the member "member@example.com" reads the usage
    Then the usage holds as many bytes as before

  Scenario: A files delete that names nothing to delete is rejected
    Given this mail reached "Acme":
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
    And the administrator "admin@example.com" holds a token scoped to "email:read email:write"
    When the administrator "admin@example.com" deletes the files of the mail "Quote attached" naming none
    Then the delete is rejected with the reason "nothing_to_delete"

  Scenario: Two files of one mail deleted in one request both answer as deleted
    Given this mail reached "Acme":
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

      The two plans of the site are attached.

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

      --files--
      """
    And the administrator "admin@example.com" holds a token scoped to "email:read email:write"
    When the administrator "admin@example.com" deletes both "plan-1.pdf" and "plan-2.pdf" of "Site plans" in one request
    Then the last answer of the request names the files "plan-1.pdf, plan-2.pdf" as deleted

  Scenario: The usage answers the bytes a workspace holds and its limit
    Given the setting "ALPHONE_EMAIL_WORKSPACE_MAX_BYTES" holds "1000000"
    And this mail reached "Acme":
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
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Opening hours
      Date: Mon, 02 Nov 2026 09:05:00 +0000
      Message-ID: <opening-hours@example.com>
      Content-Type: text/plain

      When does the shop open on Saturday?
      """
    And the member "member@example.com" holds a token scoped to "email:read"
    When the member "member@example.com" reads the usage
    Then the usage counts the bytes of every stored mail and frees nothing
    And the usage answers a limit of 1000000 bytes

  Scenario: The usage counts deleted files as freeing until the sweep runs
    Given this mail reached "Acme":
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
    And the administrator "admin@example.com" holds a token scoped to "email:read email:write"
    And the member "member@example.com" holds a token scoped to "email:read"
    When AlphOne serves with an email sweep every 10 seconds
    And the administrator "admin@example.com" deletes the file "quote.pdf" of the mail "Quote attached"
    And the member "member@example.com" reads the usage
    Then the usage counts the bytes of every stored mail and frees each deleted file once
    And the file queue of "Acme" is empty after one sweep
    When the member "member@example.com" reads the usage
    Then the usage counts the bytes of every stored mail and frees nothing

  Scenario: The usage lists the costliest mails first with their files
    Given this mail reached "Acme":
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
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Opening hours
      Date: Mon, 02 Nov 2026 09:05:00 +0000
      Message-ID: <opening-hours@example.com>
      Content-Type: text/plain

      When does the shop open on Saturday?
      """
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Quote attached
      Date: Mon, 02 Nov 2026 09:10:00 +0000
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
    And the member "member@example.com" holds a token scoped to "email:read"
    When the member "member@example.com" reads the usage
    Then the usage lists the mails "Site plans, Quote attached, Opening hours" costliest first
    And the usage lists the mail "Site plans" with the files "plan-1.pdf, plan-2.pdf, plan-3.pdf, plan-4.pdf"
    And the usage lists the mail "Quote attached" with the files "quote.pdf"
    When the member "member@example.com" reads the usage with its 2 costliest mails
    Then the usage lists the mails "Site plans, Quote attached" costliest first

  Scenario: A workspace's own limit shows in the usage
    Given the setting "ALPHONE_EMAIL_WORKSPACE_MAX_BYTES" holds "0"
    And the workspace "Globex" exists
    And the member "other.member@example.com"
    And the account "other.member@example.com" is placed in the workspace "Globex"
    And the workspace "Acme" holds its own mail limit "2000000"
    And the member "member@example.com" holds a token scoped to "email:read"
    And the member "other.member@example.com" holds a token scoped to "email:read"
    When the member "member@example.com" reads the usage
    Then the usage answers a limit of 2000000 bytes
    When the member "other.member@example.com" reads the usage
    Then the usage answers no limit
