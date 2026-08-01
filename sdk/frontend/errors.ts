// SPDX-License-Identifier: AGPL-3.0-or-later

/** ValidationError carries a backend validation message worth showing verbatim. */
export class ValidationError extends Error {}

/**
 * Returns the message of a validation error, or the fallback for anything else.
 * @param error - The mutation error.
 * @param fallback - The copy shown for non-validation failures.
 * @returns The message to show the user.
 */
export function validationMessage(error: unknown, fallback: string): string {
	if (error instanceof ValidationError) {
		return error.message
	}
	return fallback
}
