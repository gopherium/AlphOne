// SPDX-License-Identifier: AGPL-3.0-or-later

import { createRoute } from '@tanstack/react-router'
import { fireEvent, render, screen } from '@testing-library/react'
import { expect, test, vi } from 'vitest'

import { Button, Toaster, useToaster } from '../index'
import type { FrontendPlugin, ToastAction, ToasterHandle } from '../index'
import { renderPluginAt } from '../testing'

/** Raises one toast carrying the given action when its button is pressed. */
function Raiser({ action }: { action: ToastAction }) {
	const toaster: ToasterHandle = useToaster()
	return <Button onClick={() => toaster.show('Note saved.', action)}>Save note</Button>
}

test('hands plugins the toaster and the hook raising a toast in it', async () => {
	const onAct = vi.fn()

	render(
		<Toaster dismissLabel="Close">
			<Raiser action={{ label: 'Undo', onAct }} />
		</Toaster>,
	)
	fireEvent.click(screen.getByRole('button', { name: 'Save note' }))
	fireEvent.click(await screen.findByRole('button', { name: 'Undo' }))

	expect(onAct).toHaveBeenCalledTimes(1)
})

test('mounts a toaster above the screens of a plugin under test', async () => {
	const plugin: FrontendPlugin = {
		id: 'notes',
		nav: [],
		routes: (parent) => [
			createRoute({
				getParentRoute: () => parent,
				path: '/notes',
				component: function NotesScreen() {
					return <Raiser action={{ label: 'Undo', onAct: () => {} }} />
				},
			}),
		],
	}

	renderPluginAt(plugin, '/notes')
	fireEvent.click(await screen.findByRole('button', { name: 'Save note' }))

	expect(await screen.findByText('Note saved.')).toBeInTheDocument()
})
