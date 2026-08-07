// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, installTestEnvironment, server } from '@alphone/frontend-sdk/testing'
import { beforeEach } from 'vitest'

installTestEnvironment()

/** emptyTaskPage is the connection every screen sees before a test says otherwise. */
const emptyTaskPage = {
	__typename: 'TaskConnection',
	edges: [],
	pageInfo: { __typename: 'PageInfo', hasNextPage: false, endCursor: null },
}

beforeEach(() => {
	server.use(
		graphql.query('DayTasks', () => HttpResponse.json({ data: { tasks: emptyTaskPage } })),
		graphql.query('OverdueTasks', () => HttpResponse.json({ data: { tasks: emptyTaskPage } })),
	)
})
