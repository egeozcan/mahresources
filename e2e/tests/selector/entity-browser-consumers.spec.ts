import { test, expect } from '../../fixtures/base.fixture';
import * as path from 'path';

const pickerSelector = '[aria-labelledby="entity-picker-title"]';
const image = path.join(__dirname, '../../test-assets/sample-image-34.png');

test('note resource tab uses associations, not the note id as a resource owner', async ({ page, apiClient }) => {
 const name = `Picker-note-${Date.now()}`;
 const category = await apiClient.createCategory(name);
 const owner = await apiClient.createGroup({ name, categoryId: category.ID });
 const note = await apiClient.createNote({ name, ownerId: owner.ID });
 let other = await apiClient.createGroup({ name: name + '-other', categoryId: category.ID });
 if (other.ID === note.ID) other = await apiClient.createGroup({ name: name + '-distinct', categoryId: category.ID });
 const linked = await apiClient.createResource({ filePath: image, name: name + '-linked', ownerId: other.ID });
 const unrelated = await apiClient.createResource({ filePath: image, name: name + '-unrelated', ownerId: owner.ID });
 await apiClient.addResourcesToNote(note.ID, [linked.ID]);
 const block = await apiClient.createBlock(note.ID, 'gallery', 'a', { resourceIds: [] });
 await page.goto(`/note?id=${note.ID}`);await page.getByRole('button', { name: 'Edit Blocks', exact: true }).click();
 await page.getByRole('button', { name: '+ Select Resources', exact: true }).click();
 const picker = page.locator(pickerSelector);
 await expect(picker.locator(`[data-picker-id="${linked.ID}"]`)).toBeVisible();
 await expect(picker.locator(`[data-picker-id="${unrelated.ID}"]`)).toHaveCount(0);
 await picker.getByRole('textbox', { name: 'Name', exact: true }).fill('nonexistent-note-resource');
 await expect(picker.getByText('No results match these filters.')).toBeVisible();
 await expect(picker.getByRole('button', { name: "Note's Resources", exact: true })).toHaveAttribute('aria-pressed', 'true');
 await picker.getByRole('textbox', { name: 'Name', exact: true }).fill('');
 await picker.locator(`[data-picker-id="${linked.ID}"]`).getByRole('checkbox').check();
 await picker.getByRole('button', { name: 'Confirm selection', exact: true }).click();
 await expect.poll(async () => (await apiClient.getBlock(block.id)).content.resourceIds).toEqual([linked.ID]);
});

test('reference block persistence is owned by the original callback and cancellation does not write', async ({ page, apiClient }) => {
 const name = `Picker-reference-${Date.now()}`;
 const category = await apiClient.createCategory(name);
 const group = await apiClient.createGroup({ name, categoryId: category.ID });
 const note = await apiClient.createNote({ name });
 const block = await apiClient.createBlock(note.ID, 'references', 'a', { groupIds: [] });
 await page.goto(`/note?id=${note.ID}`);await page.getByRole('button', { name: 'Edit Blocks', exact: true }).click();
 const opener = page.getByRole('button', { name: '+ Select Groups', exact: true });await opener.click();
 const picker = page.locator(pickerSelector);
 await picker.getByRole('textbox', { name: 'Name', exact: true }).fill(name);
 await picker.locator(`[data-picker-id="${group.ID}"]`).getByRole('checkbox').check();
 await picker.getByRole('button', { name: 'Cancel', exact: true }).click();
 expect((await apiClient.getBlock(block.id)).content.groupIds).toEqual([]);
 await opener.click();await picker.getByRole('textbox', { name: 'Name', exact: true }).fill(name);
 await picker.locator(`[data-picker-id="${group.ID}"]`).getByRole('checkbox').check();
 await picker.getByRole('button', { name: 'Confirm selection', exact: true }).click();
 await expect.poll(async () => (await apiClient.getBlock(block.id)).content.groupIds).toEqual([group.ID]);
});

