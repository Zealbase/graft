import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

// This runs in Node.js - Don't use client-side code here (browser APIs, JSX...)

const config: Config = {
  title: 'graft',
  tagline: 'One canonical agent definition, synced across every AI-coding provider',
  favicon: 'img/favicon.ico',

  future: {
    v4: true, // Improve compatibility with the upcoming Docusaurus v4
  },

  // --------------------------------------------------------------------------
  // HOSTING: AWS S3 + CloudFront (default *.cloudfront.net domain, no custom
  // domain). Provisioned by infra/docs-site.cfn.yaml and deployed by
  // .github/workflows/deploy-docs.yml. Served at the root, so baseUrl is '/'.
  // If a custom domain is added later, update `url` accordingly.
  // --------------------------------------------------------------------------
  url: 'https://ddiyw5xqx0hu3.cloudfront.net', // CloudFront default domain
  baseUrl: '/',

  // organizationName/projectName only matter for the GitHub Pages deploy
  // command, which is not used yet. Left generic on purpose.
  organizationName: 'graft', // PLACEHOLDER
  projectName: 'graft', // PLACEHOLDER

  trailingSlash: false,

  onBrokenLinks: 'throw',

  markdown: {
    hooks: {
      onBrokenMarkdownLinks: 'throw',
    },
  },

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  headTags: [
    // Favicons
    { tagName: 'link', attributes: { rel: 'icon', type: 'image/x-icon', href: '/favicon.ico' } },
    { tagName: 'link', attributes: { rel: 'icon', type: 'image/png', sizes: '16x16', href: '/img/favicon-16.png' } },
    { tagName: 'link', attributes: { rel: 'icon', type: 'image/png', sizes: '32x32', href: '/img/favicon-32.png' } },
    { tagName: 'link', attributes: { rel: 'icon', type: 'image/svg+xml', href: '/img/logo.svg' } },
    // Apple
    { tagName: 'link', attributes: { rel: 'apple-touch-icon', sizes: '180x180', href: '/img/apple-touch-icon.png' } },
    // PWA manifest
    { tagName: 'link', attributes: { rel: 'manifest', href: '/site.webmanifest' } },
    { tagName: 'meta', attributes: { name: 'theme-color', content: '#22C55E' } },
  ],

  plugins: [require('./plugins/tailwind')],

  presets: [
    [
      'classic',
      {
        docs: {
          sidebarPath: './sidebars.ts',
          routeBasePath: '/', // docs-only site: docs served at the root
          // editUrl intentionally unset — no public repo bound yet.
        },
        blog: false, // docs-only: blog disabled
        theme: {
          customCss: ['./src/css/custom.css', './src/css/components.css'],
        },
      } satisfies Preset.Options,
    ],
  ],

  themes: [
    [
      // Local (offline) search — indexes docs at build time, no external service.
      '@easyops-cn/docusaurus-search-local',
      {
        hashed: true,
        indexBlog: false,
        docsRouteBasePath: '/',
      },
    ],
  ],

  themeConfig: {
    // Social card image — used for og:image and twitter:image on every page.
    image: 'img/og-1200x630.png',

    // Global meta tags (OG site name, Twitter card type).
    metadata: [
      { name: 'og:site_name', content: 'graft docs' },
      { name: 'twitter:card', content: 'summary_large_image' },
    ],

    colorMode: {
      respectPrefersColorScheme: true,
    },
    navbar: {
      title: 'graft',
      logo: {
        alt: 'graft',
        src: 'img/logo.svg',
        srcDark: 'img/logo-dark.svg',
      },
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'docsSidebar',
          position: 'left',
          label: 'Docs',
        },
      ],
    },
    footer: {
      style: 'dark',
      links: [
        {
          title: 'Docs',
          items: [
            {label: 'Quickstart', to: '/getting-started/quickstart'},
            {label: 'CLI reference', to: '/reference/cli'},
            {label: 'How sync works', to: '/concepts/how-sync-works'},
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} graft. Built with Docusaurus.`,
    },
    prism: {
      theme: prismThemes.nightOwlLight,
      darkTheme: prismThemes.nightOwl,
      additionalLanguages: ['bash', 'yaml', 'json', 'toml'],
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
