// SPDX-License-Identifier: AGPL-3.0-or-later

import { server } from '@alphone/frontend-sdk/testing'
import { screen } from '@testing-library/react'
import { beforeEach, expect, test } from 'vitest'

import { handlers } from '@alphone/plugin-whatsapp/handlers'
import { renderAt } from './render'

beforeEach(() => server.use(...handlers))

function canvas() {
	return document.querySelector('.godmin-layout__canvas')
}

test('pads the canvas for an ordinary screen', async () => {
	renderAt('/tasks')

	await screen.findByRole('heading', { name: 'Tasks' })

	expect(canvas()).not.toHaveClass('godmin-layout__canvas--bleed')
})

/**
 * Returns the value one design token takes on the canvas.
 * @param token - The custom property name.
 * @returns The value the nearest theme sets.
 */
function canvasToken(token: string): string {
	const themed = canvas()?.closest(`[style*="${token}:"]`) as HTMLElement
	return themed.style.getPropertyValue(token)
}

test('paints the canvas from the default WordPress palette, so its lines and greys match wp-admin', async () => {
	renderAt('/tasks')

	await screen.findByRole('heading', { name: 'Tasks' })

	expect(canvasToken('--wpds-color-background-surface-neutral-strong')).toBe('#fff')
	expect(canvasToken('--wpds-color-stroke-surface-neutral-weak')).toBe('#f0f0f0')
	expect(canvasToken('--wpds-color-stroke-interactive-neutral-strong')).toBe('#6e6e6e')
	expect(canvasToken('--wpds-color-stroke-interactive-neutral')).toBe('#8d8d8d')
	expect(canvasToken('--wpds-color-foreground-content-neutral-weak')).toBe('#707070')
})

test('paints the top bar and the rail in the WordPress admin bar grey, #1d2327 to within one step', async () => {
	renderAt('/tasks')

	await screen.findByRole('heading', { name: 'Tasks' })

	const token = '--wpds-color-background-surface-neutral-weak'
	const chrome = document.querySelector('.godmin-layout')?.closest(`[style*="${token}:"]`) as HTMLElement
	expect(chrome.style.getPropertyValue(token)).toBe('#1d2428')
})

test('lets a screen declare a full bleed canvas through its route', async () => {
	renderAt('/whatsapp/conversations/019f4a00-0000-7000-8000-000000000001')

	await screen.findByRole('log', { name: 'Messages' })

	expect(canvas()).toHaveClass('godmin-layout__canvas--bleed')
})
