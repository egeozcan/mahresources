import { test, expect } from '../../fixtures/base.fixture';
import * as path from 'path';

const dialogSelector = '[aria-labelledby="entity-picker-title"]';
for (const entity of ['group', 'note', 'resource'] as const) {
 test(`${entity} picker renders its carrier template/CSS without initializing Alpine content`, async ({ page, apiClient }) => {
  const name = `Picker-custom-${entity}-${Date.now()}`;
  const route = { group: '/v1/category', note: '/v1/note/noteType', resource: '/v1/resourceCategory' }[entity];
  const response = await apiClient.request.post(route, { data: {
   Name: name,
   CustomEntityPickerResult: `<div x-init="window.pickerInitExecuted = true"><a class="picker-person" href="/${entity}?id=[property path=\"ID\"]" target="_blank" rel="noopener noreferrer">[property path="Name"]</a></div>`,
   CustomEntityPickerResultCSS: '.entity-picker-result .picker-person { color: rgb(12, 34, 56); }',
  } });
  expect(response.ok(), await response.text()).toBe(true);const carrier = await response.json();
  const item = entity === 'group' ? await apiClient.createGroup({ name, categoryId: carrier.ID })
   : entity === 'note' ? await apiClient.createNote({ name, noteTypeId: carrier.ID })
   : await apiClient.createResource({ name, resourceCategoryId: carrier.ID, filePath: path.join(__dirname, '../../test-assets/sample-image-35.png') });
  await page.goto('/groups');
  await page.evaluate(entity => (window as any).Alpine.store('entityPicker').open({ entityType: entity, onConfirm: () => {} }), entity);
  const dialog = page.locator(dialogSelector);await dialog.getByRole('textbox', { name: 'Name', exact: true }).fill(name);
  const row = dialog.locator(`[data-picker-id="${item.ID}"]`), content = row.locator('.picker-person');
  await expect(content).toHaveText(name);await expect(content).toHaveCSS('color','rgb(12, 34, 56)');
  expect(await page.evaluate(() => (window as any).pickerInitExecuted)).toBeUndefined();
  const opened = page.waitForEvent('popup');await content.click();const popup = await opened;await popup.close();
  await expect(row.getByRole('checkbox')).not.toBeChecked();
  await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
  await expect(page.locator('style[data-entity-picker-style]')).toHaveCount(0);
 });
}

test('carrier CSS is replaced across nested steps and a broken custom result logs and falls back', async ({ page, apiClient }) => {
 const prefix = `Picker-styles-${Date.now()}`;
 const carriers = [];
 for (const suffix of ['a','b','default']) {
  const response = await apiClient.request.post('/v1/category', { data: {
   Name: prefix + suffix,
   CustomEntityPickerResult: suffix === 'default' ? '' : `<b class="picker-${suffix}">[property path="Name"]</b>`,
   CustomEntityPickerResultCSS: suffix === 'default' ? '' : `.entity-picker-result .picker-${suffix} { font-weight: 700; }`,
  } });
  expect(response.ok()).toBe(true);carriers.push(await response.json());
 }
 const groups = [];
 for (const carrier of carriers) groups.push(await apiClient.createGroup({ name: carrier.Name, categoryId: carrier.ID }));
 await page.goto('/resource/new');await page.locator('[data-selector-field="groups" i]').getByRole('button', { name: 'Browse Groups', exact: true }).click();
 const dialog = page.locator(dialogSelector);await dialog.getByRole('textbox', { name: 'Name', exact: true }).fill(prefix);
 await expect(dialog.locator('[data-picker-id]')).toHaveCount(3);await expect(page.locator('style[data-entity-picker-style]')).toHaveCount(2);
 await expect(dialog.locator(`[data-picker-id="${groups[2].ID}"]`).getByRole('link')).toHaveAttribute('target','_blank');
 await dialog.getByRole('button', { name: 'Browse Categories', exact: true }).click();
 await expect(dialog.getByRole('heading', { name: 'Browse Categories', exact: true })).toBeVisible();
 await expect(page.locator('style[data-entity-picker-style]')).toHaveCount(0);
 await dialog.getByRole('button', { name: 'Back', exact: true }).click();await expect(page.locator('style[data-entity-picker-style]')).toHaveCount(2);
 const changed = await apiClient.request.post('/v1/category', { data: { ID: carriers[1].ID, Name: carriers[1].Name, CustomEntityPickerResult: '[mrql query="this is invalid"]' } });
 expect(changed.ok()).toBe(true);
 await dialog.getByRole('textbox', { name: 'Name', exact: true }).fill(carriers[1].Name);
 await expect(dialog.getByText('A custom picker result could not be rendered; default content was used.')).toBeVisible();
 await expect(dialog.locator(`[data-picker-id="${groups[1].ID}"]`).getByRole('link')).toHaveAttribute('target','_blank');
 await expect(dialog.locator('.picker-b')).toHaveCount(0);
 await expect.poll(async () => {const response = await apiClient.request.get('/v1/logs?EntityType=template');return await response.text();}).toContain('entity_picker_render');
 await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();await expect(page.locator('style[data-entity-picker-style]')).toHaveCount(0);
});

test('browsing a category preserves its schema for the originating form', async ({ page, apiClient }) => {
 const name = `Picker-schema-${Date.now()}`;
 const category = await apiClient.createCategory(name, '', { MetaSchema: JSON.stringify({ type: 'object', properties: { pickerField: { type: 'string', title: 'Picker schema field' } } }) });
 await page.goto('/group/new');await page.locator('[data-selector-field="CategoryId" i]').getByRole('button', { name: 'Browse Category', exact: true }).click();
 const dialog = page.locator(dialogSelector);await dialog.getByRole('textbox', { name: 'Name', exact: true }).fill(name);
 await dialog.locator(`[data-picker-id="${category.ID}"]`).getByRole('radio').check();await dialog.getByRole('button', { name: 'Confirm selection', exact: true }).click();
 await expect(page.getByRole('textbox', { name: 'Picker schema field', exact: true })).toBeVisible();
});
