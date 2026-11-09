// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"fmt"
)

// lampText is the text of the mail asking for a lamp quote.
const lampText = "Could you send me a quote for the new lamp?"

// receivedLine is a header line a relay adds, which leaves the dedupe key of a mail as it is.
const receivedLine = "Received: from mail.example.com by mx.example.com with ESMTP"

// lampMail returns the mail of Maria Perez asking for a lamp quote, with header lines put before its own.
func lampMail(before ...string) []byte {
	return quoteMail(before, lampText)
}

// strangerMail returns a plain text offer from an address no contact holds, with header lines put before its own.
func strangerMail(before ...string) []byte {
	return rawMail(append(before,
		"From: stranger@example.com",
		"To: info@example.com",
		"Subject: A special offer",
		"Date: "+quoteDate,
		"Message-ID: <offer@example.com>",
		"Content-Type: text/plain",
		"",
		"Our whole price list follows below, with every lamp, every shade and every bulb we sell this season.",
	)...)
}

// brokenMail returns a mail from email whose multipart body names no boundary.
func brokenMail(email string) []byte {
	return rawMail("From: "+email, "To: info@example.com", "Subject: Broken mail", "Date: "+quoteDate,
		"Message-ID: <broken-mail@example.com>", "Content-Type: multipart/mixed", "", "The boundary is missing.")
}

// numberedMail returns a plain text mail from Maria Perez whose id and text carry n.
func numberedMail(n int) []byte {
	return rawMail("From: Maria Perez <maria.perez@example.com>", "To: info@example.com",
		fmt.Sprintf("Subject: Order %d", n), "Date: "+quoteDate, fmt.Sprintf("Message-ID: <order-%d@example.com>", n),
		"Content-Type: text/plain", "", fmt.Sprintf("Please send %d lamps.", n))
}

// infoMail returns the reply of the mailbox to Maria Perez, with header lines put before its own.
func infoMail(before ...string) []byte {
	return rawMail(append(before,
		"From: Info <info@example.com>",
		"To: Maria Perez <maria.perez@example.com>",
		"Subject: Your quote",
		"Date: "+quoteDate,
		"Message-ID: <your-quote@example.com>",
		"Content-Type: text/plain",
		"",
		"The quote for the new lamp is ready.",
	)...)
}
