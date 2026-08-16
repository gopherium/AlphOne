// SPDX-License-Identifier: AGPL-3.0-or-later

import { handlers, importID } from './handlers'

/** The importer leaf routes the outline sweep renders, bound to served fixtures. */
export const paths: Record<string, string> = {
	'/import': '/import',
	'/import/$importId': `/import/${importID}`,
}

export { handlers }
