// SPDX-License-Identifier: AGPL-3.0-or-later

import { useNavigate } from '@tanstack/react-router'

import { NewUserScreen } from './users/NewUserScreen'
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
