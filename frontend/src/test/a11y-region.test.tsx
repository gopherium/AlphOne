// SPDX-License-Identifier: AGPL-3.0-or-later

import { render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

test('queries ignore the a11y live region duplicating announced text', () => {
	render(
		<div>
			<p>Task saved</p>
			<div id="a11y-speak-polite" className="a11y-speak-region">
				Task saved
			</div>
		</div>,
	)

	expect(screen.getByText('Task saved')).toBeInTheDocument()
})

test('queries ignore the a11y intro text labelling the live region', () => {
	render(
		<div>
			<p>Notifications</p>
			<p id="a11y-speak-intro-text" className="a11y-speak-intro-text">
				Notifications
			</p>
		</div>,
	)

	expect(screen.getByText('Notifications')).toBeInTheDocument()
})
