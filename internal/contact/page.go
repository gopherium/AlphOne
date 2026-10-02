// SPDX-License-Identifier: Elastic-2.0

package contact

// Filter narrows the contact directory to the contacts a search and a channel choice match.
type Filter struct {
	// Query matches a name or an identity display name. Empty matches every contact.
	Query string
	// Digits matches the digits of an identity identifier. Empty matches no identifier.
	Digits string
	// Channels keeps the contacts reachable on any of them. Empty keeps every contact.
	Channels []string
}

// Page orders and bounds one offset page of the contact directory.
type Page struct {
	// ByCreated sorts by creation time in place of the name.
	ByCreated bool
	// Descending reverses the order.
	Descending bool
	// Limit is the most contacts the page holds.
	Limit int
	// Offset is how many contacts come before the page.
	Offset int
}
