Feature: Members read stored mail and download its files through the API
  Every member of a workspace reads its mail through the API with a token
  that holds the email area. The mailbox answers its address and aliases.
  The inbox lists the threads by their newest mail, with a total and pages,
  and a thread lists its mail newest first. A mail answers its text, its
  safe HTML and what was kept of it. A contact shows only the mail linked
  to that contact. An attachment and the original mail download under a
  safe name and never run as a page. A thread or a file of another
  workspace is never found, and one request reads only a few mail bodies
  and a bounded number of people and file lists. A request that asks for
  one field under two names is rejected.

  Background:
    Given the database is migrated
    And the administrator "admin@example.com"
    And the member "member@example.com"
    And the workspace "Acme" exists
    And the account "member@example.com" is placed in the workspace "Acme"
    And the workspace "Acme" knows "Maria Perez" at "maria.perez@example.com"

  Scenario: A token holding the email area is minted
    When the operator mints a token for "member@example.com" scoped to "email:read"
    Then the command exits with code 0
    And the token list of "member@example.com" shows a token scoped to "email:read"

  Scenario: A token without the email area cannot read mail
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
    And the member "member@example.com" holds a token scoped to "contacts:read"
    When the member "member@example.com" reads the inbox
    Then the read is rejected for want of the scope "email:read"

  Scenario: A workspace with no mailbox answers no mailbox
    Given the member "member@example.com" holds a token scoped to "email:read"
    When the member "member@example.com" reads the mailbox
    Then the workspace answers no mailbox
    And the workspace "Acme" holds no mailbox

  Scenario: The mailbox answers its address and aliases
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
    And the member "member@example.com" holds a token scoped to "email:read"
    When the member "member@example.com" reads the mailbox
    Then the mailbox answers the address "info@example.com" with the alias "sales@example.com"

  Scenario: The inbox lists the threads by their newest mail first
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
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Lamp quote
      Date: Mon, 02 Nov 2026 09:05:00 +0000
      Message-ID: <lamp-quote@example.com>
      Content-Type: text/plain

      Could you send me a quote for the new lamp?
      """
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Delivery date
      Date: Mon, 02 Nov 2026 09:10:00 +0000
      Message-ID: <delivery-date@example.com>
      Content-Type: text/plain

      When will the lamp arrive?
      """
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Re: Lamp quote
      Date: Mon, 02 Nov 2026 09:15:00 +0000
      Message-ID: <lamp-quote-again@example.com>
      In-Reply-To: <lamp-quote@example.com>
      References: <lamp-quote@example.com>
      Content-Type: text/plain

      Could the lamp come in brass?
      """
    And the member "member@example.com" holds a token scoped to "email:read"
    When the member "member@example.com" reads the inbox
    Then the inbox lists the threads "Lamp quote, Delivery date, Opening hours" in that order

  Scenario: The inbox answers its total and pages by limit and offset
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
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Lamp quote
      Date: Mon, 02 Nov 2026 09:05:00 +0000
      Message-ID: <lamp-quote@example.com>
      Content-Type: text/plain

      Could you send me a quote for the new lamp?
      """
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Delivery date
      Date: Mon, 02 Nov 2026 09:10:00 +0000
      Message-ID: <delivery-date@example.com>
      Content-Type: text/plain

      When will the lamp arrive?
      """
    And the member "member@example.com" holds a token scoped to "email:read"
    When the member "member@example.com" reads 2 threads of the inbox after skipping 1
    Then the inbox lists the threads "Lamp quote, Opening hours" in that order
    And the inbox answers a total of 3 threads on pages of 2

  Scenario: An inbox page past the cap is rejected
    Given the setting "ALPHONE_GRAPH_PAGE_CAP" holds "100"
    And the member "member@example.com" holds a token scoped to "email:read"
    When the member "member@example.com" reads 101 threads of the inbox after skipping 0
    Then the read is rejected with the reason "first_out_of_range"

  Scenario: Each inbox thread names its newest mail, its count of mail and whether it holds files
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
    And this Sent copy reached "Acme":
      """
      From: Info <info@example.com>
      To: Maria Perez <maria.perez@example.com>
      Subject: Re: Quote attached
      Date: Mon, 02 Nov 2026 09:05:00 +0000
      Message-ID: <quote-attached-reply@example.com>
      In-Reply-To: <quote-attached@example.com>
      References: <quote-attached@example.com>
      Content-Type: text/plain

      Thank you, the lamp leaves next week.
      """
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Lamp photo
      Date: Mon, 02 Nov 2026 09:10:00 +0000
      Message-ID: <lamp-photo@example.com>
      MIME-Version: 1.0
      Content-Type: multipart/related; boundary="photo"

      --photo
      Content-Type: text/html

      <p>Is it this model?</p><p><img src="cid:lamp-photo@example.com" alt="The brass lamp"></p>

      --photo
      Content-Type: image/png
      Content-ID: <lamp-photo@example.com>
      Content-Disposition: inline; filename="lamp.png"
      Content-Transfer-Encoding: base64

      iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==

      --photo--
      """
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Opening hours
      Date: Mon, 02 Nov 2026 09:15:00 +0000
      Message-ID: <opening-hours@example.com>
      Content-Type: text/plain

      When does the shop open on Saturday?
      """
    And the member "member@example.com" holds a token scoped to "email:read"
    When the member "member@example.com" reads the inbox
    Then the inbox thread "Quote attached" holds 2 mails and files
    And the newest mail of the inbox thread "Quote attached" is "Re: Quote attached" from "info@example.com"
    And the inbox thread "Lamp photo" holds 1 mail and no files
    And the inbox thread "Opening hours" holds 1 mail and no files

  Scenario: A thread lists its mail newest first and pages the older ones
    Given this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Site visit
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <site-visit@example.com>
      Content-Type: text/plain

      Could we visit the site on Friday?
      """
    And this Sent copy reached "Acme":
      """
      From: Info <info@example.com>
      To: Maria Perez <maria.perez@example.com>
      Subject: Visit time
      Date: Mon, 02 Nov 2026 09:05:00 +0000
      Message-ID: <visit-time@example.com>
      In-Reply-To: <site-visit@example.com>
      References: <site-visit@example.com>
      Content-Type: text/plain

      Could we come at ten in the morning?
      """
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Visit confirmed
      Date: Mon, 02 Nov 2026 09:10:00 +0000
      Message-ID: <visit-confirmed@example.com>
      In-Reply-To: <visit-time@example.com>
      References: <site-visit@example.com> <visit-time@example.com>
      Content-Type: text/plain

      Ten in the morning is fine.
      """
    And the member "member@example.com" holds a token scoped to "email:read"
    When the member "member@example.com" opens the thread "Site visit"
    Then the thread lists the mails "Visit confirmed, Visit time, Site visit" in that order
    When the member "member@example.com" reads 2 mails of the thread "Site visit" after skipping 2
    Then the thread lists the mails "Site visit" in that order

  Scenario: A member reads the text and the HTML of one mail
    Given this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Lamp catalogue
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <lamp-catalogue@example.com>
      MIME-Version: 1.0
      Content-Type: multipart/alternative; boundary="views"

      --views
      Content-Type: text/plain

      The new lamps are in the catalogue.

      --views
      Content-Type: text/html

      <p>The new lamps are in the catalogue.</p><p><img src="https://example.com/lamp.png" alt="Brass lamp"></p><script>alert("lamp")</script>

      --views--
      """
    And the member "member@example.com" holds a token scoped to "email:read"
    When the member "member@example.com" reads the mail "Lamp catalogue"
    Then the mail reads the text "The new lamps are in the catalogue."
    And the HTML of the mail keeps the remote image "https://example.com/lamp.png"
    And the HTML of the mail holds no script

  Scenario: A request that asks for more bodies than the bound is rejected
    Given the setting "ALPHONE_EMAIL_BODIES_PER_REQUEST" holds "1"
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
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Lamp quote
      Date: Mon, 02 Nov 2026 09:05:00 +0000
      Message-ID: <lamp-quote@example.com>
      Content-Type: text/plain

      Could you send me a quote for the new lamp?
      """
    And the member "member@example.com" holds a token scoped to "email:read"
    When the member "member@example.com" reads the text of the newest mail of every inbox thread in one request
    Then the read is rejected with the reason "bodies_per_request_exceeded"
    When the member "member@example.com" reads the mail "Lamp quote"
    Then the mail reads the text "Could you send me a quote for the new lamp?"

  Scenario: A request that asks for more people lists than the bound is rejected
    Given the setting "ALPHONE_EMAIL_LISTS_PER_REQUEST" holds "1"
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
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Lamp quote
      Date: Mon, 02 Nov 2026 09:05:00 +0000
      Message-ID: <lamp-quote@example.com>
      Content-Type: text/plain

      Could you send me a quote for the new lamp?
      """
    And the member "member@example.com" holds a token scoped to "email:read"
    When the member "member@example.com" reads the people of the newest mail of every inbox thread in one request
    Then the read is rejected with the reason "lists_per_request_exceeded"
    When the member "member@example.com" reads the people of the mail "Lamp quote"
    Then the mail names "Maria Perez" at "maria.perez@example.com" as its sender

  Scenario: A request that reads the text of one mail under two names is rejected
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
    And the member "member@example.com" holds a token scoped to "email:read"
    When the member "member@example.com" reads the text of the mail "Lamp quote" under two names in one request
    Then the read is rejected with the reason "repeated_field"
    When the member "member@example.com" reads the mail "Lamp quote"
    Then the mail reads the text "Could you send me a quote for the new lamp?"

  Scenario: A mail with two known contacts in Cc shows on both contacts
    Given the workspace "Acme" knows "Ada Lovelace" at "ada@example.com"
    And this mail reached "Acme":
      """
      From: unknown.sender@example.com
      To: info@example.com
      Cc: Maria Perez <maria.perez@example.com>, Ada Lovelace <ada@example.com>
      Subject: Site visit on Friday
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <site-visit-friday@example.com>
      Content-Type: text/plain

      We will both be at the site on Friday.
      """
    And the member "member@example.com" holds a token scoped to "contacts:read email:read"
    When the member "member@example.com" reads the mail of the contact "Maria Perez"
    Then the contact's mail lists "Site visit on Friday" with a total of 1
    When the member "member@example.com" reads the mail of the contact "Ada Lovelace"
    Then the contact's mail lists "Site visit on Friday" with a total of 1

  Scenario: A contact's mail lists only the mail linked to that contact
    Given the workspace "Acme" knows "Ada Lovelace" at "ada@example.com"
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
    And this mail reached "Acme":
      """
      From: Ada Lovelace <ada@example.com>
      To: info@example.com
      Subject: Delivery date
      Date: Mon, 02 Nov 2026 09:05:00 +0000
      Message-ID: <delivery-date@example.com>
      Content-Type: text/plain

      When will my lamp arrive?
      """
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Opening hours
      Date: Mon, 02 Nov 2026 09:10:00 +0000
      Message-ID: <opening-hours@example.com>
      Content-Type: text/plain

      When does the shop open on Saturday?
      """
    And the member "member@example.com" holds a token scoped to "contacts:read email:read"
    When the member "member@example.com" reads the mail of the contact "Maria Perez"
    Then the contact's mail lists "Opening hours, Lamp quote" with a total of 2

  Scenario: A contact's mail never counts the mail of another contact in the same thread
    Given the workspace "Acme" knows "Ada Lovelace" at "ada@example.com"
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Site visit
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <site-visit@example.com>
      Content-Type: text/plain

      Could we visit the site on Friday?
      """
    And this mail reached "Acme":
      """
      From: Ada Lovelace <ada@example.com>
      To: info@example.com
      Cc: Maria Perez <maria.perez@example.com>
      Subject: Re: Site visit
      Date: Mon, 02 Nov 2026 09:05:00 +0000
      Message-ID: <site-visit-reply@example.com>
      In-Reply-To: <site-visit@example.com>
      References: <site-visit@example.com>
      Content-Type: text/plain

      I will come to the site as well.
      """
    And this mail reached "Acme":
      """
      From: Ada Lovelace <ada@example.com>
      To: info@example.com
      Subject: Parking at the site
      Date: Mon, 02 Nov 2026 09:10:00 +0000
      Message-ID: <parking-at-the-site@example.com>
      In-Reply-To: <site-visit-reply@example.com>
      References: <site-visit@example.com> <site-visit-reply@example.com>
      Content-Type: text/plain

      Where can I park my van?
      """
    And the member "member@example.com" holds a token scoped to "contacts:read email:read"
    When the member "member@example.com" reads the inbox
    Then the inbox thread "Site visit" holds 3 mails and no files
    When the member "member@example.com" reads the mail of the contact "Maria Perez"
    Then the contact's mail lists "Re: Site visit, Site visit" with a total of 2

  Scenario: Another workspace's thread is not found
    Given the workspace "Globex" exists
    And the workspace "Globex" knows "Maria Perez" at "maria.perez@example.com"
    And the member "other.member@example.com"
    And the account "other.member@example.com" is placed in the workspace "Globex"
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
    And the member "member@example.com" holds a token scoped to "email:read"
    And the member "other.member@example.com" holds a token scoped to "email:read"
    And the member "member@example.com" shares the thread "Lamp quote"
    When the member "other.member@example.com" opens the shared thread
    Then no thread is found

  Scenario: A thread a merge removed is not found by its old id
    Given this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Re: Visit plan
      Date: Mon, 02 Nov 2026 10:00:00 +0000
      Message-ID: <visit-plan-reply@example.com>
      In-Reply-To: <visit-plan@example.com>
      References: <visit-plan@example.com>
      Content-Type: text/plain

      The plan works for us.
      """
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Re: Visit time
      Date: Mon, 02 Nov 2026 10:05:00 +0000
      Message-ID: <visit-time-reply@example.com>
      In-Reply-To: <visit-time@example.com>
      References: <visit-time@example.com>
      Content-Type: text/plain

      Ten in the morning is fine.
      """
    And the member "member@example.com" holds a token scoped to "email:read"
    And the member "member@example.com" shares the thread "Re: Visit time"
    When this Sent copy reaches "Acme":
      """
      From: Info <info@example.com>
      To: Maria Perez <maria.perez@example.com>
      Subject: Visit time
      Date: Mon, 02 Nov 2026 09:30:00 +0000
      Message-ID: <visit-time@example.com>
      In-Reply-To: <visit-plan@example.com>
      References: <visit-plan@example.com>
      Content-Type: text/plain

      Could we come at ten in the morning?
      """
    And the member "member@example.com" opens the shared thread
    Then no thread is found
    When the member "member@example.com" opens the thread "Re: Visit plan"
    Then the thread lists the mails "Visit time, Re: Visit time, Re: Visit plan" in that order

  Scenario: A cut mail answers its state and still offers its original
    Given the setting "ALPHONE_EMAIL_BODY_MAX_BYTES" holds "16"
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Quote for the new lamp
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <lamp-quote-request@example.com>
      Content-Type: text/plain

      Could you send me a quote for the new lamp?
      """
    And the member "member@example.com" holds a token scoped to "email:read"
    When the member "member@example.com" reads the mail "Quote for the new lamp"
    Then the mail answers its body as "cut"
    And the mail reads the text "Could you send m"
    And the mail offers its original

  Scenario: A mail read by its headers alone answers its state and an empty preview
    Given this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Broken mail
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <broken-mail@example.com>
      Content-Type: multipart/mixed

      The boundary is missing.
      """
    And the member "member@example.com" holds a token scoped to "email:read"
    When the member "member@example.com" reads the mail "Broken mail"
    Then the mail answers its body as "headers_only"
    And the mail answers an empty preview
    And the mail offers its original

  Scenario: A mail with an unreadable part answers how many parts it left out
    Given this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Two files
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <unreadable-part@example.com>
      MIME-Version: 1.0
      Content-Type: multipart/mixed; boundary="files"

      --files
      Content-Type: text/plain

      One file reads and one does not.

      --files
      Content-Type: application/x-foo bar

      Lost.
      --files
      Content-Type: application/pdf; name="quote.pdf"
      Content-Disposition: attachment; filename="quote.pdf"
      Content-Transfer-Encoding: base64

      JVBERi0xLjQKJcOkw7zDtsOfCg==

      --files--
      """
    And the member "member@example.com" holds a token scoped to "email:read"
    When the member "member@example.com" reads the mail "Two files"
    Then the mail answers 1 part left out
    And the mail offers the file "quote.pdf" for download

  Scenario: An attachment downloads with nosniff, sandbox and no store
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
    And the member "member@example.com" holds a token scoped to "email:read"
    When the member "member@example.com" downloads the file "quote.pdf" of the mail "Quote attached"
    Then the download answers the type "application/pdf"
    And the download is saved as an attachment named "quote.pdf"
    And the download forbids sniffing, runs in a sandbox and is never stored
    And the download starts with "%PDF-1.4"

  Scenario: An inline image downloads inline
    Given this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Lamp photo
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <lamp-photo@example.com>
      MIME-Version: 1.0
      Content-Type: multipart/related; boundary="photo"

      --photo
      Content-Type: text/html

      <p>Is it this model?</p><p><img src="cid:lamp-photo@example.com" alt="The brass lamp"></p>

      --photo
      Content-Type: image/png
      Content-ID: <lamp-photo@example.com>
      Content-Disposition: inline; filename="lamp.png"
      Content-Transfer-Encoding: base64

      iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==

      --photo--
      """
    And the member "member@example.com" holds a token scoped to "email:read"
    When the member "member@example.com" downloads the file "lamp.png" of the mail "Lamp photo"
    Then the download answers the type "image/png"
    And the download is shown inline

  Scenario: An attachment whose name holds a path and control characters downloads under a safe name
    Given this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Odd file name
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <odd-file-name@example.com>
      MIME-Version: 1.0
      Content-Type: multipart/mixed; boundary="files"

      --files
      Content-Type: text/plain

      The quote is attached.

      --files
      Content-Type: application/pdf
      Content-Disposition: attachment; filename*=utf-8''..%2F..%2Fquote%E2%80%AE%09.pdf
      Content-Transfer-Encoding: base64

      JVBERi0xLjQKJcOkw7zDtsOfCg==

      --files--
      """
    And the member "member@example.com" holds a token scoped to "email:read"
    When the member "member@example.com" downloads the only file of the mail "Odd file name"
    Then the download answers the type "application/pdf"
    And the download is saved as an attachment named "quote.pdf"

  Scenario: The original downloads as a whole mail under a name without its subject
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
    And the member "member@example.com" holds a token scoped to "email:read"
    When the member "member@example.com" downloads the original of the mail "Quote attached"
    Then the download answers the type "message/rfc822"
    And the download is saved as an original named after the id of the mail "Quote attached"
    And the download holds the mail as it reached AlphOne

  Scenario: A file that was not kept offers no download and answers not found
    Given the setting "ALPHONE_EMAIL_ATTACHMENT_MAX_BYTES" holds "8"
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
    And the member "member@example.com" holds a token scoped to "email:read"
    When the member "member@example.com" reads the mail "Quote attached"
    Then the mail offers no download of the file "quote.pdf" as it is too large
    When the member "member@example.com" downloads the file "quote.pdf" of the mail "Quote attached"
    Then the download answers not found

  Scenario: Another workspace's attachment answers not found
    Given the workspace "Globex" exists
    And the member "other.member@example.com"
    And the account "other.member@example.com" is placed in the workspace "Globex"
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
    And the member "member@example.com" holds a token scoped to "email:read"
    And the member "other.member@example.com" holds a token scoped to "email:read"
    And the member "member@example.com" shares the file "quote.pdf" of the mail "Quote attached"
    When the member "other.member@example.com" downloads the shared file
    Then the download answers not found

  Scenario: A token without the email area cannot download a file
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
    And the member "member@example.com" holds a token scoped to "email:read"
    And the member "member@example.com" shares the file "quote.pdf" of the mail "Quote attached"
    And the member "member@example.com" holds a token scoped to "contacts:read"
    When the member "member@example.com" downloads the shared file
    Then the download is forbidden
