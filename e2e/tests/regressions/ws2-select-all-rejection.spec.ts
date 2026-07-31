/**
 * UI bug hunt 2026-07-29, finding 79 (WS2). **79 is REJECTED**, and this file is
 * the control that pins the rejection.
 *
 * The report says the grid selection checkboxes "desynchronise from the store":
 * that a first "Select All" click left **zero** checkboxes checked even though
 * the bulk toolbar appeared, and that a page carries two checked checkboxes that
 * are invisible and have no accessible name. Batch 4 re-ran the report's own
 * protocol nine times, filtered and unfiltered, plus six race-timed runs clicking
 * Select All the instant the node exists, and the checkboxes tracked the store
 * exactly every time. Both halves of the evidence are artefacts of how it was
 * measured, and both are reproduced here so the next reader of that evidence does
 * not re-open the finding:
 *
 * 1. `button:has-text('Select All')` is a **substring, case-insensitive** match, so
 *    it also matches **"De-select All"**. Three buttons carry it — the standalone
 *    Select All shown while nothing is ticked, the toolbar's Deselect All, and the
 *    toolbar's Select All — and `templates/partials/bulkEditorResource.tpl` emits
 *    them in that DOM order. Taking that locator at `nth=1` therefore addresses
 *    Deselect All, which lives in the `x-show`-collapsed toolbar and is hidden at
 *    the moment of the first click. The click times out on a hidden element,
 *    nothing is ticked, and the toolbar the *real* Select All opened is still on
 *    screen — which is the reported `{"checked":0,"toolbar":true}` shape exactly,
 *    with the app behaving correctly.
 * 2. The two checked, invisible, unnamed checkboxes are the header settings
 *    toggles (`layouts/base.tpl`: `showDescriptions` and `showHoverPreviews`).
 *    They live inside the collapsed gear dropdown, carry no class and no
 *    `aria-label`, and are both on by default.
 *
 * The subject assertion is the one that would fail if the finding were real: after
 * the real Select All, every row checkbox is checked **and** the store holds
 * exactly that many ids. The precondition (rows exist, nothing is checked, the
 * store is empty) is asserted first, so "all rows are checked" can never be
 * satisfied by an empty list.
 */
import path from 'path';
import { test, expect } from '../../fixtures/base.fixture';
import { uniqueAssetFile } from '../../helpers/unique-upload';

const IMAGE = path.join(__dirname, '../../test-assets/sample-image.png');

/** The number of ids the bulk-selection store currently holds. */
const storeCount = (page: import('@playwright/test').Page) =>
  page.evaluate(() => (window as any).Alpine.store('bulkSelection').selectedIds.size as number);

