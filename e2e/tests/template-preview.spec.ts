/**
 * Step 4 (category template authoring): live template preview. Editing a
 * Custom* slot on the category form renders it against a real group inside a
 * sandboxed iframe.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Template live preview', () => {
  test('editing Custom Header renders against a seeded group', async ({ page, apiClient }) => {
    const category = await apiClient.createCategory('Preview Cat');
    const groupName = `Preview Target ${Date.now()}`;
    await apiClient.createGroup({ name: groupName, categoryId: category.ID });

    await page.goto('/category/new');
    await page.waitForLoadState('load');

    // The preview pane defaults to the most recent group; give it a moment.
    const entityInput = page.locator('#tp-entity-group');
    await expect(entityInput).toBeVisible({ timeout: 10000 });
    await expect(entityInput).toHaveValue(groupName, { timeout: 10000 });

    // Type a property shortcode into the Custom Header editor.
    const header = page.locator('.cm-content[aria-label="Custom Header"]');
    await expect(header).toBeVisible({ timeout: 10000 });
    await header.click();
    await page.keyboard.type('[property path="Name"]');

    // The sandboxed iframe should render the group's name (debounced refresh).
    const frame = page.frameLocator('iframe[title="Template slot preview"]');
    await expect(frame.locator('body')).toContainText(groupName, { timeout: 10000 });

    // The app bundle must hydrate inside the sandboxed (opaque-origin) frame:
    // module scripts are CORS-fetched, so this only works while /public/ is
    // served with Access-Control-Allow-Origin. Guards against regressing the
    // "web components and Alpine widgets hydrate in preview" behaviour.
    await expect
      .poll(
        async () => {
          const srcdocFrame = page.frames().find((f) => f.url() === 'about:srcdoc');
          if (!srcdocFrame) return false;
          return srcdocFrame
            .evaluate(() => typeof (window as { Alpine?: unknown }).Alpine !== 'undefined')
            .catch(() => false);
        },
        { timeout: 10000 },
      )
      .toBe(true);
  });

  test('Alpine expressions see the same entity scope as the real pages', async ({
    page,
    apiClient,
  }) => {
    const category = await apiClient.createCategory('Alpine Scope Cat');
    const groupName = `Alpine Scope Target ${Date.now()}`;
    await apiClient.createGroup({ name: groupName, categoryId: category.ID });

    await page.goto('/category/new');
    await page.waitForLoadState('load');

    const entityInput = page.locator('#tp-entity-group');
    await expect(entityInput).toBeVisible({ timeout: 10000 });
    await expect(entityInput).toHaveValue(groupName, { timeout: 10000 });

    const header = page.locator('.cm-content[aria-label="Custom Header"]');
    await expect(header).toBeVisible({ timeout: 10000 });
    await header.click();
    // Only Alpine can materialize this text: the server returns the markup
    // verbatim, so the assertion proves the frame recreates the display pages'
    // x-data="{ entity: ... }" scope.
    await page.keyboard.type('<div id="scope-probe" x-text="entity.Name"></div>');

    const frame = page.frameLocator('iframe[title="Template slot preview"]');
    await expect(frame.locator('#scope-probe')).toHaveText(groupName, { timeout: 10000 });
  });

  test('the List Header slot previews against the category itself (carrier mode)', async ({
    page,
    apiClient,
  }) => {
    const stamp = Date.now();
    const catName = `Carrier Preview Cat ${stamp}`;
    const category = await apiClient.createCategory(catName);

    // Carrier preview needs a saved category (the create form has no carrier yet).
    await page.goto(`/category/edit?id=${category.ID}`);
    await page.waitForLoadState('load');

    // Switch the slot selector to List Header.
    await page.locator('#tp-slot-group').selectOption('CustomListHeader');

    // The entity picker is irrelevant in carrier mode and is hidden.
    await expect(page.locator('#tp-entity-group')).toBeHidden();

    // Type a property shortcode into the Custom List Header editor.
    const editor = page.locator('.cm-content[aria-label="Custom List Header"]');
    await expect(editor).toBeVisible({ timeout: 10000 });
    await editor.click();
    await page.keyboard.type('CARRIER=[property path="Name"]');

    // The iframe renders [property path="Name"] against the category itself.
    const frame = page.frameLocator('iframe[title="Template slot preview"]');
    await expect(frame.locator('body')).toContainText(`CARRIER=${catName}`, { timeout: 10000 });
  });

  test('the CSS slot previews as a stylesheet, never as body text', async ({
    page,
    apiClient,
  }) => {
    // Production's only sink for a CustomCSS buffer is the {% custom_css %} tag,
    // which writes a <style> element and nothing else. The preview sends that
    // one buffer as both halves of its request, so the response carries it
    // twice, and the frame used to render the markup half into the body as
    // well — printing the stylesheet's own source as page text, which is the
    // one thing saving the template can never produce.
    const stamp = Date.now();
    const marker = `css-slot-applied-${stamp}`;
    // Two rules, because they prove different things. The background pins the
    // frame's internal cascade: its own body reset is preview chrome with no
    // counterpart on a real page, so it is emitted before the author's CSS and
    // must not outrank it. (That is a rule about the chrome, not a claim of
    // parity: a real page's body carries classes that beat a bare `body`
    // selector either way. See _renderFrame.) The ::after content is the marker
    // the body must NOT contain as text -- generated content never reaches
    // textContent, so the only way the marker gets there is the stylesheet
    // being injected as markup as well.
    const css = `body{background-color:rgb(3,5,7)}body::after{content:"${marker}"}`;
    const headerProbe = `header-probe-${stamp}`;
    const category = await apiClient.createCategory(`CSS Slot Cat ${stamp}`, undefined, {
      CustomCSS: css,
      CustomHeader: `<div id="${headerProbe}">rendered header</div>`,
    });
    const groupName = `CSS Slot Target ${stamp}`;
    await apiClient.createGroup({ name: groupName, categoryId: category.ID });

    await page.goto(`/category/edit?id=${category.ID}`);
    await page.waitForLoadState('load');

    const entityInput = page.locator('#tp-entity-group');
    await expect(entityInput).toBeVisible({ timeout: 10000 });
    await expect(entityInput).toHaveValue(groupName, { timeout: 10000 });

    // The pane opens on the Header slot, so the probe marks the first render.
    const frame = page.frameLocator('iframe[title="Template slot preview"]');
    await expect(frame.locator(`#${headerProbe}`)).toHaveCount(1, { timeout: 10000 });

    await page.locator('#tp-slot-group').selectOption('CustomCSS');

    // The probe leaving is what says the CSS-slot render has landed; without it
    // the assertions below could still be reading the header render.
    await expect(frame.locator(`#${headerProbe}`)).toHaveCount(0, { timeout: 10000 });

    // Applied as a stylesheet: the generated content exists only if the rule is
    // live in the frame's cascade.
    await expect
      .poll(
        async () =>
          frame
            .locator('body')
            .evaluate((el) => getComputedStyle(el, '::after').content)
            .catch(() => ''),
        { timeout: 10000 },
      )
      .toContain(marker);

    // ...and the author's own body rule outranks the frame's reset rather than
    // the other way round.
    await expect
      .poll(
        async () =>
          frame
            .locator('body')
            .evaluate((el) => getComputedStyle(el).backgroundColor)
            .catch(() => ''),
        { timeout: 10000 },
      )
      .toBe('rgb(3, 5, 7)');

    // ...and not injected as body content. The marker can only reach textContent
    // if the stylesheet was written into the body as markup as well.
    await expect(frame.locator('body')).not.toContainText(marker);
  });

  test('editing a category only offers entities from that category', async ({
    page,
    apiClient,
  }) => {
    const stamp = Date.now();
    const catA = await apiClient.createCategory(`Scoped Cat A ${stamp}`);
    const catB = await apiClient.createCategory(`Scoped Cat B ${stamp}`);
    const inA = `In Category A ${stamp}`;
    const inB = `In Category B ${stamp}`;
    await apiClient.createGroup({ name: inA, categoryId: catA.ID });
    // Created last, so it is the most recent group overall — an unfiltered
    // default would pick this one.
    await apiClient.createGroup({ name: inB, categoryId: catB.ID });

    await page.goto(`/category/edit?id=${catA.ID}`);
    await page.waitForLoadState('load');

    // The default preview entity must come from category A, not the newer
    // group in category B.
    const entityInput = page.locator('#tp-entity-group');
    await expect(entityInput).toBeVisible({ timeout: 10000 });
    await expect(entityInput).toHaveValue(inA, { timeout: 10000 });

    // Searching by the other category's group name yields no suggestions.
    await entityInput.fill(inB);
    await page.waitForTimeout(600); // debounce + request
    await expect(page.locator('#tp-suggestions-group li')).toHaveCount(0);
  });
});
