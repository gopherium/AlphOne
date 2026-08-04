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

test('lets a screen declare a full bleed canvas through its route', async () => {
	renderAt('/whatsapp/conversations/019f4a00-0000-7000-8000-000000000001')

	await screen.findByRole('log', { name: 'Messages' })

	expect(canvas()).toHaveClass('godmin-layout__canvas--bleed')
})
