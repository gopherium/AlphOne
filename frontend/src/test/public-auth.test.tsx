// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { resetLocale } from '@gopherium/gottext/testing'
import { createAuthQueryClient } from '@gopherium/react-auth'
import { activateOk, resetOk } from '@gopherium/react-auth/testing'
import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { LoginSlot, legalLinks, publicAuthScreen } from '../auth/PublicAuth'
import { startAppLocale } from '../i18n/start'

/** LEGAL holds both addresses an install can publish. */
const LEGAL = {
	privacyUrl: 'https://example.com/legal/privacy',
	termsUrl: 'https://example.com/legal/terms',
}

/** ACTIVATE_URL is a mailed activation link. */
const ACTIVATE_URL = new URL('https://crm.example.com/activate?token=t-1')

/** RESET_URL is a mailed password reset link. */
const RESET_URL = new URL('https://crm.example.com/reset-password?token=t-2')

afterEach(() => {
	resetLocale()
})

/**
 * Renders one element under a fresh auth query client.
 */
function renderScreen(node: ReactNode) {
	render(
		<QueryClientProvider client={createAuthQueryClient({ mutations: { retry: false } })}>
			{node}
		</QueryClientProvider>,
	)
}

/**
 * Expects both legal links on screen, each with its address and its relation.
 */
async function expectLegalLinks() {
	const terms = await screen.findByRole('link', { name: 'Terms of service' })
	expect(terms).toHaveAttribute('href', LEGAL.termsUrl)
	expect(terms).toHaveAttribute('rel', 'noopener noreferrer terms-of-service')
	const privacy = screen.getByRole('link', { name: 'Privacy policy' })
	expect(privacy).toHaveAttribute('href', LEGAL.privacyUrl)
	expect(privacy).toHaveAttribute('rel', 'noopener noreferrer privacy-policy')
}

test('an activation link renders the set password screen', async () => {
	const held = publicAuthScreen(ACTIVATE_URL, [], vi.fn())

	expect(held).not.toBeNull()
	renderScreen(held)

	expect(await screen.findByLabelText('Password')).toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Set password' })).toBeInTheDocument()
})

test('a reset link renders the reset password screen', async () => {
	const held = publicAuthScreen(RESET_URL, [], vi.fn())

	expect(held).not.toBeNull()
	renderScreen(held)

	expect(await screen.findByLabelText('Password')).toBeInTheDocument()
})

test('every other path renders no public screen', () => {
	for (const path of ['/', '/tasks', '/users', '/users/new', '/activateX']) {
		expect(publicAuthScreen(new URL(`https://crm.example.com${path}`), [], vi.fn())).toBeNull()
	}
})

test('the activation link hands its token to the screen and reports the way on', async () => {
	server.use(activateOk())
	const onDone = vi.fn()
	renderScreen(publicAuthScreen(ACTIVATE_URL, [], onDone))

	await userEvent.type(await screen.findByLabelText('Password'), 'correct horse battery')
	await userEvent.click(screen.getByRole('button', { name: 'Set password' }))

	await waitFor(() => expect(onDone).toHaveBeenCalled())
})

test('the reset link reports the way on once the password is replaced', async () => {
	server.use(resetOk())
	const onDone = vi.fn()
	renderScreen(publicAuthScreen(RESET_URL, [], onDone))

	await userEvent.type(await screen.findByLabelText('Password'), 'brand new password')
	await userEvent.click(screen.getByRole('button', { name: 'Set password' }))

	await userEvent.click(await screen.findByRole('button', { name: 'Go to login' }))
	await waitFor(() => expect(onDone).toHaveBeenCalled())
})

test('a link carrying no token still renders its screen', async () => {
	renderScreen(publicAuthScreen(new URL('https://crm.example.com/activate'), [], vi.fn()))

	expect(await screen.findByLabelText('Password')).toBeInTheDocument()
})

test('the login slot offers the way to a password reset and back', async () => {
	renderScreen(<LoginSlot links={[]} onLogin={vi.fn()} />)

	await userEvent.click(await screen.findByRole('button', { name: 'Lost your password?' }))

	expect(await screen.findByRole('button', { name: 'Send reset link' })).toBeInTheDocument()

	await userEvent.click(screen.getByRole('button', { name: 'Back to login' }))

	expect(await screen.findByRole('button', { name: 'Log in' })).toBeInTheDocument()
})

test('the legal links put the terms first and name each page with its relation', () => {
	expect(legalLinks(LEGAL)).toEqual([
		{ label: 'Terms of service', href: LEGAL.termsUrl, rel: 'terms-of-service' },
		{ label: 'Privacy policy', href: LEGAL.privacyUrl, rel: 'privacy-policy' },
	])
})

test('an install that set only the terms gets only that link', () => {
	expect(legalLinks({ ...LEGAL, privacyUrl: '' }).map((link) => link.href)).toEqual([LEGAL.termsUrl])
})

test('an install that set only the privacy policy gets only that link', () => {
	expect(legalLinks({ ...LEGAL, termsUrl: '' }).map((link) => link.href)).toEqual([LEGAL.privacyUrl])
})

test('an install that set neither address gets no links', () => {
	expect(legalLinks({ privacyUrl: '', termsUrl: '' })).toEqual([])
})

test('a boot read that fails gives no links', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.error()))

	const { legal } = await startAppLocale()

	expect(legalLinks(legal)).toEqual([])
})

test('the legal links read in the language the graph resolves', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale: 'es-ES' } })))

	await startAppLocale()

	expect(legalLinks(LEGAL).map((link) => link.label)).toEqual([
		'Condiciones del servicio',
		'Política de privacidad',
	])
})

test('the set password screen shows the links', async () => {
	renderScreen(publicAuthScreen(ACTIVATE_URL, legalLinks(LEGAL), vi.fn()))

	await expectLegalLinks()
})

test('the reset password screen shows the links', async () => {
	renderScreen(publicAuthScreen(RESET_URL, legalLinks(LEGAL), vi.fn()))

	await expectLegalLinks()
})

test('the login and the reset request both show the links', async () => {
	renderScreen(<LoginSlot links={legalLinks(LEGAL)} onLogin={vi.fn()} />)

	await expectLegalLinks()

	await userEvent.click(screen.getByRole('button', { name: 'Lost your password?' }))
	await screen.findByRole('button', { name: 'Send reset link' })

	await expectLegalLinks()
})

test('a screen given no links shows none', async () => {
	renderScreen(<LoginSlot links={[]} onLogin={vi.fn()} />)

	await screen.findByRole('button', { name: 'Log in' })

	expect(screen.queryByRole('link')).not.toBeInTheDocument()
})
