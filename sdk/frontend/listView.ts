// SPDX-License-Identifier: AGPL-3.0-or-later

import { listSearch, openOnTap, useListView as useGodminListView } from '@gopherium/godmin/router'
import type { ListDefaults, ListSearch, ListView as GodminListView } from '@gopherium/godmin/router'
import type { View } from '@wordpress/dataviews'

/** ListView is godmin's list view over a DataViews view. */
export type ListView = GodminListView<View>

/** useListView is godmin's list view hook, answering a DataViews view. */
export const useListView: (defaults: ListDefaults) => ListView = useGodminListView

export { listSearch, openOnTap }
export type { ListDefaults, ListSearch }
