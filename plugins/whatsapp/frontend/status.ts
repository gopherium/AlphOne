// SPDX-License-Identifier: AGPL-3.0-or-later

const failureCopy: Record<number, string> = {
	131047: 'Outside the 24-hour window. The customer must message first.',
}

const genericFailureCopy = 'Not delivered.'

/**
 * Maps a stored delivery failure detail, whose first token is the Graph
 * error code, to operator-facing copy.
 * @param detail - The stored failure detail, if any.
 * @returns The human explanation for the failure.
 */
export function copyForFailureDetail(detail: string | null | undefined): string {
	if (!detail) {
		return genericFailureCopy
	}
	const code = Number.parseInt(detail, 10)
	if (Number.isNaN(code)) {
		return genericFailureCopy
	}
	return failureCopy[code] ?? genericFailureCopy
}
