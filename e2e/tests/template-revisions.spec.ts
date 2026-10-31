/**
 * Template Revisions UI: the History page of a Category, Resource Category and
 * Note Type, its diff, and "Load into editor" (the edit form filled from a
 * revision, unsaved, with the load recorded as revisionApplied on the save).
 */
import { test, expect } from '../fixtures/base.fixture';
import type { Page } from '@playwright/test';

type Summary = {
  number: number;
  source: string;
  applied?: string[];
  changedSlots?: string[];
  label?: string;
};

async function postForm(page: Page, path: string, form: Record<string, string>): Promise<{ ID: number }> {
  const resp = await page.request.post(path, { form });
  expect(resp.ok(), `${path}: ${resp.status()} ${await resp.text()}`).toBeTruthy();
  return resp.json();
}

async function listRevisions(page: Page, prefix: string, id: number): Promise<Summary[]> {
  const resp = await page.request.get(`${prefix}/revisions?id=${id}`);
  expect(resp.ok()).toBeTruthy();
  return resp.json();
}

async function revisionDetail(page: Page, prefix: string, id: number, n: number) {
  const resp = await page.request.get(`${prefix}/revisions/${n}?id=${id}`);
  expect(resp.ok()).toBeTruthy();
  return resp.json();
}

async function save(page: Page, displayPath: string) {
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await page.waitForURL(new RegExp(`${displayPath}\\?id=`));
}

