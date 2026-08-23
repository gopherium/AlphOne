// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	ErrorNotice,
	LoadingRows,
	PageScreen,
	SelectControl,
	Stack,
	Text,
	__,
	displayLocale,
	graphError,
	useGraphMutation,
	useGraphQuery,
} from '@alphone/frontend-sdk'
import { useState } from 'react'

import { setLocaleMutation, supportedLocalesQuery } from './localeOperations'

/**
 * Renders the screen where a reader picks the language the interface speaks.
 * @returns The language screen element.
 */
export function LanguageScreen() {
	const [asked] = useGraphQuery({ query: supportedLocalesQuery })
	const [, runSetLocale] = useGraphMutation(setLocaleMutation)
	const [chosen, setChosen] = useState(displayLocale())
	const [saved, setSaved] = useState(false)
	const [notice, setNotice] = useState('')
	if (asked.error !== undefined) {
		return (
			<PageScreen title={__('Language', 'alphone')}>
				<ErrorNotice>{__('The languages could not be read.', 'alphone')}</ErrorNotice>
			</PageScreen>
		)
	}
	if (asked.data === undefined) {
		return (
			<PageScreen title={__('Language', 'alphone')}>
				<LoadingRows label={__('Reading the languages', 'alphone')} />
			</PageScreen>
		)
	}
	const offered = asked.data.supportedLocales.map((locale) => ({ label: locale, value: locale }))
	const choose = (picked: string) => {
		setSaved(false)
		setChosen(picked)
	}
	const submit = async () => {
		const answered = await runSetLocale({ locale: chosen })
		if (answered.data) {
			setNotice('')
			setSaved(true)
			return
		}
		setSaved(false)
		setNotice(graphError(answered.error)?.message ?? __('The choice could not be saved.', 'alphone'))
	}
	return (
		<PageScreen title={__('Language', 'alphone')}>
			<Stack direction="column" gap="sm">
				<SelectControl
					label={__('Language', 'alphone')}
					value={offered.find((option) => option.value === chosen)}
					items={offered}
					onValueChange={(item) => item?.value != null && choose(item.value)}
				/>
				<Button onClick={() => void submit()}>{__('Save', 'alphone')}</Button>
				{saved && <Text role="status">{__('The language changes when the page next loads.', 'alphone')}</Text>}
				{notice !== '' && <ErrorNotice>{notice}</ErrorNotice>}
			</Stack>
		</PageScreen>
	)
}
