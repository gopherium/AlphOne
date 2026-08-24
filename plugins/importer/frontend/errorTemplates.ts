// SPDX-License-Identifier: AGPL-3.0-or-later

import { __ } from '@alphone/frontend-sdk'

/** DOMAIN is the text domain the plugin's messages answer under. */
const DOMAIN = 'alphone-importer'

/**
 * Returns the message each reason of this plugin stands for, read fresh so the
 * catalogue the reader loaded is the one that answers.
 * @returns The messages, keyed by reason.
 */
export function errorTemplates(): Record<string, string> {
	return {
		import_not_found: __('That import no longer exists.', DOMAIN),
		file_too_large: __('The file runs past %(maxBytes)d bytes.', DOMAIN),
		file_unreadable: __('AlphOne could not read that file as a CSV or a spreadsheet.', DOMAIN),
		mapping_invalid: __('The mapping does not fit the columns the file holds.', DOMAIN),
		mapping_required: __('Choose what each column holds before committing.', DOMAIN),
		mapping_locked: __('This import no longer accepts a mapping.', DOMAIN),
		already_committed: __('This import was committed already.', DOMAIN),
	}
}