test('plugin single-entity parameters require confirmation and keep their parent modal open', async ({ page, apiClient }) => {
 const name = `Picker-single-${Date.now()}`;
 const category = await apiClient.createCategory(name);
 const group = await apiClient.createGroup({ name, categoryId: category.ID });
 await page.goto('/groups');
 // Exercise the public modal event contract. No synthetic plugin action is run.
 await page.evaluate(() => window.dispatchEvent(new CustomEvent('plugin-action-open', { detail: {
  plugin: 'picker-test', action: 'not-executed', label: 'Choose target', entityType: 'resource', entityIds: [],
  params: [{ name: 'target', type: 'entity_ref', entity: 'group', multi: false, label: 'Target', default: 'none' }],
 } })));
 const modal = page.locator('[aria-labelledby="plugin-action-modal-title"]');await expect(modal).toBeVisible();
 await modal.getByRole('button', { name: 'Add groups', exact: true }).click();
 const picker = page.locator(pickerSelector);
 await picker.getByRole('textbox', { name: 'Name', exact: true }).fill(name);
 await picker.locator(`[data-picker-id="${group.ID}"]`).getByRole('radio').check();
 await expect(picker).toBeVisible();await expect(modal.locator('.plugin-action-modal-entityref-chips')).not.toContainText(`#${group.ID}`);
 await picker.getByRole('button', { name: 'Confirm selection', exact: true }).click();
 await expect(picker).not.toBeVisible();await expect(modal).toBeVisible();
 await expect(modal.locator('.plugin-action-modal-entityref-chips')).toContainText(`#${group.ID}`);
});

test('lightbox tag browsing autosaves once and Escape leaves the lightbox open', async ({ page, apiClient }) => {
 const name = `Picker-lightbox-${Date.now()}`;
 const resource = await apiClient.createResource({ filePath: image, name });
 const tag = await apiClient.createTag(name);
 await page.goto(`/resource?id=${resource.ID}`);await page.locator('[data-lightbox-item]').first().click();
 const lightbox = page.locator('[role="dialog"][x-show="$store.lightbox.isOpen"]');await expect(lightbox).toBeVisible();
 const edit = lightbox.getByRole('button', { name: 'Edit Tags', exact: true });if (await edit.isVisible()) await edit.click();
 const opener = lightbox.getByRole('button', { name: 'Browse Tags', exact: true });await opener.click();
 const picker = page.locator(pickerSelector);
 const search = picker.getByRole('textbox', { name: 'Name', exact: true });await search.fill(name);await expect(search).toBeFocused();
 await page.keyboard.press('Escape');await expect(picker).not.toBeVisible();await expect(lightbox).toBeVisible();await expect(opener).toBeFocused();
 const writes: string[] = [];page.on('request', request => {if (request.method() === 'POST' && new URL(request.url()).pathname === '/v1/resources/addTags') writes.push(request.url());});
 await opener.click();await search.fill(name);await picker.locator(`[data-picker-id="${tag.ID}"]`).getByRole('checkbox').check();
 await picker.getByRole('button', { name: 'Confirm selection', exact: true }).click();
 await expect.poll(async () => (await apiClient.getResource(resource.ID)).Tags?.some(value => value.ID === tag.ID)).toBe(true);
 expect(writes).toHaveLength(1);await expect(lightbox).toBeVisible();
});

test('Browse stays hidden without JavaScript and the native create form remains', async ({ browser, baseURL }) => {
 const context = await browser.newContext({ javaScriptEnabled: false });
 try {
  const page = await context.newPage();await page.goto(`${baseURL}/group/new`);
  await expect(page.getByRole('button', { name: /^Browse / })).toHaveCount(0);
  const form = page.locator('form[action^="/v1/group"]');
  await expect(form).toHaveAttribute('method', /post/i);
  await expect(form.locator('input[name="Name" i]')).toBeVisible();
 } finally {await context.close();}
});

test('the shared browser handles every catalog family', async ({ page }) => {
 await page.goto('/groups');
 for (const entity of ['category','group','note','noteType','query','relationType','resource','resourceCategory','series','tag']) {
  const response = page.waitForResponse(r => {const url = new URL(r.url());return url.pathname === '/v1/entity-picker' && url.searchParams.get('entity') === entity;});
  await page.evaluate(entity => (window as any).Alpine.store('entityPicker').open({ entityType: entity, onConfirm: () => {} }), entity);
  expect((await response).ok(), entity).toBe(true);
  await expect(page.locator(pickerSelector)).toBeVisible();
  await page.locator(pickerSelector).getByRole('button', { name: 'Cancel', exact: true }).click();
 }
});
