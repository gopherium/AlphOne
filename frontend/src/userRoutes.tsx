// SPDX-License-Identifier: AGPL-3.0-or-later

import { NewUserScreen, UsersScreen } from '@gopherium/react-auth/wpds'
import { Link, useNavigate } from '@tanstack/react-router'

/**
 * Renders the shared users screen wired to the app's new-user route.
 * @returns The users screen element.
 */
export function UsersRoute() {
	return <UsersScreen newUserRender={<Link to="/users/new" />} />
}

/**
 * Renders the shared new-user form, returning to the user list on success.
 * @returns The new user screen element.
 */
export function NewUserRoute() {
	const navigate = useNavigate()
	return <NewUserScreen onCreated={() => navigate({ to: '/users' })} />
}
