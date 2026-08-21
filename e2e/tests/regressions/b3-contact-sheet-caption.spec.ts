/**
 * B3, from the open-work board's off-board section.
 *
 * `.simple :is(.description, h2, h4) { display: none }` carried the comment
 * "hide page-level chrome (keep card-header for overlay)". `.simple` is set on
 * <body>, so it was not page-level: rendered, the contact sheet carries eight
 * matching elements and none of them is that chrome. Seven are headings of
 * overlays that live in the base layout — the jobs cockpit, the lightbox's
 * Edit Tags / Info / Crop panels, paste upload, the entity picker and the
 * confirm dialog — four of which are `aria-labelledby` targets, so a sighted
 * user saw a titleless dialog while a screen reader still announced its name.
 * The eighth is `.card-title`, the caption the mode exists to overlay, styled
 * for it in the very next rule and never once rendered.
 *
 * Two assertions, because the fix has two halves that fail independently: the
 * caption exists at all, and a heading inside an overlay is not blanked.
 */
import { test, expect } from '../../fixtures/base.fixture';

test.describe('B3 — the contact sheet caption and the headings the same rule blanked', () => {
  let resourceId: number;
  const resourceName = `contact-sheet-${Date.now()}.txt`;

  test.beforeAll(async ({ request }) => {
    const form = new FormData();
    form.append('resource', new Blob([`b3 caption body ${Date.now()}`]), resourceName);
    form.append('Name', resourceName);
    form.append('Meta', '{}');
    const res = await request.post('/v1/resource', { multipart: { resource: { name: resourceName, mimeType: 'text/plain', buffer: Buffer.from(`b3 caption body ${Date.now()}`) }, Name: resourceName, Meta: '{}' } });
    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    resourceId = Array.isArray(body) ? body[0].ID : body.ID;
  });

  test.afterAll(async ({ request }) => {
    if (resourceId) await request.post(`/v1/resource/delete?id=${resourceId}`);
  });

  test('the hover caption carries the resource name, as a link', async ({ page }) => {
    await page.goto('/resources/simple');

    const title = page.locator('.simple .card-title').first();
    await expect(title).toHaveCount(1);

    // display:none would make this 0x0 and the text unreadable. The caption is
    // revealed on hover, so it is in the layout at all times — what the bug
    // removed was the box itself.
    const box = await title.boundingBox();
    expect(box, 'the card title has no layout box: the hammer rule is back').not.toBeNull();
    expect(box!.height).toBeGreaterThan(0);

    await expect(title.locator('a')).toHaveAttribute('href', `/resource?id=${resourceId}`);
  });

  test('a heading anywhere on this page is not blanked by the contact-sheet rule', async ({ page }) => {
    await page.goto('/resources/simple');

    // The seven overlay headings the rule hid live behind `x-if`, so they are
    // not in the DOM until their dialog opens — asserting on one of them would
    // pin that dialog's lifecycle rather than the CSS. The rule itself is what
    // regressed and what matters: it was a descendant selector from `.simple`
    // on <body>, so it reached every h2 and h4 in the document. Insert one and
    // read the computed style, which pins exactly that.
    const displays = await page.evaluate(() => {
      const probe = document.createElement('div');
      probe.innerHTML = '<h2>probe</h2><h4>probe</h4><p class="description">probe</p>';
      document.body.appendChild(probe);
      const read = (sel: string) =>
        getComputedStyle(probe.querySelector(sel) as Element).display;
      const out = { h2: read('h2'), h4: read('h4'), description: read('.description') };
      probe.remove();
      return out;
    });

    expect(await page.locator('body.simple').count(), 'not in contact-sheet mode').toBe(1);
    expect(displays.h2, 'an h2 on the contact sheet is display:none').not.toBe('none');
    expect(displays.h4, 'an h4 on the contact sheet is display:none').not.toBe('none');
    expect(displays.description, 'a .description on the contact sheet is display:none').not.toBe('none');
  });
});
