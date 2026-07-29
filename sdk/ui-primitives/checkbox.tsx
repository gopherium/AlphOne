// SPDX-License-Identifier: AGPL-3.0-or-later

import { Checkbox as BaseCheckbox } from '@base-ui/react/checkbox'
import type { ComponentPropsWithoutRef } from 'react'

import styles from './checkbox.module.css'

export interface CheckboxProps
	extends Omit<ComponentPropsWithoutRef<typeof BaseCheckbox.Root>, 'render'> {
	label: string
}

/**
 * Renders a checkbox labelled for assistive technology.
 * @returns The checkbox.
 */
export function Checkbox({ label, className, ...props }: CheckboxProps) {
	return (
		<BaseCheckbox.Root
			aria-label={label}
			className={className === undefined ? styles.checkbox : `${styles.checkbox} ${className}`}
			{...props}
		>
			<BaseCheckbox.Indicator className={styles.indicator}>
				<svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true">
					<path fill="currentColor" d="M9 16.2 4.8 12l-1.4 1.4L9 19 21 7l-1.4-1.4z" />
				</svg>
			</BaseCheckbox.Indicator>
		</BaseCheckbox.Root>
	)
}
