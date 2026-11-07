// SPDX-License-Identifier: AGPL-3.0-or-later

import type { ErrorMessages } from '@gopherium/react-auth/wpds/users'

import { appErrorTemplates } from '../i18n/errors'

/**
 * Returns the words AlphOne gives the error codes react-auth has no words for.
 * @returns The words, keyed by code.
 */
export function accountErrorMessages(): ErrorMessages {
	const templates = appErrorTemplates()
	return { request_cross_origin: templates.request_cross_origin, tenant_deactivated: templates.tenant_deactivated }
}
