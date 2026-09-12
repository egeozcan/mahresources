import { describe, expect, it } from 'vitest';
import { createDynamicEntitySelectorProfile, createMultiEntityFieldProfile, createSingleEntityFieldProfile, createTagFieldProfile, type EntityProfileName } from './entityFieldProfiles';

describe('browse profile metadata', () => {
 it.each<EntityProfileName>(['category','group','note','noteType','query','relationType','resource','resourceCategory','series','tag'])('publishes %s without changing autocomplete', entity => {
  let category = 1;
  const profile = createMultiEntityFieldProfile({ entity, maximum: 0, parameters: () => ({ CategoryId: category }), excludeValues: () => [category] });
  expect(profile.browse.entity).toBe(entity);
  expect(profile.browse.multiple).toBe(true);
  expect(profile.browse.maximum).toBe(0);
  category = 2;
  expect(profile.browse.parameters()).toEqual({ CategoryId: 2 });
  expect(profile.browse.excludedKeys()).toEqual([2]);
  expect(profile.lookup.searchUrl).not.toContain('entity-picker');
  profile.selector.destroy();
 });
 it('retains single, dynamic and lean-tag semantics', () => {
  const single = createSingleEntityFieldProfile({ entity: 'group' });
  const dynamic = createDynamicEntitySelectorProfile({ entity: 'noteType', searchUrl: '/v1/note/noteTypes', multiple: false });
  const tag = createTagFieldProfile({ usage: 'resource' });
  expect(single.browse.multiple).toBe(false);
  expect(dynamic.browse.entity).toBe('noteType');
  expect(tag.browse.entity).toBe('tag');
  expect(tag.lookup.searchUrl).toBe('/v1/tags/suggest');
  [single,dynamic,tag].forEach(profile => profile.selector.destroy());
 });
});
