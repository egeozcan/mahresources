import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

const sidebars: SidebarsConfig = {
  docs: [
    'intro',
    {
      type: 'category',
      label: 'Getting Started',
      collapsed: false,
      items: [
        'getting-started/installation',
        'getting-started/quick-start',
        'getting-started/first-steps',
      ],
    },
    {
      type: 'category',
      label: 'Core Concepts',
      items: [
        'concepts/overview',
        'concepts/resources',
        'concepts/notes',
        'concepts/groups',
        'concepts/tags-categories',
        'concepts/relationships',
      ],
    },
    {
      type: 'category',
      label: 'User Guide',
      items: [
        'user-guide/navigation',
        'user-guide/managing-resources',
        'user-guide/managing-notes',
        'user-guide/organizing-with-groups',
        'user-guide/search',
        'user-guide/bulk-operations',
      ],
    },
    {
      type: 'category',
      label: 'Configuration',
      items: [
        'configuration/overview',
        'configuration/database',
        'configuration/storage',
        'configuration/advanced',
      ],
    },
    {
      type: 'category',
      label: 'Advanced Features',
      items: [
        'features/versioning',
        'features/image-similarity',
        'features/saved-queries',
        'features/custom-templates',
      ],
    },
    {
      type: 'category',
      label: 'API Reference',
      items: [
        'api/overview',
        'api/resources',
        'api/notes',
        'api/groups',
        'api/other-endpoints',
      ],
    },
    {
      type: 'category',
      label: 'Deployment',
      items: [
        'deployment/docker',
        'deployment/systemd',
        'deployment/reverse-proxy',
        'deployment/backups',
      ],
    },
    'troubleshooting',
  ],
};

export default sidebars;
