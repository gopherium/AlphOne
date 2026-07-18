import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
	site: 'https://docs.alph.one',
	integrations: [
		starlight({
			title: 'AlphOne',
			description:
				'A plugin-first CRM. Self-hosted, conversations first, built in Go and React.',
			social: [
				{
					icon: 'github',
					label: 'GitHub',
					href: 'https://github.com/gopherium/AlphOne',
				},
			],
			editLink: {
				baseUrl: 'https://github.com/gopherium/AlphOne/edit/main/docs/',
			},
			sidebar: [
				{
					label: 'Start here',
					items: [
						{ slug: 'start/what-is-alphone' },
						{ slug: 'start/local-development' },
					],
				},
				{
					label: 'Self-hosting',
					items: [
						{ slug: 'self-hosting/install' },
						{ slug: 'self-hosting/configuration' },
						{ slug: 'self-hosting/updates-and-backups' },
					],
				},
				{
					label: 'WhatsApp',
					items: [{ slug: 'whatsapp/meta-setup' }],
				},
				{
					label: 'Reference',
					items: [{ slug: 'reference/rest-api' }],
				},
				{
					label: 'Legal',
					items: [{ slug: 'legal/privacy-policy' }],
				},
			],
		}),
	],
});
