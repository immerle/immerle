import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

// This runs in Node.js - Don't use client-side code here (browser APIs, JSX...)

const config: Config = {
  title: 'Immerle',
  tagline: 'Your music, self-hosted — and it sings.',
  favicon: 'img/favicon.ico',

  future: {
    v4: true, // Improve compatibility with the upcoming Docusaurus v4
  },

  url: 'https://immerle.github.io',
  baseUrl: '/',

  organizationName: 'immerle',
  projectName: 'immerle',

  onBrokenLinks: 'throw',

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'redocusaurus',
      {
        // Read the OpenAPI spec straight from the server's generated docs at
        // build time, so the API reference never drifts from the handlers.
        specs: [{spec: '../internal/api/docs/swagger.json', route: '/api/'}],
        theme: {primaryColor: '#1ed760'},
      },
    ],
    [
      'classic',
      {
        docs: {
          sidebarPath: './sidebars.ts',
          routeBasePath: '/', // docs are the site root; no separate landing page
          editUrl: 'https://github.com/immerle/immerle/tree/main/docs/',
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      } satisfies Preset.Options,
    ],
  ],

  themeConfig: {
    colorMode: {
      respectPrefersColorScheme: true,
    },
    navbar: {
      title: 'Immerle',
      logo: {
        alt: 'Immerle Logo',
        src: 'img/logo.svg',
      },
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'tutorialSidebar',
          position: 'left',
          label: 'Docs',
        },
        {to: '/api/', label: 'API', position: 'left'},
        {
          href: 'https://github.com/immerle/immerle',
          label: 'GitHub',
          position: 'right',
        },
      ],
    },
    footer: {
      style: 'dark',
      links: [
        {
          title: 'Docs',
          items: [
            {label: 'Introduction', to: '/'},
            {label: 'Installation', to: '/installation'},
            {label: 'Configuration', to: '/configuration'},
            {label: 'API reference', to: '/api/'},
          ],
        },
        {
          title: 'More',
          items: [{label: 'GitHub', href: 'https://github.com/immerle/immerle'}],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} Immerle. Built with Docusaurus.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
