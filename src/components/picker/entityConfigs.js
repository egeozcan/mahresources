import { entityFilterConfigs } from './entityFilterConfigs.ts';

const catalog = {
  resource: ['Resources', '/v1/resources'],
  group: ['Groups', '/v1/groups'],
  note: ['Notes', '/v1/notes'],
  category: ['Categories', '/v1/categories'],
  noteType: ['Note Types', '/v1/note/noteTypes'],
  resourceCategory: ['Resource Categories', '/v1/resourceCategories'],
  tag: ['Tags', '/v1/tags'],
  query: ['Queries', '/v1/queries'],
  relationType: ['Relation Types', '/v1/relationTypes'],
  series: ['Series', '/v1/seriesList'],
};

export const entityConfigs = Object.fromEntries(Object.entries(catalog).map(([entityType, [entityLabel, searchEndpoint]]) => [entityType, {
  entityType, entityLabel, searchEndpoint,
  filters: entityFilterConfigs[entityType].map(filter => ({ ...filter, endpoint: filter.kind === 'entity' ? catalog[filter.entity][1] : undefined })),
}]));

export function getEntityConfig(entityType) {
  const config = entityConfigs[entityType];
  if (!config) throw new Error(`Unknown entity type: ${entityType}`);
  return config;
}
