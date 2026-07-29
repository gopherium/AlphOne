// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, http, installTestEnvironment, server } from '@alphone/frontend-sdk/testing'
import { configure } from '@testing-library/react'
import { beforeEach } from 'vitest'

installTestEnvironment()

configure({ defaultIgnore: 'script, style, .a11y-speak-region' })

beforeEach(() => {
	server.use(
		http.get('/api/tasks', () => HttpResponse.json({ tasks: [], next_cursor: null })),
	)
})
