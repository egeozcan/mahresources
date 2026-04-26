// src/components/picker/entityConfigs.js

export const entityConfigs = {
  resource: {
    entityType: 'resource',
    entityLabel: 'Resources',
    searchEndpoint: '/v1/resources',
    maxResults: 50,
    searchParams: (query, filters, lockedFilters = {}, maxResults) => {
      const params = new URLSearchParams({ MaxResults: String(maxResults) });
      if (query) params.set('name', query);
      if (filters.tags) {
        filters.tags.forEach(id => params.append('Tags', id));
      }
      if (filters.group) params.set('Groups', filters.group);
      if (lockedFilters.content_types) {
        lockedFilters.content_types.forEach(ct => params.append('ContentTypes', ct));
      }
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
    searchParams: (query, filters, lockedFilters = {}, maxResults) => {
      const params = new URLSearchParams({ MaxResults: String(maxResults) });
      if (query) params.set('name', query);
      if (filters.category) params.set('categoryId', filters.category);
      if (lockedFilters.category_ids) {
        lockedFilters.category_ids.forEach(id => params.append('Categories', id));
      }
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
  },

  note: {
    entityType: 'note',
    entityLabel: 'Notes',
    searchEndpoint: '/v1/notes',
    maxResults: 50,
    searchParams: (query, filters, lockedFilters = {}, maxResults) => {
      const params = new URLSearchParams({ MaxResults: String(maxResults) });
      if (query) params.set('name', query);
      if (filters.tags) filters.tags.forEach(id => params.append('Tags', id));
      if (lockedFilters.note_type_ids) {
        lockedFilters.note_type_ids.forEach(id => params.append('NoteTypeIds', id));
      }
      return params;
    },
    filters: [
      { key: 'tags', label: 'Tags', endpoint: '/v1/tags', multi: true }
    ],
    tabs: null,
    renderItem: 'noteCard',
    gridColumns: 'grid-cols-2 md:grid-cols-3 lg:grid-cols-4',
    getItemId: (item) => item.ID,
    getItemLabel: (item) => item.Name || `Note ${item.ID}`
  }
};

export function getEntityConfig(entityType) {
  const config = entityConfigs[entityType];
  if (!config) {
    throw new Error(`Unknown entity type: ${entityType}`);
  }
  return config;
}
