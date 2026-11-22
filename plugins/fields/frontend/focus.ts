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
 * Runs a focus move only when focus is still where the action started, inside the confirm that held it, or on the page.
 * @param from - The element focus was on when the action started, or the confirm that held it.
 * @param move - The move to run.
 */
export function whenFocusStayed(from: Element | null, move: () => void): void {
	const now = document.activeElement
	if ([from, document.body].includes(now) || insideConfirm(from, now)) {
		move()
	}
}

/**
 * Reports whether an element sits inside the confirm an action started from.
 * @param from - Where the action started.
 * @param now - The element holding focus.
 * @returns Whether the action started from a confirm that holds the element.
 */
function insideConfirm(from: Element | null, now: Element | null) {
	return from?.getAttribute('role') === 'alertdialog' && from.contains(now)
}

/**
 * Returns where an action pressed in a confirm starts from: the confirm that holds the focus, else the focused element.
 * @returns The element.
 */
export function focusedConfirm(): Element | null {
	const now = document.activeElement
	return now?.closest('[role="alertdialog"]') ?? now
}