test.describe('Template Revisions history', () => {
  test('category history lists every revision with its changed slots and diffs two of them', async ({ page, apiClient }) => {
    const name = `Rev Cat ${Date.now()}`;
    const cat = await apiClient.createCategory(name, undefined, { CustomHeader: '<p>one</p>' });
    await postForm(page, '/v1/category', { ID: String(cat.ID), name, CustomHeader: '<p>two</p>' });
    await postForm(page, '/v1/category', { ID: String(cat.ID), name, CustomSidebar: '<aside>side</aside>' });

    // The display page links to History.
    await page.goto(`/category?id=${cat.ID}`);
    await page.getByRole('link', { name: 'History' }).click();
    await expect(page).toHaveURL(new RegExp(`/category/revisions\\?id=${cat.ID}`));

    await expect(page.locator('[data-testid^="revision-row-"]')).toHaveCount(3);
    await expect(page.getByTestId('revision-slots-3')).toHaveText('CustomSidebar');
    await expect(page.getByTestId('revision-slots-2')).toHaveText('CustomHeader');
    await expect(page.getByTestId('revision-source-1')).toHaveText('create');
    await expect(page.getByTestId('revision-source-2')).toHaveText('api');

    // Revision 1 against the current state: the header changed, the sidebar appeared.
    await page.getByTestId('revision-compare-1').click();
    await expect(page.getByTestId('revision-diff-heading')).toBeFocused();
    await expect(page.getByTestId('revision-diff-slot-CustomHeader')).toBeVisible();
    await expect(page.getByTestId('revision-diff-slot-CustomSidebar')).toBeVisible();
    await expect(page.getByTestId('revision-diff-slot-CustomSummary')).toHaveCount(0);
    const unified = page.getByTestId('revision-unified-CustomHeader');
    await expect(unified.locator('[data-op="delete"]')).toContainText('<p>one</p>');
    await expect(unified.locator('[data-op="insert"]')).toContainText('<p>two</p>');
    // Not by colour alone: the marker and the screen reader text are in the DOM.
    await expect(unified.locator('[data-op="delete"] .sr-only')).toHaveText('Removed: ');
    await expect(unified.locator('[data-op="insert"] .sr-only')).toHaveText('Added: ');

    await page.getByTestId('revision-layout-side').check();
    const side = page.getByTestId('revision-side-CustomHeader');
    await expect(side).toBeVisible();
    await expect(side.locator('td').first()).toContainText('<p>one</p>');

    // Two stored revisions: only the header differs between #1 and #2.
    await page.getByTestId('revision-to').selectOption('2');
    await expect(page.getByTestId('revision-diff-heading')).toContainText('#1 to revision #2');
    await expect(page.getByTestId('revision-diff-slot-CustomSidebar')).toHaveCount(0);
    await expect(page.getByTestId('revision-diff-slot-CustomHeader')).toBeVisible();
  });

  test('a label can be set from the history page and focus returns to its button', async ({ page, apiClient }) => {
    const name = `Rev Label ${Date.now()}`;
    const cat = await apiClient.createCategory(name, undefined, { CustomHeader: '<p>x</p>' });
    await page.goto(`/category/revisions?id=${cat.ID}`);
    await page.getByTestId('revision-label-edit-1').click();
    const input = page.getByTestId('revision-label-input-1');
    await expect(input).toBeFocused();
    await input.fill('before redesign');
    await input.press('Enter');
    await expect(page.getByTestId('revision-label-1')).toHaveText('before redesign');
    await expect(page.getByTestId('revision-label-edit-1')).toBeFocused();
    await expect(page.getByTestId('revisions-status')).toContainText('Saved the label of revision 1');
    expect((await listRevisions(page, '/v1/category', cat.ID))[0].label).toBe('before redesign');
  });

  test('loading one slot fills the editor unsaved and the save records loaded:<n>:<slot>', async ({ page, apiClient }) => {
    const name = `Rev Slot ${Date.now()}`;
    const cat = await apiClient.createCategory(name, undefined, { CustomHeader: '<p>one</p>' });
    await postForm(page, '/v1/category', { ID: String(cat.ID), name, CustomHeader: '<p>two</p>' });
    await postForm(page, '/v1/category', { ID: String(cat.ID), name, CustomSidebar: '<aside>side</aside>' });

    await page.goto(`/category/revisions?id=${cat.ID}`);
    await page.getByTestId('revision-compare-1').click();
    await page.getByTestId('revision-slot-load-from-CustomHeader').click();

    await expect(page).toHaveURL(new RegExp(`/category/edit\\?id=${cat.ID}$`));
    const notice = page.getByTestId('revision-load-notice');
    await expect(notice).toContainText('Loaded CustomHeader from revision #1');
    await expect(notice).toBeFocused();
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('<p>one</p>');
    // Only the chosen slot moved.
    await expect(page.locator('input[name="CustomSidebar"]')).toHaveValue('<aside>side</aside>');
    await expect(page.locator('input[name="revisionApplied"][value="loaded:1:CustomHeader"]')).toHaveCount(1);
    // Nothing saved yet.
    expect(await listRevisions(page, '/v1/category', cat.ID)).toHaveLength(3);

    await save(page, '/category');
    const revisions = await listRevisions(page, '/v1/category', cat.ID);
    expect(revisions).toHaveLength(4);
    expect(revisions[0]).toMatchObject({ number: 4, source: 'form', applied: ['loaded:1:CustomHeader'], changedSlots: ['CustomHeader'] });
  });

  test('untick CSS and the slot loads without its CSS companion', async ({ page, apiClient }) => {
    const name = `Rev Css ${Date.now()}`;
    const cat = await apiClient.createCategory(name, undefined, { CustomHeader: '<p>one</p>' });
    await postForm(page, '/v1/category', { ID: String(cat.ID), name, CustomHeader: '<p>two</p>', CustomHeaderCSS: '.x { color: red; }' });
    await page.goto(`/category/revisions?id=${cat.ID}`);
    await page.getByTestId('revision-compare-1').click();
    await page.getByTestId('revision-slot-css-CustomHeader').uncheck();
    await page.getByTestId('revision-slot-load-from-CustomHeader').click();
    await expect(page.getByTestId('revision-load-notice')).toContainText('Loaded CustomHeader from revision #1');
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('<p>one</p>');
    await expect(page.locator('input[name="CustomHeaderCSS"]')).toHaveValue('.x { color: red; }');
  });

  test('applying a preset records preset:<name> on the save', async ({ page, apiClient }) => {
    const cat = await apiClient.createCategory(`Rev Preset ${Date.now()}`);
    await page.goto(`/category/edit?id=${cat.ID}`);
    await expect.poll(async () => page.locator('#tb-preset option').count(), { timeout: 20_000 }).toBeGreaterThan(1);
    await page.locator('#tb-preset').selectOption('contact-card');
    await page.getByTestId('tb-apply-preset').click();
    await expect(page.locator('input[name="revisionApplied"][value="preset:contact-card"]')).toHaveCount(1);
    await expect(page.locator('input[name="CustomHeader"]')).not.toHaveValue('');
    await save(page, '/category');
    const newest = (await listRevisions(page, '/v1/category', cat.ID))[0];
    expect(newest.source).toBe('form');
    expect(newest.applied).toContain('preset:contact-card');
  });

  test('copying from an existing category records copied:<carrier>:<id> on the save', async ({ page, apiClient }) => {
    const source = await apiClient.createCategory(`Rev Source ${Date.now()}`, undefined, { CustomHeader: '<p>copied</p>' });
    const cat = await apiClient.createCategory(`Rev Copy ${Date.now()}`);
    await page.goto(`/category/edit?id=${cat.ID}`);
    const choice = `category:${source.ID}`;
    await expect(page.locator(`#tb-copy option[value="${choice}"]`)).toHaveCount(1, { timeout: 20_000 });
    await page.locator('#tb-copy').selectOption(choice);
    await page.getByRole('button', { name: 'Copy', exact: true }).click();
    await expect(page.locator(`input[name="revisionApplied"][value="copied:${choice}"]`)).toHaveCount(1);
    await save(page, '/category');
    const newest = (await listRevisions(page, '/v1/category', cat.ID))[0];
    expect(newest).toMatchObject({ source: 'form', applied: [`copied:${choice}`] });
  });

  test('a sparse section layout loads as the resolver output and saves without a section change', async ({ page, apiClient }) => {
    const name = `Rev Section ${Date.now()}`;
    const cat = await apiClient.createCategory(name, undefined, { CustomHeader: '<p>a</p>', SectionConfig: '{"tags":false}' });
    await postForm(page, '/v1/category', { ID: String(cat.ID), name, CustomHeader: '<p>b</p>' });

    await page.goto(`/category/edit?id=${cat.ID}&loadRevision=1`);
    await expect(page.getByTestId('revision-load-notice')).toContainText('Loaded revision #1');
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('<p>a</p>');
    const config = JSON.parse(await page.locator('input[name="SectionConfig"]').inputValue());
    expect(config.tags).toBe(false);
    expect(config.ownEntities.state).toBe('default');
    // The load is not part of the URL any more, so a reload shows the saved state.
    await expect(page).toHaveURL(new RegExp(`/category/edit\\?id=${cat.ID}$`));

    await save(page, '/category');
    const newest = (await listRevisions(page, '/v1/category', cat.ID))[0];
    expect(newest).toMatchObject({ number: 3, applied: ['loaded:1'], changedSlots: ['CustomHeader'] });
  });

  test('unusual section layouts load as defaults the form can render', async ({ page, apiClient }) => {
    const name = `Rev Odd ${Date.now()}`;
    const cat = await apiClient.createCategory(name, undefined, { CustomHeader: '<p>a</p>', SectionConfig: '{"ownEntities":null}' });
    await postForm(page, '/v1/category', { ID: String(cat.ID), name, CustomHeader: '<p>b</p>', SectionConfig: '{"ownEntities":[],"tags":false}' });
    const errors: string[] = [];
    page.on('pageerror', (e) => errors.push(e.message));

    for (const n of [1, 2]) {
      await page.goto(`/category/edit?id=${cat.ID}&loadRevision=${n}`);
      await expect(page.getByTestId('revision-load-notice')).toContainText(`Loaded revision #${n}`);
      const config = JSON.parse(await page.locator('input[name="SectionConfig"]').inputValue());
      expect(config.ownEntities).toMatchObject({ state: expect.any(String) });
      const detail = await revisionDetail(page, '/v1/category', cat.ID, n);
      expect(config).toEqual(JSON.parse(detail.editor.SectionConfig));
      await expect(page.getByLabel('Own Entities visibility')).toBeVisible();
    }
    expect(errors).toEqual([]);
  });

  test('a note type round trip restores ApplyTemplatesToShares', async ({ page }) => {
    const name = `Rev NT ${Date.now()}`;
    const created = await postForm(page, '/v1/note/noteType', { name, CustomHeader: '<p>nt1</p>', ApplyTemplatesToShares: 'true' });
    await postForm(page, '/v1/note/noteType/edit', { ID: String(created.ID), name, CustomHeader: '<p>nt2</p>', ApplyTemplatesToShares: 'false' });

    await page.goto(`/noteType?id=${created.ID}`);
    await page.getByRole('link', { name: 'History' }).click();
    await expect(page.locator('[data-testid^="revision-row-"]')).toHaveCount(2);
    await page.getByTestId('revision-load-1').click();

    await expect(page.getByTestId('revision-load-notice')).toContainText('Loaded revision #1');
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('<p>nt1</p>');
    await expect(page.locator('input[type="checkbox"][name="ApplyTemplatesToShares"]')).toBeChecked();
    await save(page, '/noteType');

    const revisions = await listRevisions(page, '/v1/noteType', created.ID);
    expect(revisions[0]).toMatchObject({ number: 3, source: 'form', applied: ['loaded:1'] });
    expect((await revisionDetail(page, '/v1/noteType', created.ID, 3)).editor.ApplyTemplatesToShares).toBe(true);
  });

  test('a resource category round trip restores AutoDetectRules', async ({ page, apiClient }) => {
    const name = `Rev RC ${Date.now()}`;
    const rulesA = { contentTypes: ['image/png'], priority: 10 };
    const rulesB = { contentTypes: ['image/jpeg'], priority: 5 };
    const rc = await apiClient.createResourceCategory(name, undefined, { CustomHeader: '<p>rc1</p>', AutoDetectRules: JSON.stringify(rulesA) });
    await postForm(page, '/v1/resourceCategory', { ID: String(rc.ID), name, CustomHeader: '<p>rc2</p>', AutoDetectRules: JSON.stringify(rulesB) });

    await page.goto(`/resourceCategory?id=${rc.ID}`);
    await page.getByRole('link', { name: 'History' }).click();
    await expect(page.locator('[data-testid^="revision-row-"]')).toHaveCount(2);
    await page.getByTestId('revision-load-1').click();

    await expect(page.getByTestId('revision-load-notice')).toContainText('Loaded revision #1');
    expect(JSON.parse(await page.locator('textarea[name="AutoDetectRules"]').inputValue())).toEqual(rulesA);
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('<p>rc1</p>');
    await save(page, '/resourceCategory');

    const revisions = await listRevisions(page, '/v1/resourceCategory', rc.ID);
    expect(revisions[0]).toMatchObject({ number: 3, source: 'form', applied: ['loaded:1'] });
    const detail = await revisionDetail(page, '/v1/resourceCategory', rc.ID, 3);
    expect(JSON.parse(detail.editor.AutoDetectRules)).toEqual(rulesA);
  });

  test('the form is locked while a revision loads, so a typed edit is never overwritten', async ({ page, apiClient }) => {
    const name = `Rev Busy ${Date.now()}`;
    const rulesA = { contentTypes: ['image/png'], priority: 10 };
    const rc = await apiClient.createResourceCategory(name, undefined, { AutoDetectRules: JSON.stringify(rulesA) });
    await postForm(page, '/v1/resourceCategory', { ID: String(rc.ID), name, AutoDetectRules: JSON.stringify({ contentTypes: ['image/jpeg'], priority: 5 }) });

    let release!: () => void;
    const gate = new Promise<void>((resolve) => { release = resolve; });
    await page.route(`**/v1/resourceCategory/revisions/1?id=${rc.ID}`, async (route) => {
      await gate;
      await route.continue();
    });
    await page.goto(`/resourceCategory/edit?id=${rc.ID}&loadRevision=1`);
    const form = page.locator('form').first();
    await expect(form).toHaveAttribute('aria-busy', 'true');
    expect(await form.evaluate((f: HTMLFormElement) => f.inert)).toBe(true);
    await expect(page.getByRole('status').filter({ hasText: 'The form is locked until it arrives' }).first()).toBeAttached();
    // Typing and saving are both out of reach until the revision lands.
    await expect(page.locator('textarea[name="AutoDetectRules"]').click({ timeout: 700 })).rejects.toThrow();
    await expect(page.getByRole('button', { name: 'Save', exact: true }).click({ timeout: 700 })).rejects.toThrow();

    release();
    await expect(page.getByTestId('revision-load-notice')).toContainText('Loaded revision #1');
    await expect(form).not.toHaveAttribute('aria-busy', 'true');
    expect(await form.evaluate((f: HTMLFormElement) => f.inert)).toBe(false);
    expect(JSON.parse(await page.locator('textarea[name="AutoDetectRules"]').inputValue())).toEqual(rulesA);
    // After the load an edit is the user's and stays.
    await page.locator('textarea[name="AutoDetectRules"]').fill('{"priority": 1}');
    await expect(page.locator('textarea[name="AutoDetectRules"]')).toHaveValue('{"priority": 1}');
  });

  test('a slow label save cannot close or refocus a different row, which cannot be edited meanwhile', async ({ page, apiClient }) => {
    const name = `Rev Slow Label ${Date.now()}`;
    const cat = await apiClient.createCategory(name, undefined, { CustomHeader: '<p>a</p>' });
    await postForm(page, '/v1/category', { ID: String(cat.ID), name, CustomHeader: '<p>b</p>' });
    let release!: () => void;
    const gate = new Promise<void>((resolve) => { release = resolve; });
    await page.route(`**/v1/category/revisions/1/label?id=${cat.ID}`, async (route) => {
      await gate;
      await route.continue();
    });
    await page.goto(`/category/revisions?id=${cat.ID}`);
    await page.getByTestId('revision-label-edit-1').click();
    await page.getByTestId('revision-label-input-1').fill('first');
    await page.getByTestId('revision-label-input-1').press('Enter');
    // While it is in flight the other row cannot start a draft that completion would discard.
    await expect(page.getByTestId('revision-label-edit-2')).toBeDisabled();
    release();
    await expect(page.getByTestId('revision-label-1')).toHaveText('first');
    await expect(page.getByTestId('revision-label-edit-1')).toBeFocused();
    await expect(page.getByTestId('revision-label-edit-2')).toBeEnabled();
    // A draft started afterwards is untouched by the earlier save.
    await page.getByTestId('revision-label-edit-2').click();
    await page.getByTestId('revision-label-input-2').fill('second');
    await expect(page.getByTestId('revision-label-input-2')).toHaveValue('second');
  });

  test('clearing From drops the diff and a response still on its way does not bring it back', async ({ page, apiClient }) => {
    const name = `Rev Clear ${Date.now()}`;
    const cat = await apiClient.createCategory(name, undefined, { CustomHeader: '<p>a</p>' });
    await postForm(page, '/v1/category', { ID: String(cat.ID), name, CustomHeader: '<p>b</p>' });
    await page.goto(`/category/revisions?id=${cat.ID}`);
    await page.getByTestId('revision-compare-1').click();
    await expect(page.getByTestId('revision-diff-slot-CustomHeader')).toBeVisible();

    let release!: () => void;
    const gate = new Promise<void>((resolve) => { release = resolve; });
    await page.route('**/v1/category/revisions/diff*', async (route) => {
      await gate;
      await route.continue();
    });
    await page.getByTestId('revision-to').selectOption('2');
    await page.getByTestId('revision-from').selectOption('');
    await expect(page.getByTestId('revision-diff')).toHaveCount(0);
    await expect(page.getByTestId('revision-slot-load-from-CustomHeader')).toHaveCount(0);
    release();
    await page.waitForTimeout(500);
    await expect(page.getByTestId('revision-diff')).toHaveCount(0);
  });

  test('loading a slot with its CSS ticked takes the revision CSS too, and saves it', async ({ page, apiClient }) => {
    const name = `Rev Css Default ${Date.now()}`;
    const cat = await apiClient.createCategory(name, undefined, { CustomHeader: '<p>old</p>' });
    await postForm(page, '/v1/category', { ID: String(cat.ID), name, CustomHeader: '<p>old</p>', CustomHeaderCSS: '.old { color: red; }' });
    await postForm(page, '/v1/category', { ID: String(cat.ID), name, CustomHeader: '<p>cur</p>', CustomHeaderCSS: '.cur { color: blue; }' });

    await page.goto(`/category/revisions?id=${cat.ID}`);
    await page.getByTestId('revision-compare-2').click();
    await expect(page.getByTestId('revision-slot-css-CustomHeader')).toBeChecked();
    await page.getByTestId('revision-slot-load-from-CustomHeader').click();
    await expect(page.getByTestId('revision-load-notice')).toContainText('Loaded CustomHeader from revision #2');
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('<p>old</p>');
    await expect(page.locator('input[name="CustomHeaderCSS"]')).toHaveValue('.old { color: red; }');
    await save(page, '/category');
    const detail = await revisionDetail(page, '/v1/category', cat.ID, 4);
    expect(detail.editor).toMatchObject({ CustomHeader: '<p>old</p>', CustomHeaderCSS: '.old { color: red; }' });
  });

  test('a load replaces the section control state wholesale, leaving no stale key', async ({ page, apiClient }) => {
    const name = `Rev Wholesale ${Date.now()}`;
    const cat = await apiClient.createCategory(name, undefined, { CustomHeader: '<p>a</p>', SectionConfig: '{"tags":false}' });
    await postForm(page, '/v1/category', { ID: String(cat.ID), name, CustomHeader: '<p>b</p>' });
    const detail = await revisionDetail(page, '/v1/category', cat.ID, 1);
    const resolved = JSON.parse(detail.editor.SectionConfig);

    // Hold the load until a stale key is in the control state.
    let release!: () => void;
    const gate = new Promise<void>((resolve) => { release = resolve; });
    await page.route(`**/v1/category/revisions/1?id=${cat.ID}`, async (route) => {
      await gate;
      await route.continue();
    });
    await page.goto(`/category/edit?id=${cat.ID}&loadRevision=1`);
    await page.evaluate(() => {
      const el = document.querySelector('[x-data^="sectionConfigForm"]') as HTMLElement;
      const data = (window as unknown as { Alpine: { $data(e: Element): { config: Record<string, unknown> } } }).Alpine.$data(el);
      data.config.staleKey = 'left over';
    });
    release();
    await expect(page.getByTestId('revision-load-notice')).toContainText('Loaded revision #1');

    const posted = JSON.parse(await page.locator('input[name="SectionConfig"]').inputValue());
    expect(posted).toEqual(resolved);
    expect(posted).not.toHaveProperty('staleKey');
    const state = await page.evaluate(() => {
      const el = document.querySelector('[x-data^="sectionConfigForm"]') as HTMLElement;
      return JSON.parse(JSON.stringify((window as unknown as { Alpine: { $data(e: Element): { config: unknown } } }).Alpine.$data(el).config));
    });
    expect(state).toEqual(resolved);
  });

  for (const failure of ['404', '403', 'abort', 'css-companion'] as const) {
    test(`a load that fails (${failure}) unlocks the form and says so`, async ({ page, apiClient }) => {
      const cat = await apiClient.createCategory(`Rev Fail ${failure} ${Date.now()}`, undefined, { CustomHeader: '<p>x</p>' });
      let url = `/category/edit?id=${cat.ID}&loadRevision=1`;
      let message = 'Could not load revision #1';
      if (failure === '404') url = `/category/edit?id=${cat.ID}&loadRevision=99`;
      if (failure === '404') message = 'Could not load revision #99';
      if (failure === 'css-companion') {
        url += '&slot=CustomHeaderCSS';
        message = 'Could not load CustomHeaderCSS from revision #1';
      }
      if (failure === '403') {
        await page.route('**/v1/category/revisions/1?*', (route) => route.fulfill({ status: 403, contentType: 'application/json', body: '{"error":"insufficient permissions"}' }));
      }
      if (failure === 'abort') {
        await page.route('**/v1/category/revisions/1?*', (route) => route.abort());
      }
      await page.goto(url);
      const notice = page.getByTestId('revision-load-notice');
      await expect(notice).toContainText(message);
      await expect(notice).toBeFocused();
      const form = page.locator('form').first();
      expect(await form.evaluate((f: HTMLFormElement) => f.inert)).toBe(false);
      await expect(form).not.toHaveAttribute('aria-busy', 'true');
      await expect(page.locator('input[name="revisionApplied"]')).toHaveCount(0);
      // Editing works.
      await page.locator('input[name="name"]').fill('still editable');
      await expect(page.locator('input[name="name"]')).toHaveValue('still editable');
    });
  }

  test('typing into the label input while its save is in flight is refused, not lost', async ({ page, apiClient }) => {
    const cat = await apiClient.createCategory(`Rev Same Row ${Date.now()}`, undefined, { CustomHeader: '<p>x</p>' });
    let release!: () => void;
    const gate = new Promise<void>((resolve) => { release = resolve; });
    await page.route(`**/v1/category/revisions/1/label?id=${cat.ID}`, async (route) => {
      await gate;
      await route.continue();
    });
    await page.goto(`/category/revisions?id=${cat.ID}`);
    await page.getByTestId('revision-label-edit-1').click();
    const input = page.getByTestId('revision-label-input-1');
    await input.fill('saved');
    await input.press('Enter');
    await expect(input).toHaveJSProperty('readOnly', true);
    await expect(page.getByTestId('revisions-status')).toContainText('Saving the label of revision 1');
    await input.focus();
    await page.keyboard.type('lost?');
    await expect(input).toHaveValue('saved');
    release();
    await expect(page.getByTestId('revision-label-1')).toHaveText('saved');
  });

  test('a diff body that arrives after the selection changed is not committed', async ({ page, apiClient }) => {
    const name = `Rev Stale Body ${Date.now()}`;
    const cat = await apiClient.createCategory(name, undefined, { CustomHeader: '<p>a</p>' });
    await postForm(page, '/v1/category', { ID: String(cat.ID), name, CustomHeader: '<p>b</p>' });
    await postForm(page, '/v1/category', { ID: String(cat.ID), name, CustomSidebar: '<aside>s</aside>' });
    // Headers of the first diff response arrive at once; its body waits for the test.
    await page.addInitScript(() => {
      const w = window as unknown as { __release?: () => void; __held?: boolean };
      const gate = new Promise<void>((resolve) => { w.__release = resolve; });
      const real = window.fetch.bind(window);
      w.__held = false;
      window.fetch = async (input, init) => {
        const resp = await real(input, init);
        const url = typeof input === 'string' ? input : (input as Request).url;
        if (url.includes('/revisions/diff') && url.includes('to=2') && !w.__held) {
          w.__held = true;
          const json = resp.json.bind(resp);
          resp.json = async () => { await gate; return json(); };
        }
        return resp;
      };
    });
    await page.goto(`/category/revisions?id=${cat.ID}`);
    await expect(page.getByTestId('revision-row-3')).toBeVisible();
    await page.getByTestId('revision-from').selectOption('1');
    await page.getByTestId('revision-to').selectOption('2'); // held: would show only the header change
    await page.getByTestId('revision-to').selectOption('3'); // current answer
    await expect(page.getByTestId('revision-diff-slot-CustomSidebar')).toBeVisible();
    await page.evaluate(() => (window as unknown as { __release: () => void }).__release());
    await page.waitForTimeout(500);
    await expect(page.getByTestId('revision-diff-heading')).toContainText('#1 to revision #3');
    await expect(page.getByTestId('revision-diff-slot-CustomSidebar')).toBeVisible();
  });
});
