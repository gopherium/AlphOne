// SPDX-License-Identifier: AGPL-3.0-or-later

import { graphql, server } from '@alphone/frontend-sdk/testing'
import { screen, waitFor } from '@testing-library/react'
import { expect, test, vi } from 'vitest'

import { renderAt } from './render'

const panelChunkRequested = vi.hoisted(() => vi.fn())

vi.mock('../../../plugins/fields/frontend/ContactFieldsPanel', async (importOriginal) => {
	panelChunkRequested()
	return importOriginal()
})

test('starts the contributed contact panels loading while the contact is on its way', async () => {
	server.use(graphql.query('ContactDetail', () => new Promise(() => {})))
	renderAt('/contacts/0198c000-0000-7000-8000-000000000001')

	expect(await screen.findByRole('status')).toHaveTextContent('Loading contact…')
	await waitFor(() => expect(panelChunkRequested).toHaveBeenCalledOnce())
	expect(screen.getByRole('status')).toHaveTextContent('Loading contact…')
})
