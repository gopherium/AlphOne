// SPDX-License-Identifier: AGPL-3.0-or-later

import { Badge, Button, Stack, Text } from '@alphone/frontend-sdk'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'

import { useSession } from '../auth/session'
import { fetchUsers, setUserDisabled } from './api'
import type { User } from './api'

/**
 * Renders one user row with status and a disable toggle, hidden for the
 * signed-in account.
 * @param user - The user the row describes.
 * @param isSelf - Whether the row is the signed-in account.
 * @returns The table row element.
 */
function UserRow({ user, isSelf }: { user: User; isSelf: boolean }) {
	const queryClient = useQueryClient()
	const toggle = useMutation({
		mutationFn: () => setUserDisabled(user.id, !user.disabled),
		onSuccess: () => queryClient.invalidateQueries({ queryKey: ['users'] }),
	})

	return (
		<tr>
			<td>{user.name}</td>
			<td>{user.email}</td>
			<td>
				<Badge intent={user.disabled ? 'draft' : 'stable'}>
					{user.disabled ? 'Disabled' : 'Active'}
				</Badge>
			</td>
			<td>
				{isSelf ? null : (
					<Stack direction="column" gap="xs">
						<Button
							variant="outline"
							aria-label={`${user.disabled ? 'Enable' : 'Disable'} ${user.name}`}
							disabled={toggle.isPending}
							onClick={() => toggle.mutate()}
						>
							{user.disabled ? 'Enable' : 'Disable'}
						</Button>
						{toggle.isError ? <Text role="alert">Update failed.</Text> : null}
					</Stack>
				)}
			</td>
		</tr>
	)
}

/**
 * Renders the user administration screen: the account list with status and
 * disable controls, plus a link to create a new user.
 * @returns The users screen, or a loading or error message.
 */
export function UsersScreen() {
	const currentUserId = useSession().data?.id
	const users = useQuery({ queryKey: ['users'], queryFn: fetchUsers })

	if (users.isPending) {
		return <Text role="status">Loading users…</Text>
	}
	if (users.isError) {
		return <Text role="alert">Users could not be loaded.</Text>
	}
	return (
		<Stack direction="column" gap="lg">
			<Stack direction="row" align="center" gap="md">
				<Text variant="heading-lg" render={<h1 />}>
					Users
				</Text>
				<Link to="/users/new">
					<Button>New user</Button>
				</Link>
			</Stack>
			<table className="alphone-users">
				<thead>
					<tr>
						<th scope="col">Name</th>
						<th scope="col">Email</th>
						<th scope="col">Status</th>
						<th scope="col">
							<span className="alphone-users__actions-heading">Actions</span>
						</th>
					</tr>
				</thead>
				<tbody>
					{users.data.map((user) => (
						<UserRow
							key={user.id}
							user={user}
							isSelf={user.id === currentUserId}
						/>
					))}
				</tbody>
			</table>
		</Stack>
	)
}
