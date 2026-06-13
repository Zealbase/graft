import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

// This runs in Node.js - Don't use client-side code here (browser APIs, JSX...)

// Single explicit "docs" sidebar. Order = user-intent first, then system area
// (per documentation-structure-rules.md): Intro → Getting Started → Concepts
// → Guides → Reference → Providers. Progressive disclosure: quickstart first,
// reference later.
const sidebars: SidebarsConfig = {
  docsSidebar: [
    'intro',
    {
      type: 'category',
      label: 'Getting started',
      collapsed: false,
      items: ['getting-started/quickstart', 'getting-started/install'],
    },
    {
      type: 'category',
      label: 'Concepts',
      items: [
        'concepts/overview',
        'concepts/canonical-store',
        'concepts/providers',
        'concepts/how-sync-works',
        'concepts/drift-and-status',
        'concepts/skills',
      ],
    },
    {
      type: 'category',
      label: 'Guides',
      items: [
        'guides/sync-an-agent',
        'guides/resolve-conflicts',
        'guides/check-status',
        'guides/validate',
        'guides/destroy',
      ],
    },
    {
      type: 'category',
      label: 'Reference',
      items: [
        'reference/cli',
        'reference/skill-command',
        'reference/config',
        'reference/canonical-format',
        'reference/endpoints',
        'reference/faq',
      ],
    },
    {
      type: 'category',
      label: 'Providers',
      items: ['providers/overview'],
    },
  ],
};

export default sidebars;
