// SPDX-License-Identifier: AGPL-3.0-or-later

import { graphKeysOf } from '@alphone/frontend-sdk'
import { configureAuthTransport, createAuthQueryClient } from '@gopherium/react-auth'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

import '@gopherium/godmin/base.css'
import '@gopherium/react-auth/wpds/style.css'
import './index.css'
import { App } from './App'
import { graphAuthTransport } from './auth/graphTransport'
import { legalLinks, publicAuthScreen } from './auth/PublicAuth'
import { configureAppChannelNames } from './contacts/channel'
import { configureAppErrorText } from './i18n/errors'
import { startAppLocale } from './i18n/start'
import { plugins } from './plugins'
import { createAppRouter } from './router'

const { legal } = await startAppLocale()
configureAppErrorText()
configureAppChannelNames()

configureAuthTransport(graphAuthTransport)
const queryClient = createAuthQueryClient()
const graphKeys = graphKeysOf(plugins)
const router = createAppRouter()
const links = legalLinks(legal)
const publicScreen = publicAuthScreen(new URL(window.location.href), links, () => {
	window.location.assign('/')
})

createRoot(document.getElementById('root')!).render(
	<StrictMode>
		<App queryClient={queryClient} router={router} links={links} publicScreen={publicScreen} graphKeys={graphKeys} />
	</StrictMode>,
)
