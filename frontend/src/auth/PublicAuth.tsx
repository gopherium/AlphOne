// SPDX-License-Identifier: AGPL-3.0-or-later

import type { User } from '@gopherium/react-auth'
import {
	LoginScreen,
	RequestResetScreen,
	ResetPasswordScreen,
	SetPasswordScreen,
} from '@gopherium/react-auth/wpds'
import { useState } from 'react'
import type { ReactNode } from 'react'

/** BRAND is the product name the account screens carry. */
const BRAND = 'AlphOne'

/** The paths a mailed account link lands on. */
const ACTIVATE_PATH = '/activate'
const RESET_PATH = '/reset-password'

/**
 * Returns the screen a mailed account link asks for, or null for every other path.
 * @param url - The address the reader arrived at.
 * @param onDone - Called once the link is spent and the reader moves on.
 * @returns The screen element, or null when the path carries no link.
 */
export function publicAuthScreen(url: URL, onDone: () => void): ReactNode | null {
	const token = url.searchParams.get('token') ?? ''
	if (url.pathname === ACTIVATE_PATH) {
		return <SetPasswordScreen brand={BRAND} token={token} onAccepted={onDone} />
	}
	if (url.pathname === RESET_PATH) {
		return <ResetPasswordScreen brand={BRAND} token={token} onDone={onDone} />
	}
	return null
}

/**
 * Renders the login, swapping to the reset request while the reader asks for a link.
 * @param onLogin - Called with the user after a successful login.
 * @returns The login slot element.
 */
export function LoginSlot({ onLogin }: { onLogin: (user: User) => void | Promise<void> }) {
	const [asking, setAsking] = useState(false)

	if (asking) {
		return <RequestResetScreen brand={BRAND} onBack={() => setAsking(false)} />
	}
	return (
		<LoginScreen brand={BRAND} onLogin={onLogin} onForgotPassword={() => setAsking(true)} />
	)
}
