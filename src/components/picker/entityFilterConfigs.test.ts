import { readFileSync, readdirSync } from 'node:fs';
import { expect, it } from 'vitest';
import { entityFilterConfigs } from './entityFilterConfigs';

const forms = {
 resource: 'partials/form/searchFormResource.tpl', group: 'listGroups.tpl', note: 'listNotes.tpl',
 category: 'listCategories.tpl', noteType: 'listNoteTypes.tpl', resourceCategory: 'listResourceCategories.tpl',
 tag: 'listTags.tpl', query: 'listQueries.tpl', relationType: 'listRelationTypes.tpl',
};
it.each(Object.entries(forms))('covers the ordinary %s list filters', (entity, file) => {
 const markup = readFileSync(new URL(`../../../templates/${file}`, import.meta.url), 'utf8');
 const fields = [...markup.matchAll(/(?:name|elName)=["']([A-Za-z][A-Za-z0-9]*)["']/g)].map(match => match[1].toLowerCase());
 const configured = entityFilterConfigs[entity].map(field => field.key.toLowerCase());
 // Sorting is presentation; entity browsing deliberately has one stable ID order.
 for (const name of fields.filter(name => name !== 'sortby')) expect(configured, `${entity}.${name}`).toContain(name);
});
it('puts a browse button beside every shared or custom autocomplete input', () => {
 const root = new URL('../../../templates/', import.meta.url);
 let hosts = 0;
 function visit(directory: URL) {
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
   const path = new URL(entry.name + (entry.isDirectory() ? '/' : ''), directory);
   if (entry.isDirectory()) {visit(path);continue;}
   if (!entry.name.endsWith('.tpl')) continue;
   const markup = readFileSync(path, 'utf8');
   const inputs = [...markup.matchAll(/x-ref="autocompleter"/g)].length;
   if (!inputs) continue;
   hosts++;
   expect([...markup.matchAll(/include "\/partials\/form\/entityBrowseButton.tpl"/g)].length, path.pathname).toBe(inputs);
  }
 }
 visit(root);expect(hosts).toBeGreaterThanOrEqual(6);
});
it('also exposes series search and relation-side filters', () => {
 expect(entityFilterConfigs.series.map(field => field.key)).toEqual(expect.arrayContaining(['Name','Slug','CreatedBefore','CreatedAfter']));
 expect(entityFilterConfigs.group.map(field => field.key)).toEqual(expect.arrayContaining(['RelationTypeId','RelationSide','MetaQuery']));
});