test.describe('finding 79 (rejected) — the grid checkboxes track the bulk-selection store', () => {
  let runId: string;
  let ownerGroupId: number;
  const RESOURCE_COUNT = 3;

  test.beforeAll(async ({ apiClient }) => {
    runId = `${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;
    const category = await apiClient.createCategory(`WS2 SelectAll Cat ${runId}`, 'finding 79');
    const owner = await apiClient.createGroup({
      name: `WS2 SelectAll Owner ${runId}`,
      categoryId: category.ID,
    });
    ownerGroupId = owner.ID;

    // Own the data. The worker server is shared by the whole suite, so an
    // unscoped /resources cannot support an exact-count assertion.
    for (let i = 1; i <= RESOURCE_COUNT; i++) {
      await apiClient.createResource({
        filePath: uniqueAssetFile(IMAGE),
        name: `WS2 SelectAll Resource ${i} ${runId}`,
        ownerId: ownerGroupId,
      });
    }
  });

  test('Select All checks every row, and the store agrees', async ({ page }) => {
    const pageErrors: string[] = [];
    page.on('pageerror', (e) => pageErrors.push(e.message));

    await page.goto(`/resources?OwnerId=${ownerGroupId}`);

    const rowBoxes = page.locator('input.card-checkbox');
    await expect(rowBoxes).toHaveCount(RESOURCE_COUNT);

    // Precondition. Without this, "every row is checked" passes on an empty list.
    expect(await rowBoxes.evaluateAll((els) => els.filter((e) => (e as HTMLInputElement).checked).length)).toBe(0);
    expect(await storeCount(page)).toBe(0);

    // The *real* control: the visible one, addressed by the hook the app gives it
    // rather than by its label text — which is the whole point of this file.
    const selectAll = page.locator('button[data-bulk-select-all]:visible');
    await expect(selectAll).toHaveCount(1);
    await selectAll.click();

    /**
     * `expect.poll` returns on the first matching sample, so it cannot assert
     * where something ends up — but here it is only waiting for Alpine to flush
     * the `:checked` bindings, and the assertion that decides the test is the
     * single synchronous read of the store on the line after it. If a later
     * sample diverged, that read would catch it.
     */
    await expect
      .poll(async () => rowBoxes.evaluateAll((els) => els.filter((e) => (e as HTMLInputElement).checked).length))
      .toBe(RESOURCE_COUNT);
    expect(await storeCount(page)).toBe(RESOURCE_COUNT);

    // The toolbar reacted, which is the half of the report's evidence that was
    // always true.
    await expect(page.locator('.bulk-editors')).toBeVisible();

    // ...and it still tracks on the way back down.
    await page.locator('button:visible', { hasText: /^\s*Deselect All\s*$/i }).first().click();
    await expect
      .poll(async () => rowBoxes.evaluateAll((els) => els.filter((e) => (e as HTMLInputElement).checked).length))
      .toBe(0);
    expect(await storeCount(page)).toBe(0);

    expect(pageErrors, 'the page threw while selecting').toEqual([]);
  });

  test("the report's `nth=1` Select All locator addresses a hidden De-select All", async ({ page }) => {
    await page.goto(`/resources?OwnerId=${ownerGroupId}`);
    await expect(page.locator('input.card-checkbox')).toHaveCount(RESOURCE_COUNT);

    // has-text is a case-insensitive substring match, so "De-select All" matches
    // "select All". This is the locator the report's probe used.
    const bySubstring = page.locator('button:has-text("Select All")');
    expect(await bySubstring.count()).toBeGreaterThan(1);

    const labels = await bySubstring.evaluateAll((els) => els.map((e) => (e.textContent || '').trim()));
    expect(labels[1], `DOM order changed: ${JSON.stringify(labels)}`).toMatch(/^Deselect All$/i);

    // ...and at the moment of a first click it is hidden, so the probe's click
    // could never have ticked anything.
    await expect(bySubstring.nth(1)).toBeHidden();
    expect(await storeCount(page)).toBe(0);
  });

  test("the page's two unnamed checked checkboxes are the header settings toggles", async ({ page }) => {
    await page.goto(`/resources?OwnerId=${ownerGroupId}`);
    await expect(page.locator('input.card-checkbox')).toHaveCount(RESOURCE_COUNT);

    // The report's probe collected checkboxes with no class and no aria-label —
    // the two properties its evidence quotes. Reproduced exactly.
    const probed = await page.evaluate(() =>
      [...document.querySelectorAll('input[type=checkbox]')]
        .filter((e) => {
          const el = e as HTMLInputElement;
          return el.checked && !el.className && !el.getAttribute('aria-label');
        })
        .map((e) => ({
          name: (e as HTMLInputElement).name,
          visible: (e as HTMLElement).offsetParent !== null,
          // ...and they are *not* nameless: each sits inside a <label> with
          // visible text, which is why this is a rejection and not a defect.
          labelText: (e.closest('label')?.textContent || '').trim(),
        })),
    );

    // The report counted two. They are these two, they are hidden inside the
    // collapsed gear dropdown, and they are on by default.
    expect(probed.map((c) => c.name).sort()).toEqual(['showDescriptions', 'showHoverPreviews']);
    expect(probed.every((c) => !c.visible)).toBe(true);
    expect(probed.map((c) => c.labelText).sort()).toEqual(['Show Descriptions', 'Show Hover Previews']);

    // Positive control: the row checkboxes on the same page *are* named, so
    // "nothing on this page has a name" cannot be what the probe saw.
    const namedRows = await page.locator('input.card-checkbox').evaluateAll((els) =>
      els.filter((e) => (e.getAttribute('aria-label') || '').startsWith('Select ')).length,
    );
    expect(namedRows).toBe(RESOURCE_COUNT);
  });
});
