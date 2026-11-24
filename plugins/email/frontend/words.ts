// SPDX-License-Identifier: AGPL-3.0-or-later

import { __ } from '@alphone/frontend-sdk'

/** MailText is the part of a mail the inbox reads its preview and marks from. */
interface MailText {
	snippet: string
	encrypted: boolean
	signed: boolean
	bodyState: string
}

/** MailSender is the part of a mail the inbox reads its sender from. */
interface MailSender {
	from: { name: string; email: string } | null
}

/**
 * Returns the preview of a mail, fixed words for a mail that holds no text to show.
 * @param mail - The mail.
 * @returns The fixed words for an encrypted mail or one read by its headers alone, the stored preview otherwise.
 */
export function previewOf(mail: MailText): string {
	if (mail.encrypted) {
		return __('Encrypted message', 'alphone-email')
	}
	if (mail.bodyState === 'headers_only') {
		return __('No text could be read', 'alphone-email')
	}
	return mail.snippet
}

/**
 * Returns the words of each mark a mail carries, Encrypted before Signed.
 * @param mail - The mail.
 * @returns The words, none for a plain mail.
 */
export function marksOf(mail: MailText): string[] {
	return [
		...(mail.encrypted ? [__('Encrypted', 'alphone-email')] : []),
		...(mail.signed ? [__('Signed', 'alphone-email')] : []),
	]
}

/**
 * Returns the name a mail's sender shows under.
 * @param mail - The mail.
 * @returns The sender's name, the address when it gives none, fixed words when the mail names no sender.
 */
export function senderOf(mail: MailSender): string {
	return mail.from?.name || mail.from?.email || __('(no sender)', 'alphone-email')
}

/**
 * Returns the subject a thread shows.
 * @param thread - The thread.
 * @returns The subject, fixed words when the thread has none.
 */
export function subjectOf(thread: { subject: string }): string {
	return thread.subject || __('(no subject)', 'alphone-email')
}
