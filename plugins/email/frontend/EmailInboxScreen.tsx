// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	ErrorNotice,
	ListEmpty,
	LoadingScreen,
	PageScreen,
	Stack,
	__,
	_x,
	caution,
	inbox,
	paginationOf,
	sprintf,
	useAdminSettings,
	useGraphQuery,
	useListView,
	useServerPaging,
} from '@alphone/frontend-sdk'
import type { AdminSettings, ListView, PagedView } from '@alphone/frontend-sdk'
import { DataViews } from '@alphone/frontend-sdk/dataviews'
import type { Field, View } from '@alphone/frontend-sdk/dataviews'
import { useMemo, useSyncExternalStore } from 'react'

import './email.css'
import type { EmailInboxQuery, EmailMailboxQuery } from './gql/graphql'
import { drawnView, inboxFields, newestFirst, withHeldColumns, withoutDrawnStyles } from './inboxFields'
import type { InboxThread } from './inboxFields'
import { inboxQuery, mailboxQuery } from './operations'

/** NARROW matches a viewport up to the medium breakpoint of 782px, where the inbox keeps the sender column alone. */
const NARROW = '(max-width: 782px)'

/** noThreads stands in for the list until the threads arrive. */
const noThreads: InboxThread[] = []

/** tableOnBothSides lays the inbox out as a table on either side of the phone breakpoint. */
const tableOnBothSides = { wide: 'table', phone: 'table' }

/**
 * Calls back each time the viewport crosses the medium breakpoint.
 * @param onChange - The callback.
 * @returns What stops the calls.
 */
function subscribeNarrow(onChange: () => void): () => void {
	const query = window.matchMedia(NARROW)
	query.addEventListener('change', onChange)
	return () => query.removeEventListener('change', onChange)
}

/**
 * Reports whether the viewport sits at or below the medium breakpoint now.
 * @returns Whether it does.
 */
function narrowNow(): boolean {
	return window.matchMedia(NARROW).matches
}

/**
 * Reports whether the viewport sits at or below the medium breakpoint, answering again each time it crosses it.
 * @returns Whether it does.
 */
function useNarrow(): boolean {
	return useSyncExternalStore(subscribeNarrow, narrowNow)
}

/** wideOnly names the columns only a wide screen draws. */
const wideOnly = new Set(['subject', 'messages', 'date'])

/**
 * Returns the fields a narrow screen offers, the ones its sender cell shows.
 * @param fields - The fields the inbox shows.
 * @returns The fields left once the subject, messages and date columns are taken out.
 */
function narrowFields(fields: Field<InboxThread>[]): Field<InboxThread>[] {
	return fields.filter((field) => !wideOnly.has(field.id))
}

/**
 * Returns the part of a view the server pages by, its page size only once the admin settings name the sizes.
 * @param view - The view the list shows.
 * @param settings - The admin settings, absent until they arrive or when they could not be read.
 * @returns The page alone while the settings are missing, the page and its size once they arrive.
 */
function pagedView(view: View, settings: AdminSettings | undefined): PagedView {
	return settings === undefined ? { page: view.page } : view
}

/**
 * Reads the page of threads a view stands for, stepping by the size the graph served.
 * @param view - The view the list shows.
 * @param settings - The admin settings, absent until they arrive or when they could not be read.
 * @param waiting - Whether the list still waits for the admin settings.
 * @returns The query result.
 */
function useInboxPage(view: View, settings: AdminSettings | undefined, waiting: boolean) {
	const paging = useServerPaging(pagedView(view, settings))
	const [result] = useGraphQuery({
		query: inboxQuery,
		variables: paging.window,
		pause: waiting,
		requestPolicy: 'cache-and-network',
	})
	paging.record(result.data?.emailThreads, result.operation?.variables.limit ?? null)
	return result
}

/**
 * Renders what the inbox shows while its mailbox holds no mail.
 * @returns The empty state.
 */
function NoMail() {
	return (
		<ListEmpty
			icon={inbox}
			title={__("You're set up. No mail yet.", 'alphone-email')}
			description={__('New mail will appear here.', 'alphone-email')}
		/>
	)
}

/**
 * Renders what the inbox shows while the workspace holds no mailbox, centred in a callout box in place of the list.
 * @returns The empty state in its box.
 */
function NoMailbox() {
	return (
		<Stack className="alphone-email-inbox__callout" align="center" justify="center">
			<ListEmpty
				icon={caution}
				title={__('No mailbox is connected', 'alphone-email')}
				description={__('Mail shows here once a mailbox is connected to this workspace.', 'alphone-email')}
			/>
		</Stack>
	)
}

