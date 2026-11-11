Feature: A mail is stored on the known contacts it names
  AlphOne keeps a mail only when it names a known contact, and links it to
  every known contact it names. A mail that names nobody AlphOne knows
  leaves no row and no file. The raw message and each attachment go to the
  file store, and the text stays on the row. A mail that cannot be read
  whole keeps its headers when it names a known contact. Support feeds one
  raw mail into a workspace with email:ingest, which only previews until it
  is confirmed and needs an account allowed to manage the mailbox. The same
  mail is stored once however many copies arrive. A new mail tells the
  webhooks that listen for it, while automated mail and copies from the
  Sent folder are stored and stay quiet.

  Background:
    Given the database is migrated
    And the administrator "admin@example.com"
    And the workspace "Acme" exists
    And the workspace "Acme" knows "Maria Perez" at "maria.perez@example.com"

  Scenario: Mail from a known contact attaches to them
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: Info <info@example.com>
      Subject: Quote for the new lamp
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <lamp-quote-request@example.com>
      Content-Type: text/plain

      Could you send me a quote for the new lamp?
      """
    Then the command answers "stored"
    And the contact "Maria Perez" in "Acme" holds the mail "Quote for the new lamp"
    And the mail "Quote for the new lamp" is inbound
    And the mail "Quote for the new lamp" keeps the text "Could you send me a quote for the new lamp?"
    And the mail "Quote for the new lamp" keeps its raw copy

  Scenario: Mail from an unknown address is not stored, and no file is kept
    When this mail reaches "Acme":
      """
      From: stranger@example.com
      To: info@example.com
      Subject: A special offer
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <special-offer@example.com>
      MIME-Version: 1.0
      Content-Type: multipart/mixed; boundary="files"

      --files
      Content-Type: text/plain

      Our price list is attached.

      --files
      Content-Type: application/pdf; name="prices.pdf"
      Content-Disposition: attachment; filename="prices.pdf"
      Content-Transfer-Encoding: base64

      UHJpY2UgbGlzdA==

      --files--
      """
    Then the command answers "no_known_contact"
    And the workspace "Acme" keeps no mail and no file

  Scenario: A message with two known contacts in Cc shows on both
    Given the workspace "Acme" knows "Ada Lovelace" at "ada@example.com"
    When this mail reaches "Acme":
      """
      From: colleague@example.com
      To: info@example.com
      Cc: Maria Perez <maria.perez@example.com>, Ada Lovelace <ada@example.com>
      Subject: Site visit on Friday
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <site-visit-friday@example.com>
      Content-Type: text/plain

      We will both be at the site on Friday.
      """
    Then the workspace "Acme" holds 1 mail
    And the contact "Maria Perez" in "Acme" holds the mail "Site visit on Friday"
    And the contact "Ada Lovelace" in "Acme" holds the mail "Site visit on Friday"

  Scenario: A sender in capitals still matches
    When this mail reaches "Acme":
      """
      From: MARIA.PEREZ@EXAMPLE.COM
      To: info@example.com
      Subject: Opening hours
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <opening-hours@example.com>
      Content-Type: text/plain

      When does the shop open on Saturday?
      """
    Then the contact "Maria Perez" in "Acme" holds the mail "Opening hours"

  Scenario: A sender whose name holds a raw Latin-1 byte still matches
    When a mail titled "Lamp catalogue" reaches "Acme" from a sender whose name holds a raw Latin-1 byte
    Then the contact "Maria Perez" in "Acme" holds the mail "Lamp catalogue"

  Scenario: A subject holding an encoded NUL is stored
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: =?utf-8?q?Quote_for_the=00_lamp?=
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <nul-subject@example.com>
      Content-Type: text/plain

      The subject holds a NUL.
      """
    Then the contact "Maria Perez" in "Acme" holds the mail "Quote for the lamp"

  Scenario: A recipient address holding an encoded NUL is left out and the mail is stored
    Given the workspace "Acme" knows "Ada Lovelace" at "ada@example.com"
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Cc: =?utf-8?q?ad=00a?=@example.com
      Subject: Copy for the file
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <nul-recipient@example.com>
      Content-Type: text/plain

      One address in Cc holds a NUL.
      """
    Then the contact "Maria Perez" in "Acme" holds the mail "Copy for the file"
    And the contact "Ada Lovelace" in "Acme" holds no mail
    And the mail "Copy for the file" names no address "ada@example.com"

  Scenario: A calendar invite with a NUL in its method is stored with no method
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Invitation: Site visit
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <nul-invite@example.com>
      MIME-Version: 1.0
      Content-Type: multipart/alternative; boundary="invite"

      --invite
      Content-Type: text/plain

      Site visit on Friday at ten.

      --invite
      Content-Type: text/calendar; method*=utf-8''RE%00QUEST

      BEGIN:VCALENDAR
      METHOD:REQUEST
      END:VCALENDAR

      --invite--
      """
    Then the contact "Maria Perez" in "Acme" holds the mail "Invitation: Site visit"
    And the mail "Invitation: Site visit" holds no invite method

  Scenario: An attachment with no real media type is stored with the rest of the mail
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Photo of the wall
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <photo-without-subtype@example.com>
      MIME-Version: 1.0
      Content-Type: multipart/mixed; boundary="files"

      --files
      Content-Type: text/plain

      The photo is attached.

      --files
      Content-Type: inline; name="photo.jpg"
      Content-Disposition: attachment; filename="photo.jpg"
      Content-Transfer-Encoding: base64

      UGhvdG8gb2YgdGhlIGZyb250IHdhbGwu

      --files--
      """
    Then the mail "Photo of the wall" keeps the text "The photo is attached."
    And the mail "Photo of the wall" keeps the file "photo.jpg" as "application/octet-stream"

  Scenario: An attachment with no media type at all is stored as application/octet-stream and keeps the file name its mail gives it
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Notes from the visit
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <notes-without-type@example.com>
      MIME-Version: 1.0
      Content-Type: multipart/mixed; boundary="files"

      --files
      Content-Type: text/plain

      My notes from the visit are attached.

      --files
      Content-Disposition: attachment; filename="notes.txt"

      Bring the colour samples on Friday.

      --files--
      """
    Then the mail "Notes from the visit" keeps the text "My notes from the visit are attached."
    And the mail "Notes from the visit" keeps the file "notes.txt" as "application/octet-stream"

  Scenario: A mail with one unreadable part keeps the rest
    When this mail reaches "Acme":
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
    Then the mail "Two files" counts 1 skipped part
    And the mail "Two files" keeps the text "One file reads and one does not."
    And the mail "Two files" keeps the file "quote.pdf" as "application/pdf"

  Scenario: A mail with four attached messages of three levels each is stored under the default container limit
    When a mail titled "Forwarded threads" reaches "Acme" holding 4 attached messages of 3 levels each
    Then the contact "Maria Perez" in "Acme" holds the mail "Forwarded threads"
    And the mail "Forwarded threads" is stored whole

  Scenario: A contact on the mailbox's own address is not matched
    Given the workspace "Acme" knows "Front Desk" at "info@example.com"
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: Info <info@example.com>
      Subject: Opening hours
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <opening-hours@example.com>
      Content-Type: text/plain

      When does the shop open on Saturday?
      """
    Then the contact "Maria Perez" in "Acme" holds the mail "Opening hours"
    And the contact "Front Desk" in "Acme" holds no mail

  Scenario: A contact on a mailbox alias is not matched
    Given the workspace "Acme" knows "Sales Desk" at "sales@example.com"
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: Sales <sales@example.com>
      Subject: A second lamp
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <second-lamp@example.com>
      Content-Type: text/plain

      Could I order a second lamp?
      """
    Then the contact "Maria Perez" in "Acme" holds the mail "A second lamp"
    And the contact "Sales Desk" in "Acme" holds no mail

  Scenario: A contact in Bcc of a Sent copy gains the mail
    Given the workspace "Acme" knows "Ada Lovelace" at "ada@example.com"
    When this Sent copy reaches "Acme":
      """
      From: Info <info@example.com>
      To: Maria Perez <maria.perez@example.com>
      Bcc: Ada Lovelace <ada@example.com>
      Subject: Your quote
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <your-quote@example.com>
      Content-Type: text/plain

      The quote for the new lamp is ready.
      """
    Then the contact "Ada Lovelace" in "Acme" holds the mail "Your quote"
    And the contact "Maria Perez" in "Acme" holds the mail "Your quote"
    And the mail "Your quote" is outbound

  Scenario: A Bcc field on an inbound mail matches nobody
    Given the workspace "Acme" knows "Ada Lovelace" at "ada@example.com"
    When this mail reaches "Acme":
      """
      From: stranger@example.com
      To: info@example.com
      Bcc: Ada Lovelace <ada@example.com>
      Subject: A special offer
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <inbound-bcc@example.com>
      Content-Type: text/plain

      A sender can write anything in Bcc.
      """
    Then the command answers "no_known_contact"
    And the contact "Ada Lovelace" in "Acme" holds no mail
    And the workspace "Acme" keeps no mail and no file

  Scenario: A body over the body cap is stored cut
    Given the setting "ALPHONE_EMAIL_BODY_MAX_BYTES" holds "16"
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Quote for the new lamp
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <lamp-quote-request@example.com>
      Content-Type: text/plain

      Could you send me a quote for the new lamp?
      """
    Then the mail "Quote for the new lamp" is stored cut with the text "Could you send m"
    And the mail "Quote for the new lamp" keeps its raw copy

  Scenario: A mail over the message cap that names a known contact keeps a row with its headers only
    Given the setting "ALPHONE_EMAIL_MESSAGE_MAX_BYTES" holds "300"
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Quote for the new lamp
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <lamp-quote-request@example.com>
      Content-Type: text/plain

      Could you send me a quote for the new lamp? We would like it in brass,
      with a white shade, delivered to the shop before the end of the month.
      The old lamp can go back with the driver.
      """
    Then the command answers "stored_headers_only"
    And the contact "Maria Perez" in "Acme" holds the mail "Quote for the new lamp"
    And the mail "Quote for the new lamp" keeps its headers only
    And the mail "Quote for the new lamp" keeps no raw copy as it is too large

  Scenario: A mail too broken to read that names a known contact keeps a row with its headers only
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Broken mail
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <broken-mail@example.com>
      Content-Type: multipart/mixed

      The boundary is missing.
      """
    Then the command answers "stored_headers_only"
    And the contact "Maria Perez" in "Acme" holds the mail "Broken mail"
    And the mail "Broken mail" keeps its headers only
    And the mail "Broken mail" keeps its raw copy

  Scenario: A mail over the message cap from an unknown address leaves nothing
    Given the setting "ALPHONE_EMAIL_MESSAGE_MAX_BYTES" holds "300"
    When this mail reaches "Acme":
      """
      From: stranger@example.com
      To: info@example.com
      Subject: A special offer
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <large-offer@example.com>
      Content-Type: text/plain

      Our whole price list follows below, with every lamp, every shade and
      every bulb we sell this season, and the price of each one in a table
      that runs well past the size this workspace reads for one mail.
      """
    Then the command answers "headers_only_no_known_contact"
    And the workspace "Acme" keeps no mail and no file

  Scenario: A deactivated workspace stops ingest
    Given the setting "ALPHONE_TENANT_MACHINE_GRACE" holds "0s"
    And the workspace "Acme" was deactivated a day ago
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Opening hours
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <opening-hours@example.com>
      Content-Type: text/plain

      When does the shop open on Saturday?
      """
    Then the command answers "workspace_closed"
    And the workspace "Acme" holds no mailbox
    And the workspace "Acme" keeps no mail and no file

  Scenario: The command only previews without -yes
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
    When this mail is previewed for "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Quote for the new lamp
      Date: Mon, 02 Nov 2026 09:05:00 +0000
      Message-ID: <lamp-quote-request@example.com>
      Content-Type: text/plain

      Could you send me a quote for the new lamp?
      """
    Then the command answers "stored"
    And the answer says nothing changed until it is confirmed with "-yes"
    And the workspace "Acme" holds 1 mail
    When the previewed mail reaches "Acme"
    Then the workspace "Acme" holds 2 mails

  Scenario: A first preview on a new workspace names the outcome and writes nothing
    When this mail is previewed for "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Quote for the new lamp
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <lamp-quote-request@example.com>
      Content-Type: text/plain

      Could you send me a quote for the new lamp?
      """
    Then the command answers "stored"
    And the answer says nothing changed until it is confirmed with "-yes"
    And the workspace "Acme" holds no mailbox
    And the workspace "Acme" keeps no mail and no file

  Scenario: The command needs an account holding email.manage_mailbox
    Given the member "member@example.com"
    When the member "member@example.com" feeds this mail to "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Opening hours
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <opening-hours@example.com>
      Content-Type: text/plain

      When does the shop open on Saturday?
      """
    Then the command exits with code 1
    And the error says "lacks email.manage_mailbox"
    And the workspace "Acme" keeps no mail and no file

  Scenario: A redelivery stores nothing new
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
    When the same mail reaches "Acme" again
    Then the command answers "duplicate"
    And the workspace "Acme" holds 1 mail
    And the workspace "Acme" keeps 1 file

  Scenario: The same mail seen in Inbox and Sent is stored once
    Given this Sent copy reached "Acme":
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
    When this mail reaches "Acme":
      """
      Received: from mail.example.com by mx.example.com with ESMTP
      From: Info <info@example.com>
      To: Maria Perez <maria.perez@example.com>
      Cc: Info <info@example.com>
      Subject: Your quote
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <your-quote@example.com>
      Content-Type: text/plain

      The quote for the new lamp is ready.
      """
    Then the command answers "duplicate"
    And the workspace "Acme" holds 1 mail
    And the mail "Your quote" is outbound
    And the mail "Your quote" has 2 provider references

  Scenario: A Sent copy arriving after its Inbox copy turns the row outbound and adds its Bcc
    Given the workspace "Acme" knows "Ada Lovelace" at "ada@example.com"
    And this mail reached "Acme":
      """
      Received: from mail.example.com by mx.example.com with ESMTP
      From: Info <info@example.com>
      To: Maria Perez <maria.perez@example.com>
      Cc: Info <info@example.com>
      Subject: Your quote
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <your-quote@example.com>
      Content-Type: text/plain

      The quote for the new lamp is ready.
      """
    Then the mail "Your quote" is inbound
    When this Sent copy reaches "Acme":
      """
      From: Info <info@example.com>
      To: Maria Perez <maria.perez@example.com>
      Cc: Info <info@example.com>
      Bcc: Ada Lovelace <ada@example.com>
      Subject: Your quote
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <your-quote@example.com>
      Content-Type: text/plain

      The quote for the new lamp is ready.
      """
    Then the command answers "linked"
    And the workspace "Acme" holds 1 mail
    And the mail "Your quote" is outbound
    And the contact "Ada Lovelace" in "Acme" holds the mail "Your quote"
    And the mail "Your quote" names the address "ada@example.com"

  Scenario: A second copy naming another contact gains the link and the address, not a copy
    Given the workspace "Acme" knows "Ada Lovelace" at "ada@example.com"
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
    When this mail reaches "Acme":
      """
      Received: from mail.example.com by mx.example.com with ESMTP
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Cc: Ada Lovelace <ada@example.com>
      Subject: Quote for the new lamp
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <lamp-quote-request@example.com>
      Content-Type: text/plain

      Could you send me a quote for the new lamp?
      """
    Then the command answers "linked"
    And the workspace "Acme" holds 1 mail
    And the contact "Ada Lovelace" in "Acme" holds the mail "Quote for the new lamp"
    And the mail "Quote for the new lamp" names the address "ada@example.com"

  Scenario: A broken sender reusing one Message-ID keeps both mails
    Given this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: First order
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <reused-by-the-sender@example.com>
      Content-Type: text/plain

      Please send one brass lamp.
      """
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Second order
      Date: Tue, 03 Nov 2026 09:00:00 +0000
      Message-ID: <reused-by-the-sender@example.com>
      Content-Type: text/plain

      Please send two white shades.
      """
    Then the command answers "stored"
    And the workspace "Acme" holds 2 mails
    And the contact "Maria Perez" in "Acme" holds the mail "First order"
    And the contact "Maria Perez" in "Acme" holds the mail "Second order"

  Scenario: Two different mails with one Message-ID, one From and one Date keep both
    Given this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Invoice
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <fixed-invoice-id@example.com>
      MIME-Version: 1.0
      Content-Type: multipart/mixed; boundary="files"

      --files
      Content-Type: text/plain

      The invoice is attached.

      --files
      Content-Type: application/pdf; name="invoice.pdf"
      Content-Disposition: attachment; filename="invoice.pdf"
      Content-Transfer-Encoding: base64

      SW52b2ljZSBmb3IgTWFyY2g=

      --files--
      """
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Invoice
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <fixed-invoice-id@example.com>
      MIME-Version: 1.0
      Content-Type: multipart/mixed; boundary="files"

      --files
      Content-Type: text/plain

      The invoice is attached.

      --files
      Content-Type: application/pdf; name="invoice.pdf"
      Content-Disposition: attachment; filename="invoice.pdf"
      Content-Transfer-Encoding: base64

      SW52b2ljZSBmb3IgQXByaWw=

      --files--
      """
    Then the command answers "stored"
    And the workspace "Acme" holds 2 mails

  Scenario: Two broken mails with one Message-ID, one From and one Date keep both
    Given this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Broken mail
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <broken-mail@example.com>
      Content-Type: multipart/mixed

      The invoice for March, and the boundary is missing.
      """
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Broken mail
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <broken-mail@example.com>
      Content-Type: multipart/mixed

      The invoice for April, and the boundary is missing.
      """
    Then the command answers "stored_headers_only"
    And the workspace "Acme" holds 2 mails

  Scenario: Two mails with one Message-ID, one From and one Date that differ in a part AlphOne cannot read keep both
    Given this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Invoice
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <unreadable-invoice@example.com>
      MIME-Version: 1.0
      Content-Type: multipart/mixed; boundary="files"

      --files
      Content-Type: text/plain

      Your invoice is attached.
      --files
      Content-Type: application/x-foo bar

      The invoice for March.
      --files--
      """
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Invoice
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <unreadable-invoice@example.com>
      MIME-Version: 1.0
      Content-Type: multipart/mixed; boundary="files"

      --files
      Content-Type: text/plain

      Your invoice is attached.
      --files
      Content-Type: application/x-foo bar

      The invoice for April.
      --files--
      """
    Then the command answers "stored"
    And the workspace "Acme" holds 2 mails

  Scenario: An auto reply is stored but never published
    Given a webhook of "Acme" listens for "email.message.received"
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Automatic reply: Your quote
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <auto-reply@example.com>
      Auto-Submitted: auto-replied
      Content-Type: text/plain

      I am out of the office until Monday.
      """
    Then the contact "Maria Perez" in "Acme" holds the mail "Automatic reply: Your quote"
    And no "email.message.received" event is queued

  Scenario: A Sent copy is stored but never published
    Given a webhook of "Acme" listens for "email.message.received"
    When this Sent copy reaches "Acme":
      """
      From: Info <info@example.com>
      To: Maria Perez <maria.perez@example.com>
      Subject: Your quote
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <your-quote@example.com>
      Content-Type: text/plain

      The quote for the new lamp is ready.
      """
    Then the contact "Maria Perez" in "Acme" holds the mail "Your quote"
    And no "email.message.received" event is queued

  Scenario: A forged mail from the mailbox's own address is stored as inbound and never published
    Given a webhook of "Acme" listens for "email.message.received"
    When this mail reaches "Acme":
      """
      From: Info <info@example.com>
      To: Maria Perez <maria.perez@example.com>
      Subject: Your account
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <forged-own-address@example.com>
      Content-Type: text/plain

      Please confirm your account details.
      """
    Then the contact "Maria Perez" in "Acme" holds the mail "Your account"
    And the mail "Your account" is inbound
    And no "email.message.received" event is queued

  Scenario: A new mail publishes its ids, its contacts, its stored subject and its text, never its HTML
    Given a webhook of "Acme" listens for "email.message.received"
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Quote for the new lamp
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <lamp-quote-request@example.com>
      MIME-Version: 1.0
      Content-Type: multipart/alternative; boundary="alternative"

      --alternative
      Content-Type: text/plain

      Could you send me a quote for the new lamp?

      --alternative
      Content-Type: text/html

      <p>Could you send me a quote for the <b>new lamp</b>?</p>

      --alternative--
      """
    Then the event "email.message.received" carries the id, thread, contacts, subject and text of the mail "Quote for the new lamp" and nothing else

  Scenario: A text longer than the event cap reaches the event cut at the cap, and the row keeps the whole text
    Given the setting "ALPHONE_EMAIL_EVENT_TEXT_MAX_BYTES" holds "16"
    And a webhook of "Acme" listens for "email.message.received"
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Quote for the new lamp
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <lamp-quote-request@example.com>
      Content-Type: text/plain

      Could you send me a quote for the new lamp?
      """
    Then the event "email.message.received" carries the text "Could you send m"
    And the mail "Quote for the new lamp" keeps the text "Could you send me a quote for the new lamp?"
