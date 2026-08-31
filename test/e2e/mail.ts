// SPDX-License-Identifier: AGPL-3.0-or-later

import { Buffer } from 'node:buffer'

import { SMTPServer } from 'smtp-server'

/** smtpPort is the port the mail sink listens on for the server under test. */
export const smtpPort = Number(process.env.ALPHONE_E2E_SMTP_PORT ?? 4792)

/** publicURL is the address the server under test builds its mailed links from. */
export const publicURL = process.env.ALPHONE_E2E_PUBLIC_URL ?? 'http://localhost:8080'

/**
 * MailSink collects the messages the server under test delivers.
 */
export interface MailSink {
	/** Answers the decoded body of the first mail addressed to one recipient. */
	waitFor(recipient: string): Promise<string>
	/** Answers how many mails the sink holds. */
	count(): number
	/** Forgets every mail collected so far. */
	clear(): void
	/** Stops listening. */
	close(): Promise<void>
}

/**
 * Decodes a quoted-printable transfer encoding the way a mail client does.
 * @param encoded - The encoded body.
 * @returns The decoded text.
 */
function decodeQuotedPrintable(encoded: string): string {
	return encoded
		.replace(/=\r?\n/g, '')
		.replace(/=([0-9A-Fa-f]{2})/g, (_, hex: string) =>
			String.fromCharCode(Number.parseInt(hex, 16)),
		)
}

/**
 * Returns the decoded body of one raw message.
 * @param raw - The message as the relay received it.
 * @returns The decoded body.
 */
function bodyOf(raw: string): string {
	const separator = raw.indexOf('\r\n\r\n')
	if (separator === -1) {
		return decodeQuotedPrintable(raw)
	}
	return decodeQuotedPrintable(raw.slice(separator + 4))
}

/** running is the sink this process shares, with the number of holders. */
let running: { sink: MailSink; holders: number } | undefined

/**
 * Starts the shared mail sink, or joins the one already listening.
 * @returns The listening sink, released by its own close.
 */
export async function startMailSink(): Promise<MailSink> {
	if (running) {
		running.holders += 1
		return running.sink
	}
	const sink = await listen()
	running = { sink, holders: 1 }
	return sink
}

/**
 * Starts one mail sink on the configured port.
 * @returns The listening sink.
 */
async function listen(): Promise<MailSink> {
	const held: { to: string; body: string }[] = []
	const server = new SMTPServer({
		authOptional: true,
		disabledCommands: ['STARTTLS', 'AUTH'],
		onData(stream, session, callback) {
			const chunks: Buffer[] = []
			stream.on('data', (chunk: Buffer) => chunks.push(chunk))
			stream.on('end', () => {
				const raw = Buffer.concat(chunks).toString('utf8')
				for (const recipient of session.envelope.rcptTo) {
					held.push({ to: recipient.address, body: bodyOf(raw) })
				}
				callback()
			})
		},
	})
	await new Promise<void>((resolve) => {
		server.listen(smtpPort, '127.0.0.1', resolve)
	})

	return {
		async waitFor(recipient: string): Promise<string> {
			const deadline = Date.now() + 10_000
			while (Date.now() < deadline) {
				const found = held.find((mail) => mail.to === recipient)
				if (found) {
					return found.body
				}
				await new Promise((resolve) => setTimeout(resolve, 50))
			}
			throw new Error(`no mail arrived for ${recipient}, the sink holds ${held.length}`)
		},
		count: () => held.length,
		clear: () => {
			held.length = 0
		},
		close: async () => {
			if (running && running.holders > 1) {
				running.holders -= 1
				return
			}
			running = undefined
			await new Promise<void>((resolve) => {
				server.close(() => resolve())
			})
		},
	}
}

/**
 * Returns the path and query of a link the mail body carries.
 * @param body - The decoded mail body.
 * @param path - The link path to find.
 * @returns The link, relative to the site.
 */
export function linkFrom(body: string, path: string): string {
	const marker = `${publicURL}${path}?token=`
	const at = body.indexOf(marker)
	if (at === -1) {
		throw new Error(`the mail carries no ${path} link: ${body}`)
	}
	const token = body.slice(at + marker.length).split(/\s/)[0]
	return `${path}?token=${token}`
}
