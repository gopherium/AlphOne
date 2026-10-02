// SPDX-License-Identifier: AGPL-3.0-or-later

import { rememberFormatLocale } from '@alphone/frontend-sdk'
import { HttpResponse, graphql, installTestEnvironment, server } from '@alphone/frontend-sdk/testing'
import { beforeEach } from 'vitest'

installTestEnvironment()

/** emptyTaskPage is the connection every screen sees before a test says otherwise. */
const emptyTaskPage = {
	__typename: 'TaskConnection',
	edges: [],
	pageInfo: { __typename: 'PageInfo', hasNextPage: false, endCursor: null },
}

/** adminSettings are the settings every screen reads before a test says otherwise. */
const adminSettings = {
	__typename: 'AdminSettings',
	toastMilliseconds: 6000,
	listPageSizes: [10, 20, 50, 100],
	listPageSize: 20,
	contactPageCap: 200,
	formatLocale: 'es-ES',
}

beforeEach(() => {
	rememberFormatLocale(adminSettings.formatLocale)
	server.use(
		graphql.query('DayTasks', () => HttpResponse.json({ data: { tasks: emptyTaskPage } })),
		graphql.query('OverdueTasks', () => HttpResponse.json({ data: { tasks: emptyTaskPage } })),
		graphql.query('AdminSettings', () => HttpResponse.json({ data: { adminSettings } })),
	)
})