/**
 * Renders the failure the inbox reports when its mail could not be read.
 * @returns The error notice.
 */
function MailFailed() {
	return <ErrorNotice>{__('Mail could not be loaded.', 'alphone-email')}</ErrorNotice>
}

/** InboxTableProps holds what the table of threads draws. */
interface InboxTableProps {
	/** list is the view the address and the reader hold, beside its controls. */
	list: ListView
	/** settings are the admin settings, absent until they arrive or when they could not be read. */
	settings: AdminSettings | undefined
	/** page is the page of threads the server served, absent until it arrives. */
	page: EmailInboxQuery['emailThreads'] | undefined
	/** loading tells whether the page is still on its way. */
	loading: boolean
}

/**
 * Renders a page of threads as a table under a header row holding the view options.
 * @param props - The view, the admin settings, the page and whether it loads.
 * @returns The table, with the sender column alone on a narrow screen.
 */
function InboxTable({ list, settings, page, loading }: InboxTableProps) {
	const narrow = useNarrow()
	const fields = useMemo(() => inboxFields(), [])
	const offered = useMemo(() => narrowFields(fields), [fields])
	return (
		<DataViews<InboxThread>
			data={page?.items ?? noThreads}
			paginationInfo={paginationOf(page)}
			fields={narrow ? offered : fields}
			view={drawnView(list.view, narrow)}
			onChangeView={(changed) =>
				list.onChangeView(narrow ? withHeldColumns(changed, list.view) : withoutDrawnStyles(changed))
			}
			defaultLayouts={list.defaultLayouts}
			isLoading={loading || page === undefined}
			config={settings === undefined ? undefined : { perPageSizes: settings.listPageSizes }}
			empty={<NoMail />}
		>
			<Stack className="alphone-email-inbox__header" justify="flex-end" align="center">
				<DataViews.ViewConfig />
			</Stack>
			<DataViews.Layout />
			<DataViews.Footer />
		</DataViews>
	)
}

/**
 * Reads the threads the view stands for once the admin settings name the page size, and draws them.
 * @returns The table, or the failure when the threads could not be read.
 */
function InboxList() {
	const { settings, failed } = useAdminSettings()
	const list = useListView({
		fields: ['subject', 'messages', 'date'],
		titleField: 'from',
		descriptionField: 'address',
		mediaField: 'avatar',
		sort: newestFirst,
		perPage: settings?.listPageSize,
		layouts: tableOnBothSides,
	})
	const waiting = settings === undefined && !failed
	const result = useInboxPage(list.view, settings, waiting)
	if (result.error) {
		return <MailFailed />
	}
	return (
		<InboxTable list={list} settings={settings} page={result.data?.emailThreads} loading={waiting || result.fetching} />
	)
}

/**
 * Renders the inbox's content: the failure, a spinner, the state with no mailbox, or the list of threads.
 * @param props - Whether the mailbox read failed, and the mailbox read, absent until it arrives.
 * @returns The content, or the spinner until the mailbox read answers.
 */
function InboxContent({ failed, mailbox }: { failed: boolean; mailbox: EmailMailboxQuery | undefined }) {
	if (failed) {
		return <MailFailed />
	}
	if (mailbox === undefined) {
		return <LoadingScreen label={__('Loading mail…', 'alphone-email')} />
	}
	if (mailbox.emailMailbox === null) {
		return <NoMailbox />
	}
	return <InboxList />
}

/**
 * Returns the sentence under the title naming the mailbox the inbox reads.
 * @param mailbox - The mailbox read, absent until it arrives.
 * @returns The sentence, or nothing while no mailbox is known.
 */
function subtitleOf(mailbox: EmailMailboxQuery | undefined): string | undefined {
	const address = mailbox?.emailMailbox?.address
	if (address === undefined) {
		return undefined
	}
	return sprintf(__('Read the mail of %(address)s in one place.', 'alphone-email'), { address })
}

/**
 * Renders the workspace's mail as a list of threads, the newest first.
 * @returns The inbox screen.
 */
export function EmailInboxScreen() {
	const [mailbox] = useGraphQuery({ query: mailboxQuery, requestPolicy: 'cache-and-network' })

	return (
		<PageScreen
			className="alphone-email-inbox"
			title={_x('Email', 'admin section', 'alphone-email')}
			headingLevel={2}
			subtitle={subtitleOf(mailbox.data)}
			list
			listLabel={__('Inbox', 'alphone-email')}
		>
			<InboxContent failed={mailbox.error !== undefined} mailbox={mailbox.data} />
		</PageScreen>
	)
}
