// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	ErrorNotice,
	InputControl,
	PageScreen,
	Stack,
	Text,
	__,
	graphError,
	sprintf,
	useGraphMutation,
	validationMessage,
} from '@alphone/frontend-sdk'
import { useState } from 'react'

import {
	defaultLifetime,
	grantableAreas,
	lifetimeChoices,
	lifetimeDays,
} from './tokenFormat'
import { apiTokenCreateMutation } from './tokenOperations'

/** grants is the access a token may be granted in one area. */
type Grants = Record<string, 'read' | 'write' | undefined>

/**
 * Returns the scopes the ticked areas ask for.
 * @param grants - The access ticked per area.
 * @returns The scope strings, sorted by area.
 */
function scopesOf(grants: Grants): string[] {
	return grantableAreas
		.filter((area) => grants[area] !== undefined)
		.map((area) => `${area}:${grants[area]}`)
}

/**
 * Renders the checkbox granting one access in one area.
 * @returns The checkbox element.
 */
function GrantBox({
	area,
	access,
	grants,
	onGrant,
}: {
	area: string
	access: 'read' | 'write'
	grants: Grants
	onGrant: (grants: Grants) => void
}) {
	const label = sprintf(
		access === 'read' ? __('Read %(area)s', 'alphone') : __('Write %(area)s', 'alphone'),
		{ area },
	)
	return (
		<label>
			<input
				type="checkbox"
				aria-label={label}
				checked={grants[area] === access}
				onChange={(event) =>
					onGrant({ ...grants, [area]: event.target.checked ? access : undefined })
				}
			/>
			{label}
		</label>
	)
}

/**
 * Renders the minted secret with the control copying it.
 * @returns The secret panel.
 */
function MintedSecret({ secret }: { secret: string }) {
	const [copied, setCopied] = useState<boolean | null>(null)
	const copy = async () => {
		try {
			await navigator.clipboard.writeText(secret)
			setCopied(true)
		} catch {
			setCopied(false)
		}
	}

	return (
		<Stack direction="column" gap="sm">
			<Text>{__('Copy the secret now, it is never shown again.', 'alphone')}</Text>
			<code>{secret}</code>
			<Button variant="outline" aria-label={__('Copy secret', 'alphone')} onClick={() => void copy()}>
				{__('Copy secret', 'alphone')}
			</Button>
			{copied === true ? <Text role="status">{__('Copied.', 'alphone')}</Text> : null}
			{copied === false ? <Text role="alert">{__('Copy it by hand.', 'alphone')}</Text> : null}
		</Stack>
	)
}

/**
 * Renders the form minting a scoped token, showing its secret exactly once.
 * @returns The new token screen.
 */
export function NewTokenScreen() {
	const [name, setName] = useState('')
	const [grants, setGrants] = useState<Grants>({})
	const [lifetime, setLifetime] = useState(defaultLifetime)
	const [secret, setSecret] = useState<string | null>(null)
	const [mint, runMint] = useGraphMutation(apiTokenCreateMutation)
	const scopes = scopesOf(grants)
	const submit = async () => {
		const result = await runMint({ name, scopes, ttlDays: lifetimeDays(lifetime) })
		if (result.data) {
			setSecret(result.data.apiTokenCreate.secret)
		}
	}

	if (secret !== null) {
		return (
			<PageScreen title={__('New API token', 'alphone')}>
				<MintedSecret secret={secret} />
			</PageScreen>
		)
	}
	return (
		<PageScreen title={__('New API token', 'alphone')}>
			<form
				className="godmin-form"
				onSubmit={(event) => {
					event.preventDefault()
					void submit()
				}}
			>
				<InputControl
					label={__('Name', 'alphone')}
					value={name}
					onChange={(event) => setName(event.target.value)}
				/>
				<fieldset>
					<legend>{__('Areas', 'alphone')}</legend>
					{grantableAreas.map((area) => (
						<Stack key={area} direction="row" gap="sm">
							<GrantBox area={area} access="read" grants={grants} onGrant={setGrants} />
							<GrantBox area={area} access="write" grants={grants} onGrant={setGrants} />
						</Stack>
					))}
				</fieldset>
				<label>
					{__('Expires', 'alphone')}
					<select value={lifetime} onChange={(event) => setLifetime(event.target.value)}>
						{lifetimeChoices().map((choice) => (
							<option key={choice.value} value={choice.value}>
								{choice.label}
							</option>
						))}
					</select>
				</label>
				<Button
					type="submit"
					disabled={name.trim() === '' || scopes.length === 0 || mint.fetching}
					loading={mint.fetching}
				>
					{__('Create token', 'alphone')}
				</Button>
				{mint.error ? (
					<ErrorNotice>
						{validationMessage(graphError(mint.error), __('The token could not be created.', 'alphone'))}
					</ErrorNotice>
				) : null}
			</form>
		</PageScreen>
	)
}
