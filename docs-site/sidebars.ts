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
        'concepts/note-blocks',
        'concepts/groups',
        'concepts/tags-categories',
        'concepts/relationships',
        'concepts/series',
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
        'features/admin-overview',
        'features/versioning',
        'features/image-similarity',
        'features/saved-queries',
        'features/custom-templates',
        'features/meta-schemas',
        'features/note-sharing',
        'features/download-queue',
        'features/job-system',
        'features/activity-log',
        'features/thumbnail-generation',
        'features/custom-block-types',
        'features/entity-picker',
        'features/plugin-system',
        'features/plugin-actions',
        'features/plugin-hooks',
        'features/plugin-lua-api',
        'features/shortcodes',
        'features/mentions',
        'features/timeline-view',
        'features/mrql',
        'features/cli',
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
        'api/plugins',
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
        'deployment/public-sharing',
        'deployment/backups',
      ],
    },
    'troubleshooting',
  ],
};

export default sidebars;
