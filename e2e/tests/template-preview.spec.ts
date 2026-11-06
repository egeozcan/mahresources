/**
 * Live template preview on the Category, Resource Category and Note Type
 * editors. Every slot cluster ends with its own preview, which renders that
 * slot against the entity chosen once at the top of the Templates fieldset,
 * inside a sandboxed iframe.
 */
import { test, expect } from '../fixtures/base.fixture';
import { previewEntityChip } from '../helpers/template-preview-entity';
import type { Page, Request } from '@playwright/test';

// Scrolls a cluster's preview into view (previews render lazily, the first
// time they are seen) and returns its frame.
async function showPreview(page: Page, slot: string) {
  const preview = page.locator(`[data-slot-preview="${slot}"]`);
  await preview.scrollIntoViewIfNeeded();
  return page.frameLocator(`[data-slot-preview="${slot}"] iframe`);
}

// The slots the preview endpoint has been asked to render, in order.
function recordPreviewRequests(page: Page): string[] {
  const slots: string[] = [];
  page.on('request', (req: Request) => {
    if (req.method() === 'POST' && /\/previewTemplate$/.test(new URL(req.url()).pathname)) {
      try {
        slots.push(JSON.parse(req.postData() || '{}').slot);
      } catch {
        /* not JSON */
      }
    }
  });
  return slots;
}

