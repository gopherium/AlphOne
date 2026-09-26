// SPDX-License-Identifier: AGPL-3.0-or-later

import { __ } from '@alphone/frontend-sdk'

/** DOMAIN is the text domain the plugin's messages answer under. */
const DOMAIN = 'alphone-fields'

/**
 * Returns the message each reason of this plugin stands for, read fresh so the
 * catalogue the reader loaded is the one that answers.
 * @returns The messages, keyed by reason.
 */
export function errorTemplates(): Record<string, string> {
	return {
		field_name_malformed: __('A field name starts lowercase and runs together, like birthDate.', DOMAIN),
		field_label_required: __('A field needs a label.', DOMAIN),
		field_label_too_long: __('That label is too long. Try a shorter one.', DOMAIN),
		field_kind_unknown: __('That kind is not one AlphOne offers.', DOMAIN),
		field_name_reserved: __('The contact already holds a detail by that name.', DOMAIN),
		field_sub_fields_required: __('A repeater needs at least one sub field.', DOMAIN),
		field_sub_fields_unexpected: __('Only a repeater holds sub fields.', DOMAIN),
		field_sub_field_nested: __('A repeater cannot hold another repeater.', DOMAIN),
		field_sub_field_name_invalid: __('A sub field name starts lowercase and runs together, like followUp.', DOMAIN),
		field_sub_field_name_taken: __('Two sub fields of this repeater hold the same name.', DOMAIN),
		field_name_taken: __('Another field already holds that name.', DOMAIN),
		field_kind_locked: __('An archived field of that name holds another kind or other sub fields.', DOMAIN),
		field_not_found: __('That field no longer exists.', DOMAIN),
		field_unknown: __('That field is not one this contact holds.', DOMAIN),
		value_kind_mismatch: __('That value does not match the kind the field declares.', DOMAIN),
		values_not_an_object: __('Send the values as field names to values.', DOMAIN),
	}
}
