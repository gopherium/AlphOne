// SPDX-License-Identifier: AGPL-3.0-or-later

import { __ } from '@alphone/frontend-sdk'

/** DOMAIN is the text domain the core messages answer under. */
const DOMAIN = 'alphone'

/**
 * Returns the message each core reason stands for, read fresh so the
 * catalogue the reader loaded is the one that answers.
 * @returns The messages, keyed by reason.
 */
export function errorTemplates(): Record<string, string> {
	return {
		authentication_required: __('Sign in to go on.', DOMAIN),
		credentials_invalid: __('That email and password do not match an account.', DOMAIN),
		rate_limited: __('Too many attempts. Wait %(retryAfter)d seconds and try again.', DOMAIN),
		scope_missing: __('This token does not reach %(scope)s.', DOMAIN),
		capability_missing: __('Your role does not allow %(capability)s.', DOMAIN),
		contact_name_required: __('A contact needs a name.', DOMAIN),
		identity_channel_required: __('Choose the channel this address belongs to.', DOMAIN),
		identity_identifier_required: __('Write the address itself.', DOMAIN),
		identity_taken: __('Another contact already holds that address.', DOMAIN),
		channel_not_writable: __('AlphOne cannot write to that channel.', DOMAIN),
		identity_not_found: __('That address is no longer on this contact.', DOMAIN),
		contact_not_found: __('That contact no longer exists.', DOMAIN),
		task_title_required: __('A task needs a title.', DOMAIN),
		task_priority_unknown: __('That priority is not one AlphOne offers.', DOMAIN),
		task_status_unknown: __('That status is not one AlphOne offers.', DOMAIN),
		task_filter_choice_required: __('Ask for tasks by one filter at a time.', DOMAIN),
		task_not_found: __('That task no longer exists.', DOMAIN),
		origin_source_required: __('An event needs the source it came from.', DOMAIN),
		event_unknown: __('That event name is not one AlphOne knows.', DOMAIN),
		webhook_url_invalid: __('Write the address as a full URL.', DOMAIN),
		webhook_events_required: __('Choose at least one event to send.', DOMAIN),
		webhook_not_found: __('That webhook no longer exists.', DOMAIN),
		first_out_of_range: __('Ask for between %(min)d and %(max)d at a time.', DOMAIN),
		locale_unknown: __('AlphOne does not speak that language yet.', DOMAIN),
		cursor_malformed: __('That page marker is not one AlphOne issued.', DOMAIN),
		value_malformed: __('That value is not in the form AlphOne expects.', DOMAIN),
		token_name_required: __('A token needs a name.', DOMAIN),
		token_not_found: __('That token no longer exists.', DOMAIN),
		scope_malformed: __('Write a scope as an area, a colon, then read or write.', DOMAIN),
		scopes_required: __('Choose at least one area the token reaches.', DOMAIN),
		area_unknown: __('That area is not one AlphOne offers.', DOMAIN),
		lifetime_negative: __('A lifetime is zero days or more.', DOMAIN),
		lifetime_too_long: __('A lifetime runs to %(maxDays)d days at most.', DOMAIN),
		email_invalid: __('Write the address as a full email address.', DOMAIN),
		email_taken: __('Another account already holds that address.', DOMAIN),
		name_required: __('An account needs a name.', DOMAIN),
		name_too_long: __('A name runs to %(max)d characters at most.', DOMAIN),
		password_too_short: __('A password runs to %(min)d characters at least.', DOMAIN),
		password_too_long: __('A password runs to %(max)d characters at most.', DOMAIN),
		user_not_found: __('That account no longer exists.', DOMAIN),
		self_disable_refused: __('You cannot disable your own account.', DOMAIN),
		self_role_refused: __('You cannot change your own role.', DOMAIN),
		last_privileged_refused: __('Somebody must be able to manage users.', DOMAIN),
		role_beyond_reach: __('That role holds more than your own.', DOMAIN),
		role_unknown: __('That role is not one this deployment names.', DOMAIN),
	}
}

/**
 * Returns the words a reader is told when the server said nothing readable.
 * @returns The fallback message.
 */
export function errorFallback(): string {
	return __('Something went wrong. Try again.', DOMAIN)
}
