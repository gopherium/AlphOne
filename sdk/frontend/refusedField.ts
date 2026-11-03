// SPDX-License-Identifier: AGPL-3.0-or-later

import { speak } from '@wordpress/a11y'
import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { RefObject } from 'react'

import { validationMessage } from './errors'
import { graphError } from './graph'

/**
 * Shows the message of each field of a form that fails, and speaks the one in a field that already held the focus.
 * @param form - The form holding the fields.
 */
function reportFields(form: HTMLFormElement | null) {
	const before = document.activeElement
	form?.reportValidity()
	if (before === document.activeElement && before instanceof HTMLInputElement) {
		speak(before.validationMessage, 'assertive')
	}
}

/**
 * Holds what the server said about one field, shown under it until the field holds another value.
 * @param form - The form the field sits in.
 * @param held - The value the field holds now.
 * @returns The custom validity the field takes, the way to refuse a sent value and the way to forget one.
 */
export function useRefusedField(form: RefObject<HTMLFormElement | null>, held: string) {
	const [refused, setRefused] = useState<{ message: string; value: string }>()
	if (refused !== undefined && refused.value !== held) {
		setRefused(undefined)
	}
	const holding = useRef(held)
	useLayoutEffect(() => {
		holding.current = held
	})
	useEffect(() => {
		if (refused !== undefined) {
			reportFields(form.current)
		}
	}, [form, refused])
	return {
		validity: refused === undefined ? undefined : { type: 'invalid' as const, message: refused.message },
		refuse: (failure: unknown, sent: string): boolean => {
			const message = validationMessage(graphError(failure), '')
			const shown = form.current !== null && holding.current === sent
			if (message === '' || !shown) {
				return false
			}
			setRefused({ message, value: sent })
			return true
		},
		forget: () => setRefused(undefined),
	}
}
