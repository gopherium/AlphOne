// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

import { declarationsIn } from '../testing'

/** phone is the condition of the media block a sheet keeps its phone rules in. */
const phone = '(max-width: 639px)'

/** sheet is a stylesheet holding a rule at the top level, rules beside it and one media block. */
const sheet = [
	'.alphone-probe:hover {',
	'\tcolor: green;',
	'}',
	'',
	'.alphone-probe .alphone-probe__inner {',
	'\tcolor: purple;',
	'}',
	'',
	'.alphone-probe {',
	'\tcolor: red;',
	"\tbackground: url('data:image/png') no-repeat;",
	'\tmargin-block-start:   var(--wpds-dimension-gap-sm, 8px) ;',
	'}',
	'',
	'.alphone-probe::before,',
	'.alphone-probe::after {',
	'\tcontent: attr(data-label);',
	'}',
	'',
	"[data-status='active'] {",
	'\tfont-weight: 600;',
	'}',
	'',
	`@media ${phone} {`,
	'\t.alphone-probe {',
	'\t\tcolor: blue;',
	'\t}',
	'',
	'\t.alphone-probe__phone {',
	'\t\tdisplay: none;',
	'\t}',
	'}',
	'',
].join('\n')

test('answers the declarations of a top level rule, keyed by property, each value split at its first colon', () => {
	expect(declarationsIn(sheet, '.alphone-probe')).toEqual({
		color: 'red',
		background: "url('data:image/png') no-repeat",
		'margin-block-start': 'var(--wpds-dimension-gap-sm, 8px)',
	})
})

test('answers the rule for exactly the selector asked, never a longer one that starts with it', () => {
	expect(declarationsIn(sheet, '.alphone-probe:hover')).toEqual({ color: 'green' })
	expect(declarationsIn(sheet, '.alphone-probe .alphone-probe__inner')).toEqual({ color: 'purple' })
})

test('reads a selector list across lines and a selector holding brackets and quotes as they are written', () => {
	expect(declarationsIn(sheet, '.alphone-probe::before,\n.alphone-probe::after')).toEqual({
		content: 'attr(data-label)',
	})
	expect(declarationsIn(sheet, "[data-status='active']")).toEqual({ 'font-weight': '600' })
})

test('answers the declarations of a rule one tab deep in the named media block', () => {
	expect(declarationsIn(sheet, '.alphone-probe', phone)).toEqual({ color: 'blue' })
	expect(declarationsIn(sheet, '.alphone-probe__phone', phone)).toEqual({ display: 'none' })
})

test('never answers a rule inside a media block for the top level', () => {
	expect(declarationsIn(sheet, '.alphone-probe__phone')).toEqual({})
})

test('answers an empty record for a rule the sheet lacks', () => {
	expect(declarationsIn(sheet, '.alphone-missing')).toEqual({})
	expect(declarationsIn(sheet, '.alphone-missing', phone)).toEqual({})
	expect(declarationsIn(sheet, '.alphone-probe', '(min-width: 640px)')).toEqual({})
})
