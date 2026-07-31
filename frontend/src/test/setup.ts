// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, http, installTestEnvironment, server } from '@alphone/frontend-sdk/testing'
import { beforeEach } from 'vitest'

installTestEnvironment()

beforeEach(() => {
	server.use(
		http.get('/api/tasks', () => HttpResponse.json({ tasks: [], next_cursor: null })),
	)
})
