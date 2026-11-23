// SPDX-License-Identifier: AGPL-3.0-or-later

import { Badge, Icon, InitialsAvatar, Text, __, file, formatNumber } from '@alphone/frontend-sdk'
import { listDate } from '@alphone/frontend-sdk/dataviews'
import type { Field, View } from '@alphone/frontend-sdk/dataviews'

import type { EmailInboxQuery } from './gql/graphql'
import { marksOf, previewOf, senderOf, subjectOf } from './words'

/** InboxThread is one thread as the inbox lists it. */
export type InboxThread = EmailInboxQuery['emailThreads']['items'][number]

/** InboxMail is the newest mail of a thread as the inbox reads it. */
type InboxMail = InboxThread['latest']

/** newestFirst is the order the server answers threads in, the only order the inbox shows. */
export const newestFirst = { field: 'date', direction: 'desc' } as const

/** subjectFills gives the subject column the width the other columns leave, in DataViews' own column style. */
const subjectFills = { subject: { width: '100%', maxWidth: 0 } }

/**
 * Returns the name of a thread's newest sender, beside a Sent badge when the workspace wrote that mail.
 * @param mail - The newest mail of the thread.
 * @returns The sender line.
 */
function senderLine(mail: InboxMail) {
	const sender = senderOf(mail)
	return (
		<span className="alphone-email-inbox__line">
			<span className="alphone-email-inbox__text" title={sender}>
				{sender}
			</span>
			{mail.direction === 'outbound' ? <Badge intent="none">{__('Sent', 'alphone-email')}</Badge> : null}
		</span>
	)
}

/**
 * Returns the address of a thread's newest sender in small body text, nothing when the sender line already shows it.
 * @param mail - The newest mail of the thread.
 * @returns The address line, or nothing for a sender shown by the address alone or a mail that names no sender.
 */
function addressLine(mail: InboxMail) {
	if (!mail.from || senderOf(mail) === mail.from.email) {
		return null
	}
	return (
		<Text
			variant="body-sm"
			className="alphone-email-inbox__text alphone-email-inbox__weak alphone-email-inbox__address"
			title={mail.from.email}
		>
			{mail.from.email}
		</Text>
	)
}

/**
 * Returns the badges of a thread's newest mail and a file glyph when it holds files, together in one box.
 * @param thread - The thread.
 * @returns The marks box, or nothing for a thread with no mark.
 */
function marksBox(thread: InboxThread) {
	const marks = marksOf(thread.latest)
	if (marks.length === 0 && !thread.hasAttachments) {
		return null
	}
	return (
		<span className="alphone-email-inbox__marks">
			{marks.map((mark) => (
				<Badge key={mark} intent="none">
					{mark}
				</Badge>
			))}
			{thread.hasAttachments ? (
				<span className="alphone-email-inbox__file" role="img" title={__('Has attachments', 'alphone-email')}>
					<Icon icon={file} size={16} />
				</span>
			) : null}
		</span>
	)
}

/**
 * Returns a thread's subject beside the marks of its newest mail.
 * @param thread - The thread.
 * @returns The subject line.
 */
function subjectLine(thread: InboxThread) {
	const subject = subjectOf(thread)
	return (
		<span className="alphone-email-inbox__line">
			<span className="alphone-email-inbox__text" title={subject}>
				{subject}
			</span>
			{marksBox(thread)}
		</span>
	)
}

/**
 * Returns a thread's subject line over the preview of its newest mail in small body text.
 * @param thread - The thread.
 * @returns The subject cell.
 */
function subjectCell(thread: InboxThread) {
	const preview = previewOf(thread.latest)
	return (
		<span className="alphone-email-inbox__cell">
			{subjectLine(thread)}
			<Text variant="body-sm" className="alphone-email-inbox__text alphone-email-inbox__weak" title={preview}>
				{preview}
			</Text>
		</span>
	)
}

/**
 * Builds the fields the inbox shows: avatar, sender, address, subject, count of messages and date of the newest mail.
 * @returns The fields, none of them sorted or filtered.
 */
export function inboxFields(): Field<InboxThread>[] {
	return [
		{
			id: 'avatar',
			label: __('Avatar', 'alphone-email'),
			render: ({ item }) => <InitialsAvatar name={item.latest.from?.name || item.latest.from?.email || ''} />,
			enableSorting: false,
			filterBy: false,
		},
		{
			id: 'from',
			label: __('From', 'alphone-email'),
			type: 'text',
			render: ({ item }) => senderLine(item.latest),
			enableHiding: false,
			enableSorting: false,
			filterBy: false,
		},
		{
			id: 'address',
			label: __('Email', 'alphone-email'),
			render: ({ item }) => addressLine(item.latest),
			enableHiding: false,
			enableSorting: false,
			filterBy: false,
		},
		{
			id: 'subjectLine',
			label: __('Subject', 'alphone-email'),
			render: ({ item }) => subjectLine(item),
			enableHiding: false,
			enableSorting: false,
			filterBy: false,
		},
		{
			id: 'subject',
			label: __('Subject', 'alphone-email'),
			render: ({ item }) => subjectCell(item),
			enableSorting: false,
			filterBy: false,
		},
		{
			id: 'messages',
			label: __('Messages', 'alphone-email'),
			type: 'integer',
			render: ({ item }) => formatNumber(item.messageCount),
			enableSorting: false,
			filterBy: false,
		},
		{
			id: 'date',
			label: __('Date', 'alphone-email'),
			render: ({ item }) => listDate(item.lastMessageAt),
			enableSorting: false,
			filterBy: false,
		},
	]
}

/**
 * Returns the view the list draws, in the server's order, for a narrow or a wide screen.
 * @param view - The view the address and the reader hold.
 * @param narrow - Whether the viewport sits at or below the medium breakpoint.
 * @returns The sender alone with the subject under it when narrow, the subject filling the free room when wide.
 */
export function drawnView(view: View, narrow: boolean): View {
	const ordered = { ...view, sort: newestFirst }
	if (narrow) {
		return { ...ordered, fields: [], descriptionField: 'subjectLine' }
	}
	return { ...ordered, layout: { ...ordered.layout, styles: subjectFills } } as View
}

/**
 * Returns a view changed on a wide screen without the column styles the screen draws, which the view never holds.
 * @param changed - The view as the list changed it.
 * @returns The changed view keeping the rest of its layout, such as the density.
 */
export function withoutDrawnStyles(changed: View): View {
	const layout = Object.fromEntries(Object.entries({ ...changed.layout }).filter(([key]) => key !== 'styles'))
	return { ...changed, layout } as View
}

/**
 * Returns a view changed on a narrow screen with the columns the reader held put back.
 * @param changed - The view as the list changed it.
 * @param held - The view the address and the reader hold.
 * @returns The changed view keeping the held columns and the line under the sender.
 */
export function withHeldColumns(changed: View, held: View): View {
	return { ...changed, fields: held.fields, descriptionField: held.descriptionField }
}
