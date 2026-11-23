// SPDX-License-Identifier: AGPL-3.0-or-later

import { execFileSync } from 'node:child_process'
import { join } from 'node:path'

import type { APIRequestContext } from '@playwright/test'

import { credentials, databaseURL, repoRoot } from './env'
import { graph } from './graph'

/** defaultWorkspace is the id of the one workspace an install starts with. */
const defaultWorkspace = '00000000-0000-7000-8000-000000000001'

/** mailboxAddress is the address of the mailbox the first fed mail stores for the default workspace. */
export const mailboxAddress = 'info@example.com'

/** Sender is the name and address a mail comes from. */
type Sender = { name: string; email: string }

/** Mail holds the parts a raw mail is built from: its sender, subject, Message-ID, received time, reply and lines. */
type Mail = {
	from: Sender
	subject: string
	id: string
	receivedAt: Date
	inReplyTo?: string
	body: string[]
}

/**
 * Creates the contact a sender's mail is stored on, holding the sender's address as its email.
 * @param request - The Playwright request context carrying the credential.
 * @param sender - The sender the contact is named after.
 * @returns The contact id.
 */
export async function knowSender(request: APIRequestContext, sender: Sender): Promise<string> {
	const created = await graph<{ createContact: { id: string } }>(
		request,
		'mutation($name: String!, $identities: [ContactIdentityInput!]) {' +
			' createContact(name: $name, identities: $identities) { id } }',
		{ name: sender.name, identities: [{ channel: 'email', identifier: sender.email }] },
	)
	return created.createContact.id
}

/**
 * Returns a moment as the time the ingest command reads, in whole seconds.
 * @param at - The moment.
 * @returns The moment, such as 2026-11-02T09:00:00Z.
 */
function receivedText(at: Date): string {
	return at.toISOString().replace(/\.\d{3}Z$/, 'Z')
}

/**
 * Builds a raw plain text mail to the mailbox from its parts, every line ended the way mail ends it.
 * @param mail - The parts of the mail.
 * @returns The raw mail.
 */
function rawMail(mail: Mail): string {
	const reply = mail.inReplyTo ? [`In-Reply-To: <${mail.inReplyTo}>`, `References: <${mail.inReplyTo}>`] : []
	const lines = [
		`From: ${mail.from.name} <${mail.from.email}>`,
		`To: Info <${mailboxAddress}>`,
		`Subject: ${mail.subject}`,
		`Date: ${mail.receivedAt.toUTCString().replace('GMT', '+0000')}`,
		`Message-ID: <${mail.id}>`,
		...reply,
		'MIME-Version: 1.0',
		'Content-Type: text/plain; charset=utf-8',
		'',
		...mail.body,
	]
	return `${lines.join('\r\n')}\r\n`
}

/**
 * Feeds a mail into the default workspace as the e2e administrator, storing its mailbox when it holds none.
 * @param mail - The parts of the mail.
 * @returns The one word the command answers, such as stored.
 */
export function ingest(mail: Mail): string {
	const answer = execFileSync(
		join(repoRoot, 'alphone'),
		[
			'email:ingest',
			defaultWorkspace,
			'-as',
			credentials.email,
			'-mailbox',
			mailboxAddress,
			'-received',
			receivedText(mail.receivedAt),
			'-yes',
		],
		{
			input: rawMail(mail),
			encoding: 'utf8',
			env: { ...process.env, ALPHONE_DATABASE_URL: databaseURL },
		},
	)
	return answer.trim()
}
