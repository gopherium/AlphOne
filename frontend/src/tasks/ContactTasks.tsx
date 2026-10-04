// SPDX-License-Identifier: AGPL-3.0-or-later

import { Button, ErrorNotice, SectionTitle, Text, __ } from '@alphone/frontend-sdk'
import type { ConnectionResult } from '@alphone/frontend-sdk'

import { isoDate } from './format'
import { QuickAddForm } from './QuickAddForm'
import { TaskList } from './TaskList'
import type { RowControls, ListedTask } from './TaskList'
import { useRowControls } from './useRowControls'

/**
 * Renders a contact's open tasks and the form that adds one.
 * @returns The contact tasks section.
 */
export function ContactTasks({
	contactId,
	tasks,
}: {
	contactId: string
	tasks: ConnectionResult<ListedTask>
}) {
	const today = isoDate(new Date())
	const { controls, failed } = useRowControls(today)

	return (
		<div className="alphone-tasks__contact-block">
			<SectionTitle>{__('Tasks', 'alphone')}</SectionTitle>
			<QuickAddForm day={today} contactId={contactId} className="alphone-tasks__add--contact" />
			{failed ? <ErrorNotice>{__('The task could not be updated.', 'alphone')}</ErrorNotice> : null}
			<ContactTaskList tasks={tasks} controls={controls} />
		</div>
	)
}

/**
 * Renders the empty and loaded states of a contact's tasks.
 * @returns The contact task list.
 */
function ContactTaskList({
	tasks,
	controls,
}: {
	tasks: ConnectionResult<ListedTask>
	controls: RowControls
}) {
	const rows = tasks.rows
	if (rows.length === 0) {
		return <Text role="status">{__('No open tasks.', 'alphone')}</Text>
	}
	return (
		<>
			<TaskList label={__('Contact tasks', 'alphone')} tasks={rows} controls={controls} showDueDate />
			{tasks.hasNextPage ? (
				<Button onClick={() => void tasks.fetchNextPage()} disabled={tasks.isFetchingNextPage}>
					{__('Load more tasks', 'alphone')}
				</Button>
			) : null}
		</>
	)
}
