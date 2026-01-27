import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

const config: Config = {
  title: 'Mahresources Documentation',
  tagline: 'Personal information management for files, notes, and relationships',
  favicon: 'img/favicon.ico',

  future: {
    v4: true,
  },

  url: 'https://egeozcan.github.io',
  baseUrl: '/mahresources/',

  organizationName: 'egeozcan',
  projectName: 'mahresources',
  trailingSlash: false,

  onBrokenLinks: 'throw',
  onBrokenMarkdownLinks: 'warn',

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      {
        docs: {
          routeBasePath: '/',
          sidebarPath: './sidebars.ts',
          editUrl: 'https://github.com/egeozcan/mahresources/tree/master/docs-site/',
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
    announcementBar: {
      id: 'security_warning',
      content: '⚠️ <strong>Security Notice:</strong> Mahresources has no authentication. Only run on trusted private networks.',
      backgroundColor: '#dc2626',
      textColor: '#ffffff',
      isCloseable: false,
    },
    navbar: {
      title: 'Mahresources',
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'docs',
          position: 'left',
          label: 'Documentation',
        },
        {
          href: 'https://github.com/egeozcan/mahresources',
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
            {label: 'Getting Started', to: '/getting-started/installation'},
            {label: 'User Guide', to: '/user-guide/navigation'},
            {label: 'API Reference', to: '/api/overview'},
          ],
        },
        {
          title: 'More',
          items: [
            {label: 'GitHub', href: 'https://github.com/egeozcan/mahresources'},
            {label: 'Issues', href: 'https://github.com/egeozcan/mahresources/issues'},
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} Mahresources. Built with Docusaurus.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: ['bash', 'json', 'yaml', 'go'],
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
