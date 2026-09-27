// SPDX-License-Identifier: AGPL-3.0-or-later

/** CONTROLS matches the cells a form's first control is found among. */
const CONTROLS = 'input:not([aria-hidden="true"]), textarea, [role="checkbox"]'

/**
 * Moves focus to a form's first cell.
 * @param form - The form, which always holds at least one cell.
 */
export function focusFirstControl(form: HTMLFormElement): void {
	;(form.querySelector(CONTROLS) as HTMLElement).focus()
}

/**
 * Runs a focus move only when focus is still where the action started, or fell to the page.
 * @param from - The element focus was on when the action started.
 * @param move - The move to run.
 */
export function whenFocusStayed(from: Element | null, move: () => void): void {
	const now = document.activeElement
	if (now === from || now === document.body) {
		move()
	}
}
