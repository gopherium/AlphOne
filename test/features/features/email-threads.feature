Feature: A mail joins the thread of its conversation
  A mail joins a thread through the provider's thread id, its reply
  headers, and then a reply subject inside a time window. A thread takes a
  mail only when the two share a contact. A reply header counts for any
  contact the mail names, while a provider thread id or a subject counts
  only for the sender's own contact, so two customers who write the same
  subject never meet in one thread. A reply stored before the mail it
  answers still joins it, and a thread lists its mail in the order it was
  received.

  Background:
    Given the database is migrated
    And the administrator "admin@example.com"
    And the workspace "Acme" exists
    And the workspace "Acme" knows "Maria Perez" at "maria.perez@example.com"
    And the workspace "Acme" knows "Ada Lovelace" at "ada@example.com"

  Scenario: A reply stored before its parent joins the parent's thread
    Given this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: Info <info@example.com>
      Subject: Re: Your quote
      Date: Mon, 02 Nov 2026 10:00:00 +0000
      Message-ID: <quote-reply@example.com>
      In-Reply-To: <your-quote@example.com>
      References: <your-quote@example.com>
      Content-Type: text/plain

      Thank you, the price works for us.
      """
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
    Then the workspace "Acme" holds 1 thread
    And the mails "Your quote" and "Re: Your quote" share a thread

  Scenario: A parent that arrives late joins two threads of one contact into one
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
    And the workspace "Acme" holds 2 threads
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
    Then the workspace "Acme" holds 1 thread
    And the mails "Re: Visit plan" and "Re: Visit time" share a thread

  Scenario: Two contacts who both write Invoice keep separate threads
    Given this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Re: Invoice
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <invoice-question-maria@example.com>
      Content-Type: text/plain

      Is the invoice paid?
      """
    When this mail reaches "Acme":
      """
      From: Ada Lovelace <ada@example.com>
      To: info@example.com
      Subject: Re: Invoice
      Date: Mon, 02 Nov 2026 09:05:00 +0000
      Message-ID: <invoice-question-ada@example.com>
      Content-Type: text/plain

      Could you send the invoice again?
      """
    Then the workspace "Acme" holds 2 threads
    And the contacts "Maria Perez" and "Ada Lovelace" in "Acme" share no thread

  Scenario: A Re subject from another contact with the same third person in Cc starts a new thread
    Given the workspace "Acme" knows "Accounts Desk" at "accounts@example.com"
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Cc: Accounts Desk <accounts@example.com>
      Subject: Re: Invoice
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <invoice-copied-maria@example.com>
      Content-Type: text/plain

      Is the invoice paid?
      """
    When this mail reaches "Acme":
      """
      From: Ada Lovelace <ada@example.com>
      To: info@example.com
      Cc: Accounts Desk <accounts@example.com>
      Subject: Re: Invoice
      Date: Mon, 02 Nov 2026 09:05:00 +0000
      Message-ID: <invoice-copied-ada@example.com>
      Content-Type: text/plain

      Could you send the invoice again?
      """
    Then the workspace "Acme" holds 2 threads
    And the contacts "Maria Perez" and "Ada Lovelace" in "Acme" share no thread

  Scenario: A Re subject from the same contact inside the window joins the thread
    Given this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Invoice
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <invoice-sent@example.com>
      Content-Type: text/plain

      Here is the invoice for the lamp.
      """
    And 3 days pass
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Re: Invoice
      Date: Thu, 05 Nov 2026 09:00:00 +0000
      Message-ID: <invoice-follow-up@example.com>
      Content-Type: text/plain

      Did the invoice reach you?
      """
    Then the workspace "Acme" holds 1 thread
    And the mails "Invoice" and "Re: Invoice" share a thread

  Scenario: A Re subject outside the window starts a new thread
    Given this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Invoice
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <invoice-sent@example.com>
      Content-Type: text/plain

      Here is the invoice for the lamp.
      """
    And 15 days pass
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Re: Invoice
      Date: Tue, 17 Nov 2026 09:00:00 +0000
      Message-ID: <invoice-follow-up@example.com>
      Content-Type: text/plain

      Did the invoice reach you?
      """
    Then the workspace "Acme" holds 2 threads
    And the mails "Invoice" and "Re: Invoice" do not share a thread

  Scenario: A repeated plain subject starts a new thread
    Given this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Invoice
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <invoice-march@example.com>
      Content-Type: text/plain

      Here is the invoice for March.
      """
    And 1 day passes
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Invoice
      Date: Tue, 03 Nov 2026 09:00:00 +0000
      Message-ID: <invoice-april@example.com>
      Content-Type: text/plain

      Here is the invoice for April.
      """
    Then the workspace "Acme" holds 2 threads

  Scenario: A repeated subject under one provider thread id with no reply ids starts a new thread
    Given this mail reached "Acme" in the provider thread "conversation-invoices":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Invoice
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <invoice-march@example.com>
      Content-Type: text/plain

      Here is the invoice for March.
      """
    And 30 days pass
    When this mail reaches "Acme" in the provider thread "conversation-invoices":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Invoice
      Date: Wed, 02 Dec 2026 09:00:00 +0000
      Message-ID: <invoice-april@example.com>
      Content-Type: text/plain

      Here is the invoice for April.
      """
    Then the workspace "Acme" holds 2 threads

  Scenario: A References header pointing at another contact's thread starts a new thread
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
    When this mail reaches "Acme":
      """
      From: Ada Lovelace <ada@example.com>
      To: info@example.com
      Subject: Re: Site visit
      Date: Mon, 02 Nov 2026 09:05:00 +0000
      Message-ID: <site-visit-reference@example.com>
      In-Reply-To: <site-visit@example.com>
      References: <site-visit@example.com>
      Content-Type: text/plain

      I hold the id of a thread that is not mine.
      """
    Then the workspace "Acme" holds 2 threads
    And the contacts "Maria Perez" and "Ada Lovelace" in "Acme" share no thread

  Scenario: A reply from an unknown sender with the thread's contact in Cc joins the thread
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
    When this mail reaches "Acme":
      """
      From: colleague@example.com
      To: info@example.com
      Cc: Maria Perez <maria.perez@example.com>
      Subject: Re: Site visit
      Date: Mon, 02 Nov 2026 09:05:00 +0000
      Message-ID: <site-visit-colleague@example.com>
      In-Reply-To: <site-visit@example.com>
      References: <site-visit@example.com>
      Content-Type: text/plain

      I will come to the site as well.
      """
    Then the workspace "Acme" holds 1 thread
    And the mails "Site visit" and "Re: Site visit" share a thread

  Scenario: The same Message-ID in two workspaces stays apart
    Given the workspace "Globex" exists
    And the workspace "Globex" knows "Maria Perez" at "maria.perez@example.com"
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
    When the same mail reaches "Globex"
    Then the command answers "stored"
    And the workspace "Acme" holds 1 mail
    And the workspace "Globex" holds 1 mail
    And the workspace "Acme" holds 1 thread
    And the workspace "Globex" holds 1 thread

  Scenario: A mail naming two threads of one contact joins them
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
    And this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Delivery date
      Date: Mon, 02 Nov 2026 09:05:00 +0000
      Message-ID: <delivery-date@example.com>
      Content-Type: text/plain

      When will the lamp arrive?
      """
    And the workspace "Acme" holds 2 threads
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Both plans
      Date: Mon, 02 Nov 2026 09:10:00 +0000
      Message-ID: <both-plans@example.com>
      In-Reply-To: <site-visit@example.com>
      References: <delivery-date@example.com> <site-visit@example.com>
      Content-Type: text/plain

      The visit and the delivery can be on the same day.
      """
    Then the workspace "Acme" holds 1 thread

  Scenario: A mail naming threads of two different contacts merges nothing
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
    When this mail reaches "Acme":
      """
      From: colleague@example.com
      To: info@example.com
      Cc: Maria Perez <maria.perez@example.com>, Ada Lovelace <ada@example.com>
      Subject: Re: Site visit
      Date: Mon, 02 Nov 2026 09:10:00 +0000
      Message-ID: <both-customers@example.com>
      In-Reply-To: <site-visit@example.com>
      References: <delivery-date@example.com> <site-visit@example.com>
      Content-Type: text/plain

      One reply names the threads of two customers.
      """
    Then the workspace "Acme" holds 2 threads
    And the mails "Site visit" and "Re: Site visit" share a thread
    And the mails "Site visit" and "Delivery date" do not share a thread

  Scenario: A stored mail takes its own id back from a mention
    Given this mail reached "Acme":
      """
      From: Ada Lovelace <ada@example.com>
      To: info@example.com
      Subject: Delivery date
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <delivery-date@example.com>
      References: <lamp-quote@example.com>
      Content-Type: text/plain

      My note names the id of a mail that has not arrived yet.
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
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Delivery of the lamp
      Date: Mon, 02 Nov 2026 09:10:00 +0000
      Message-ID: <lamp-quote-follow-up@example.com>
      In-Reply-To: <lamp-quote@example.com>
      References: <lamp-quote@example.com>
      Content-Type: text/plain

      Please deliver the lamp on Friday.
      """
    Then the mails "Lamp quote" and "Delivery of the lamp" share a thread
    And the mails "Lamp quote" and "Delivery date" do not share a thread

  Scenario: A provider thread id shared by two contacts keeps them apart
    Given this mail reached "Acme" in the provider thread "conversation-invoices":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Invoice
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <invoice-maria@example.com>
      Content-Type: text/plain

      Here is my invoice.
      """
    When this mail reaches "Acme" in the provider thread "conversation-invoices":
      """
      From: Ada Lovelace <ada@example.com>
      To: info@example.com
      Subject: Invoice
      Date: Mon, 02 Nov 2026 09:05:00 +0000
      Message-ID: <invoice-ada@example.com>
      Content-Type: text/plain

      Here is my invoice too.
      """
    Then the workspace "Acme" holds 2 threads
    And the contacts "Maria Perez" and "Ada Lovelace" in "Acme" share no thread

  Scenario: The timeline follows the received time, not a far future Date
    Given this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Site visit
      Date: Thu, 01 Jan 2099 09:00:00 +0000
      Message-ID: <site-visit@example.com>
      Content-Type: text/plain

      Could we visit the site on Friday?
      """
    When this mail reaches "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Re: Site visit
      Date: Mon, 02 Nov 2026 09:05:00 +0000
      Message-ID: <site-visit-again@example.com>
      In-Reply-To: <site-visit@example.com>
      References: <site-visit@example.com>
      Content-Type: text/plain

      Friday at ten works for me.
      """
    Then the thread of "Site visit" lists "Site visit, Re: Site visit" in time order

  Scenario: A mail under a provider thread id joins the conversation near its own time
    Given this mail reached "Acme" in the provider thread "conversation-invoices":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Invoice for March
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <invoice-march@example.com>
      Content-Type: text/plain

      Here is the invoice for March.
      """
    And 30 days pass
    And this mail reached "Acme" in the provider thread "conversation-invoices":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Invoice for April
      Date: Wed, 02 Dec 2026 09:00:00 +0000
      Message-ID: <invoice-april@example.com>
      Content-Type: text/plain

      Here is the invoice for April.
      """
    And 1 day passes
    When this mail reaches "Acme" in the provider thread "conversation-invoices":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: A question on the April invoice
      Date: Thu, 03 Dec 2026 09:00:00 +0000
      Message-ID: <invoice-question@example.com>
      Content-Type: text/plain

      Does the April invoice include the lamp?
      """
    Then the workspace "Acme" holds 2 threads
    And the mails "Invoice for April" and "A question on the April invoice" share a thread
    And the mails "Invoice for March" and "Invoice for April" do not share a thread

  Scenario: An older mail fed after a newer conversation joins the conversation of its own time
    Given this mail reached "Acme" in the provider thread "conversation-invoices":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Invoice for March
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <invoice-march@example.com>
      Content-Type: text/plain

      Here is the invoice for March.
      """
    And 30 days pass
    And this mail reached "Acme" in the provider thread "conversation-invoices":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Invoice for April
      Date: Wed, 02 Dec 2026 09:00:00 +0000
      Message-ID: <invoice-april@example.com>
      Content-Type: text/plain

      Here is the invoice for April.
      """
    And the next mail is received 1 day after the first mail
    When this mail reaches "Acme" in the provider thread "conversation-invoices":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: A question on the March invoice
      Date: Tue, 03 Nov 2026 09:00:00 +0000
      Message-ID: <invoice-march-question@example.com>
      Content-Type: text/plain

      Does the March invoice include the lamp?
      """
    Then the workspace "Acme" holds 2 threads
    And the mails "Invoice for March" and "A question on the March invoice" share a thread
    And the mails "Invoice for March" and "Invoice for April" do not share a thread

  Scenario: A copy that brings a provider thread id leads later mail of that thread to the stored mail
    Given this mail reached "Acme":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Lamp quote
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <lamp-quote@example.com>
      Content-Type: text/plain

      Could you send me a quote for the lamp?
      """
    And this mail reached "Acme" in the provider thread "conversation-lamp":
      """
      Received: from mail.example.com by mx.example.com with ESMTP
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Lamp quote
      Date: Mon, 02 Nov 2026 09:00:00 +0000
      Message-ID: <lamp-quote@example.com>
      Content-Type: text/plain

      Could you send me a quote for the lamp?
      """
    When this mail reaches "Acme" in the provider thread "conversation-lamp":
      """
      From: Maria Perez <maria.perez@example.com>
      To: info@example.com
      Subject: Delivery date
      Date: Mon, 02 Nov 2026 10:00:00 +0000
      Message-ID: <delivery-date@example.com>
      Content-Type: text/plain

      When can the lamp arrive?
      """
    Then the workspace "Acme" holds 2 mails
    And the mails "Lamp quote" and "Delivery date" share a thread
