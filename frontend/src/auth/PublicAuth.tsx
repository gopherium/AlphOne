// SPDX-License-Identifier: AGPL-3.0-or-later

import { __ } from '@alphone/frontend-sdk'
import type { User } from '@gopherium/react-auth'
import {
	LoginScreen,
	RequestResetScreen,
	ResetPasswordScreen,
	SetPasswordScreen,
} from '@gopherium/react-auth/wpds'
import type { AuthLink } from '@gopherium/react-auth/wpds'
import { useState } from 'react'
import type { ReactNode } from 'react'

import type { LegalAddresses } from '../i18n/api'

/** BRAND is the product name the account screens carry. */
const BRAND = 'AlphOne'

/** The paths a mailed account link lands on. */
const ACTIVATE_PATH = '/activate'
const RESET_PATH = '/reset-password'

/**
 * Returns the links under the account screens, the terms first, one per address set.
 * @param addresses - The legal pages the install publishes.
 * @returns The links, none when no address is set.
 */
export function legalLinks({ privacyUrl, termsUrl }: LegalAddresses): AuthLink[] {
	return [
		{ label: __('Terms of service', 'alphone'), href: termsUrl, rel: 'terms-of-service' },
		{ label: __('Privacy policy', 'alphone'), href: privacyUrl, rel: 'privacy-policy' },
	].filter((link) => link.href !== '')
}

/**
 * Returns the screen a mailed account link asks for, or null for every other path.
 * @param url - The address the reader arrived at.
 * @param links - The links under the form.
 * @param onDone - Called once the link is spent and the reader moves on.
 * @returns The screen element, or null when the path carries no link.
 */
export function publicAuthScreen(url: URL, links: readonly AuthLink[], onDone: () => void): ReactNode | null {
	const token = url.searchParams.get('token') ?? ''
	if (url.pathname === ACTIVATE_PATH) {
		return <SetPasswordScreen brand={BRAND} token={token} onAccepted={onDone} links={links} />
	}
	if (url.pathname === RESET_PATH) {
		return <ResetPasswordScreen brand={BRAND} token={token} onDone={onDone} links={links} />
	}
	return null
}

/**
 * Renders the login, swapping to the reset request while the reader asks for a link.
 * @param links - The links under the form.
 * @param onLogin - Called with the user after a successful login.
 * @returns The login slot element.
 */
export function LoginSlot({
	links,
	onLogin,
}: {
	links: readonly AuthLink[]
	onLogin: (user: User) => void | Promise<void>
}) {
	const [asking, setAsking] = useState(false)

	if (asking) {
		return <RequestResetScreen brand={BRAND} onBack={() => setAsking(false)} links={links} />
	}
	return (
		<LoginScreen brand={BRAND} onLogin={onLogin} onForgotPassword={() => setAsking(true)} links={links} />
	)
}