test.describe('Template live preview', () => {
  test('picker result previews with its wrapper and companion CSS', async ({ page, apiClient }) => {
    const category = await apiClient.createCategory(`Picker Preview ${Date.now()}`);
    const groupName = `Picker Target ${Date.now()}`;
    await apiClient.createGroup({ name: groupName, categoryId: category.ID });
    await page.goto(`/category/edit?id=${category.ID}`);
    await expect(previewEntityChip(page)).toHaveAttribute('aria-label', `Remove ${groupName}`);
    await page.locator('.cm-content[aria-label="Entity Picker Result"]').fill('<b>[property path="Name"]</b>');
    await page.locator('.cm-content[aria-label="Entity Picker Result CSS"]').fill('.entity-picker-result b { color: rgb(12, 34, 56); }');
    const result = (await showPreview(page, 'CustomEntityPickerResult')).locator('.entity-picker-result b');
    await expect(result).toHaveText(groupName);
    await expect(result).toHaveCSS('color', 'rgb(12, 34, 56)');
  });

  test('editing Custom Header renders against a seeded group', async ({ page, apiClient }) => {
    const category = await apiClient.createCategory('Preview Cat');
    const groupName = `Preview Target ${Date.now()}`;
    await apiClient.createGroup({ name: groupName, categoryId: category.ID });

    await page.goto('/category/new');
    await page.waitForLoadState('load');

    // The shared picker defaults to the most recent group.
    const entityInput = page.locator('#tp-entity-group');
    await expect(entityInput).toBeVisible({ timeout: 10000 });
    await expect(previewEntityChip(page)).toHaveAttribute('aria-label', `Remove ${groupName}`, { timeout: 10000 });

    const header = page.locator('.cm-content[aria-label="Custom Header"]');
    await expect(header).toBeVisible({ timeout: 10000 });
    await header.click();
    await page.keyboard.type('[property path="Name"]');

    const frame = await showPreview(page, 'CustomHeader');
    await expect(frame.locator('body')).toContainText(groupName, { timeout: 10000 });

    // The app bundle must hydrate inside the sandboxed (opaque-origin) frame:
    // module scripts are CORS-fetched, so this only works while /public/ is
    // served with Access-Control-Allow-Origin.
    await expect
      .poll(
        async () => {
          const handle = await page.locator('[data-slot-preview="CustomHeader"] iframe').elementHandle();
          const srcdocFrame = await handle?.contentFrame();
          if (!srcdocFrame) return false;
          return srcdocFrame
            .evaluate(() => typeof (window as { Alpine?: unknown }).Alpine !== 'undefined')
            .catch(() => false);
        },
        { timeout: 10000 },
      )
      .toBe(true);
  });

  test('Alpine expressions see the same entity scope as the real pages', async ({ page, apiClient }) => {
    const category = await apiClient.createCategory('Alpine Scope Cat');
    const groupName = `Alpine Scope Target ${Date.now()}`;
    await apiClient.createGroup({ name: groupName, categoryId: category.ID });

    await page.goto('/category/new');
    await expect(previewEntityChip(page)).toHaveAttribute('aria-label', `Remove ${groupName}`, { timeout: 10000 });

    const header = page.locator('.cm-content[aria-label="Custom Header"]');
    await header.click();
    // Only Alpine can materialize this text: the server returns the markup
    // verbatim, so the assertion proves the frame recreates the display pages'
    // x-data="{ entity: ... }" scope.
    await page.keyboard.type('<div id="scope-probe" x-text="entity.Name"></div>');

    const frame = await showPreview(page, 'CustomHeader');
    await expect(frame.locator('#scope-probe')).toHaveText(groupName, { timeout: 10000 });
  });

  test('the List Header preview renders against the category itself (carrier mode)', async ({ page, apiClient }) => {
    const catName = `Carrier Preview Cat ${Date.now()}`;
    const category = await apiClient.createCategory(catName);

    await page.goto(`/category/edit?id=${category.ID}`);
    const editor = page.locator('.cm-content[aria-label="Custom List Header"]');
    await editor.click();
    await page.keyboard.type('CARRIER=[property path="Name"]');

    const frame = await showPreview(page, 'CustomListHeader');
    await expect(frame.locator('body')).toContainText(`CARRIER=${catName}`, { timeout: 10000 });
  });

  test('on the create form the List Header preview says to save first', async ({ page }) => {
    await page.goto('/category/new');
    const preview = page.locator('[data-slot-preview="CustomListHeader"]');
    await preview.scrollIntoViewIfNeeded();
    await expect(preview.getByRole('alert')).toContainText('Save this category first');
  });

  test('Custom CSS styles the slot previews and never becomes body text', async ({ page, apiClient }) => {
    // Every production sink for a CustomCSS buffer renders it as stylesheet
    // content and nothing else. The ::after content is the marker the body must
    // NOT contain as text: generated content never reaches textContent, so it
    // can only get there by the stylesheet being injected as markup.
    const stamp = Date.now();
    const marker = `css-applied-${stamp}`;
    const headerProbe = `header-probe-${stamp}`;
    const category = await apiClient.createCategory(`CSS Cat ${stamp}`, undefined, {
      CustomHeader: `<div id="${headerProbe}">rendered header</div>`,
    });
    await apiClient.createGroup({ name: `CSS Target ${stamp}`, categoryId: category.ID });

    await page.goto(`/category/edit?id=${category.ID}`);
    await expect(previewEntityChip(page)).toHaveCount(1);
    const frame = await showPreview(page, 'CustomHeader');
    await expect(frame.locator(`#${headerProbe}`)).toHaveCount(1);

    // The frame's own body reset is preview chrome, so the author's body rule
    // must outrank it.
    await page
      .locator('.cm-content[aria-label="Custom CSS"]')
      .fill(`body{background-color:rgb(3,5,7)}body::after{content:"${marker}"}`);
    await showPreview(page, 'CustomHeader');
    await expect
      .poll(() => frame.locator('body').evaluate((el) => getComputedStyle(el).backgroundColor).catch(() => ''))
      .toBe('rgb(3, 5, 7)');
    await expect
      .poll(() => frame.locator('body').evaluate((el) => getComputedStyle(el, '::after').content).catch(() => ''))
      .toContain(marker);
    await expect(frame.locator('body')).not.toContainText(marker);

    // Custom CSS has no markup, so it has no preview of its own.
    await expect(page.locator('[data-slot-preview="CustomCSS"]')).toHaveCount(0);
  });

  test('editing a category only offers entities from that category', async ({ page, apiClient }) => {
    const stamp = Date.now();
    const catA = await apiClient.createCategory(`Scoped Cat A ${stamp}`);
    const catB = await apiClient.createCategory(`Scoped Cat B ${stamp}`);
    const inA = `In Category A ${stamp}`;
    const inB = `In Category B ${stamp}`;
    await apiClient.createGroup({ name: inA, categoryId: catA.ID });
    // Created last, so an unfiltered default would pick this one.
    await apiClient.createGroup({ name: inB, categoryId: catB.ID });

    await page.goto(`/category/edit?id=${catA.ID}`);
    const entityInput = page.locator('#tp-entity-group');
    await expect(previewEntityChip(page)).toHaveAttribute('aria-label', `Remove ${inA}`, { timeout: 10000 });

    await entityInput.fill(inB);
    await page.waitForTimeout(600); // debounce + request
    await expect(page.locator('#tp-entity-group-listbox [role="option"]')).toHaveCount(0);
  });

  test('picking another entity re-renders the visible previews', async ({ page, apiClient }) => {
    const stamp = Date.now();
    const category = await apiClient.createCategory(`Pick Cat ${stamp}`, undefined, {
      CustomHeader: 'HDR=[property path="Name"]',
    });
    const first = `Pick First ${stamp}`;
    const second = `Pick Second ${stamp}`;
    await apiClient.createGroup({ name: first, categoryId: category.ID });
    await apiClient.createGroup({ name: second, categoryId: category.ID });

    await page.goto(`/category/edit?id=${category.ID}`);
    const entityInput = page.locator('#tp-entity-group');
    await expect(previewEntityChip(page)).toHaveCount(1);
    const current = ((await previewEntityChip(page).getAttribute('aria-label')) ?? '').replace(/^Remove /, '');
    const other = current === first ? second : first;
    const frame = await showPreview(page, 'CustomHeader');
    await expect(frame.locator('body')).toContainText(`HDR=${current}`);

    await entityInput.fill(other);
    await page.locator('#tp-entity-group-listbox [role="option"]', { hasText: other }).click();
    await expect(previewEntityChip(page)).toHaveAttribute('aria-label', `Remove ${other}`);
    await showPreview(page, 'CustomHeader');
    await expect(frame.locator('body')).toContainText(`HDR=${other}`);
  });

  test('the entity search is navigable with the arrow keys and Enter', async ({ page, apiClient }) => {
    const stamp = Date.now();
    const category = await apiClient.createCategory(`Keys Cat ${stamp}`, undefined, {
      CustomHeader: 'HDR=[property path="Name"]',
    });
    await apiClient.createGroup({ name: `Keys One ${stamp}`, categoryId: category.ID });
    await apiClient.createGroup({ name: `Keys Two ${stamp}`, categoryId: category.ID });
    // The chosen entity is left out of the suggestions, so three leave two to move between.
    await apiClient.createGroup({ name: `Keys Three ${stamp}`, categoryId: category.ID });

    await page.goto(`/category/edit?id=${category.ID}`);
    await expect(previewEntityChip(page)).toHaveCount(1);
    const entityInput = page.locator('#tp-entity-group');
    await entityInput.fill(`Keys`);
    const options = page.locator('#tp-entity-group-listbox [role="option"]');
    await expect(options).toHaveCount(2);

    // Each ArrowDown moves the active option, which the combobox names.
    await entityInput.press('ArrowDown');
    const firstActive = await entityInput.getAttribute('aria-activedescendant');
    await entityInput.press('ArrowDown');
    const active = await entityInput.getAttribute('aria-activedescendant');
    expect(active).toBeTruthy();
    expect(active).not.toBe(firstActive);
    const target = (await page.locator(`#${active}`).innerText()).trim();
    await expect(page.locator(`#${active}`)).toHaveAttribute('aria-selected', 'true');

    await entityInput.press('Enter');
    await expect(previewEntityChip(page)).toHaveAttribute('aria-label', `Remove ${target}`);
    const frame = await showPreview(page, 'CustomHeader');
    await expect(frame.locator('body')).toContainText(`HDR=${target}`);
  });

  test('previews render lazily and only for the slot that changed', async ({ page, apiClient }) => {
    const category = await apiClient.createCategory(`Lazy Cat ${Date.now()}`);
    await apiClient.createGroup({ name: `Lazy Target ${Date.now()}`, categoryId: category.ID });
    const slots = recordPreviewRequests(page);

    await page.goto(`/category/edit?id=${category.ID}`);
    await expect(previewEntityChip(page)).toHaveCount(1);
    // The page opens at the top, far above the last cluster.
    await page.waitForTimeout(1500);
    expect(slots).not.toContain('CustomEntityPickerResult');

    await showPreview(page, 'CustomEntityPickerResult');
    await expect.poll(() => slots.includes('CustomEntityPickerResult')).toBe(true);

    // Editing the Header re-renders the Header and leaves Summary alone.
    await showPreview(page, 'CustomSummary');
    await expect.poll(() => slots.includes('CustomSummary')).toBe(true);
    const before = slots.filter((s) => s === 'CustomSummary').length;
    await page.locator('.cm-content[aria-label="Custom Header"]').fill('<p id="lazy-hdr">changed</p>');
    await expect((await showPreview(page, 'CustomHeader')).locator('#lazy-hdr')).toHaveCount(1);
    await page.waitForTimeout(1000);
    expect(slots.filter((s) => s === 'CustomSummary').length).toBe(before);
  });

  test('a failed shortcode is explained in full under its preview', async ({ page, apiClient }) => {
    const category = await apiClient.createCategory(`Render Error Preview ${Date.now()}`);
    await apiClient.createGroup({ name: `Render Error Target ${Date.now()}`, categoryId: category.ID });
    await page.goto(`/category/edit?id=${category.ID}`);
    await expect(previewEntityChip(page)).toHaveCount(1);

    await page.locator('.cm-content[aria-label="Custom Header"]').fill('<p>[plugin:no-such-plugin:chart]</p>[mrql query="type = bogus (("]');

    const frame = await showPreview(page, 'CustomHeader');
    const preview = page.locator('[data-slot-preview="CustomHeader"]');
    await expect(frame.locator('.shortcode-error')).toHaveText('⚠ plugin:no-such-plugin:chart');
    const errors = preview.locator('h3', { hasText: 'Render errors' }).locator('..');
    await expect(errors).toBeVisible();
    await expect(errors).toContainText('plugin:no-such-plugin:chart: shortcode "plugin:no-such-plugin:chart" not found');
    await expect(errors.locator('li', { hasText: /^\s*mrql: \S/ })).toHaveCount(1);
    await expect(preview.locator('p[aria-live="polite"]', { hasText: '2 render errors in the preview' })).toHaveCount(1);

    await page.locator('.cm-content[aria-label="Custom Header"]').fill('<p>fine</p>');
    await expect(frame.locator('p')).toHaveText('fine');
    await expect(errors).toBeHidden();
  });
});

