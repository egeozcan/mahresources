// src/components/picker/entityConfigs.js

export const entityConfigs = {
  resource: {
    entityType: 'resource',
    entityLabel: 'Resources',
    searchEndpoint: '/v1/resources',
    maxResults: 50,
    searchParams: (query, filters, maxResults) => {
      const params = new URLSearchParams({ MaxResults: String(maxResults) });
      if (query) params.set('name', query);
      if (filters.tags) {
        filters.tags.forEach(id => params.append('Tags', id));
      }
      if (filters.group) params.set('Groups', filters.group);
      return params;
    },
    filters: [
      { key: 'tags', label: 'Tags', endpoint: '/v1/tags', multi: true },
      { key: 'group', label: 'Group', endpoint: '/v1/groups', multi: false }
    ],
    tabs: [
      { id: 'note', label: "Note's Resources" },
      { id: 'all', label: 'All Resources' }
    ],
    renderItem: 'thumbnail',
    gridColumns: 'grid-cols-3 sm:grid-cols-4 md:grid-cols-5',
    getItemId: (item) => item.ID,
    getItemLabel: (item) => item.Name || `Resource ${item.ID}`
  },

  group: {
    entityType: 'group',
    entityLabel: 'Groups',
    searchEndpoint: '/v1/groups',
    maxResults: 50,
    searchParams: (query, filters, maxResults) => {
      const params = new URLSearchParams({ MaxResults: String(maxResults) });
      if (query) params.set('name', query);
      if (filters.category) params.set('categoryId', filters.category);
      return params;
    },
    filters: [
      { key: 'category', label: 'Category', endpoint: '/v1/categories', multi: false }
    ],
    tabs: null,
    renderItem: 'groupCard',
    gridColumns: 'grid-cols-2 md:grid-cols-3 lg:grid-cols-4',
    getItemId: (item) => item.ID,
    getItemLabel: (item) => item.Name || `Group ${item.ID}`
  }
};

export function getEntityConfig(entityType) {
  const config = entityConfigs[entityType];
  if (!config) {
    throw new Error(`Unknown entity type: ${entityType}`);
  }
  return config;
}
