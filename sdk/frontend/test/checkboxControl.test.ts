// SPDX-License-Identifier: AGPL-3.0-or-later

import { CheckboxControl as wordpressCheckboxControl, Fieldset as wordpressFieldset } from '@wordpress/ui'
import { expect, test } from 'vitest'

import { CheckboxControl, Fieldset } from '../index'

test("hands plugins WordPress's own CheckboxControl and Fieldset", () => {
	expect(CheckboxControl).toBe(wordpressCheckboxControl)
	expect(Fieldset).toBe(wordpressFieldset)
})