test.describe('A preview after every slot cluster', () => {
  // Each cluster's last child is its preview, titled after the slot.
  async function clusterPreviews(page: Page) {
    return page.locator('[data-template-cluster]').evaluateAll((clusters) =>
      clusters.map((c) => {
        const last = c.lastElementChild;
        return {
          cluster: c.getAttribute('data-template-cluster'),
          preview: last?.getAttribute('data-slot-preview') ?? null,
          hasFrame: !!last?.querySelector('iframe[sandbox="allow-scripts"]'),
        };
      }),
    );
  }

  for (const [path, create] of [
    ['/category/edit', 'category'],
    ['/resourceCategory/edit', 'resourceCategory'],
    ['/noteType/edit', 'noteType'],
  ] as const) {
    test(`${create} editor: every cluster ends with its own preview`, async ({ page, apiClient }) => {
      const name = `Clusters ${create} ${Date.now()}`;
      const carrier =
        create === 'category'
          ? await apiClient.createCategory(name)
          : create === 'resourceCategory'
            ? await apiClient.createResourceCategory(name)
            : await apiClient.createNoteType(name);
      await page.goto(`${path}?id=${carrier.ID}`);
      const rows = await clusterPreviews(page);
      expect(rows.length).toBeGreaterThan(8);
      for (const row of rows) {
        expect(row, `cluster ${row.cluster}`).toEqual({ cluster: row.cluster, preview: row.cluster, hasFrame: true });
      }
      // The single pane and its controls are gone.
      await expect(page.locator('[data-template-preview-pane]')).toHaveCount(0);
      await expect(page.locator('[id^="tp-slot-"], [id^="tp-dock-"]')).toHaveCount(0);
    });
  }

  test('a resource category previews its resource-only slots', async ({ page, apiClient }) => {
    const rc = await apiClient.createResourceCategory(`RC Slots ${Date.now()}`);
    await page.goto(`/resourceCategory/edit?id=${rc.ID}`);
    for (const slot of ['CustomPreview', 'CustomLightbox', 'CustomCell']) {
      await expect(page.locator(`[data-template-cluster="${slot}"] > [data-slot-preview="${slot}"]`)).toHaveCount(1);
    }
  });
});
