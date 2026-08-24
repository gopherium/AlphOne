// SPDX-License-Identifier: AGPL-3.0-or-later

import { __ } from '@alphone/frontend-sdk'

/**
 * Returns the copy for the known failure codes, read fresh so the loaded catalogue answers.
 * @returns The copy, keyed by Graph error code.
 */
function failureCopy(): Record<number, string> {
	return {
		131047: __('Outside the 24-hour window. The customer must message first.', 'alphone-whatsapp'),
	}
}

/**
 * Returns the copy for a failure no code explains.
 * @returns The generic failure copy.
 */
function genericFailureCopy(): string {
	return __('Not delivered.', 'alphone-whatsapp')
}

/**
 * Maps a Graph failure code to operator-facing copy.
 * @param code - The Graph error code.
 * @returns The mapped copy, or null when the code is unknown.
 */
export function copyForFailureCode(code: number): string | null {
	return failureCopy()[code] ?? null
}

/**
 * Maps a stored delivery failure detail, whose first token is the Graph
 * error code, to operator-facing copy.
 * @param detail - The stored failure detail, if any.
 * @returns The human explanation for the failure.
 */
export function copyForFailureDetail(detail: string | null | undefined): string {
	if (!detail) {
		return genericFailureCopy()
	}
	const code = Number.parseInt(detail, 10)
	if (Number.isNaN(code)) {
		return genericFailureCopy()
	}
	return copyForFailureCode(code) ?? genericFailureCopy()
}
