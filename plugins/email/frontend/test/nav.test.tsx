// SPDX-License-Identifier: AGPL-3.0-or-later

import { envelope } from '@alphone/frontend-sdk'
import { adminSession, renderPluginAt, resetLocaleData, setLocaleData } from '@alphone/frontend-sdk/testing'
import { render, screen, within } from '@testing-library/react'
import { expect, onTestFinished, test } from 'vitest'

import { DOMAIN, plugin } from '../index'

/**
 * Returns the sidebar the host draws.
 * @returns The region named Navigation.
 */
function sidebar(): Promise<HTMLElement> {
	return screen.findByRole('region', { name: 'Navigation' })
}

/**
 * Returns the markup a glyph draws inside its svg.
 * @param held - The element holding the glyph.
 * @returns The markup.
 */
function drawingIn(held: Element): string | undefined {
	return held.querySelector('svg')?.innerHTML
}

test('shows an Email row in the menu that leads to the inbox', async () => {
	renderPluginAt(plugin, '/', { session: adminSession })

	const row = await within(await sidebar()).findByRole('link', { name: 'Email' })
	expect(row).toHaveAttribute('href', '/email')
})

test('draws the envelope glyph on the Email row', async () => {
	renderPluginAt(plugin, '/', { session: adminSession })

	const row = await within(await sidebar()).findByRole('link', { name: 'Email' })
	expect(drawingIn(row)).toBe(drawingIn(render(envelope).container))
})

test('names the Email row in Spanish once its catalogue loads', async () => {
	setLocaleData(await plugin.locale?.load('es-ES'), DOMAIN)
	onTestFinished(() => resetLocaleData(undefined, DOMAIN))
	renderPluginAt(plugin, '/', { session: adminSession })

	const row = await within(await sidebar()).findByRole('link', { name: 'Correo electrónico' })
	expect(row).toHaveAttribute('href', '/email')
})
