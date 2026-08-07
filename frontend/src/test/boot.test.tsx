// SPDX-License-Identifier: AGPL-3.0-or-later

import { render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import { BootLoading } from '../boot'

const sources = import.meta.glob('../main.tsx', {
	query: '?raw',
	import: 'default',
	eager: true,
}) as Record<string, string>

test('ghosts the screen while the session loads', () => {
	render(<BootLoading />)

	const status = screen.getByRole('status')
	expect(status).toHaveTextContent('Loading…')
	expect(status.closest('.godmin-loading-screen')).not.toBeNull()
})

test('the entry hands that ghost to the auth gate', () => {
	expect(sources['../main.tsx']).toMatch(/loading=\{<BootLoading \/>\}/)
})
