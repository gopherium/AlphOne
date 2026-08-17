// SPDX-License-Identifier: AGPL-3.0-or-later

import { useNavigate } from '@tanstack/react-router'

import { NewTokenScreen } from './users/NewTokenScreen'
import { NewUserScreen } from './users/NewUserScreen'
import { TokensScreen } from './users/TokensScreen'
import { UsersScreen } from './users/UsersScreen'

/**
 * Renders the users screen.
 * @returns The users screen element.
 */
export function UsersRoute() {
	return <UsersScreen />
}

/**
 * Renders the new user form, returning to the user list on success.
 * @returns The new user screen element.
 */
export function NewUserRoute() {
	const navigate = useNavigate()
	return <NewUserScreen onCreated={() => navigate({ to: '/users' })} />
}

/**
 * Renders the API tokens screen.
 * @returns The tokens screen element.
 */
export function TokensRoute() {
	return <TokensScreen />
}

/**
 * Renders the new API token form.
 * @returns The new token screen element.
 */
export function NewTokenRoute() {
	return <NewTokenScreen />
}
