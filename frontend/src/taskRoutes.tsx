// SPDX-License-Identifier: AGPL-3.0-or-later

import { getRouteApi } from '@tanstack/react-router'

import { TasksScreen } from './tasks/TasksScreen'
import { isoDate, isValidDate } from './tasks/format'

const tasksRouteApi = getRouteApi('/tasks')

/**
 * Renders the task list for the day in the search params, defaulting to
 * today.
 * @returns The tasks route element.
 */
export function TasksRoute() {
	const { date } = tasksRouteApi.useSearch()
	const today = isoDate(new Date())
	return <TasksScreen date={date !== undefined && isValidDate(date) ? date : today} today={today} />
}
