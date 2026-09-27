import { test, expect } from '../../fixtures/a11y.fixture';

// A shortcut pressed inside a dialog it will not open over says why, inside that
// dialog; and the search field shows where focus is.

test.describe('A shortcut inside another dialog', () => {
  test('the Jobs shortcut inside search says, inside search, why Jobs did not open', async ({ page }) => {
    await page.goto('/dashboard');
    await page.keyboard.press('ControlOrMeta+k');
    const search = page.getByRole('dialog', { name: 'Search' });
    await expect(search).toBeVisible();
    const input = search.getByRole('combobox', { name: 'Search' });
    await expect(input).toBeFocused();

    await page.keyboard.press('Control+Shift+D');
    await expect(page.getByRole('dialog', { name: 'Jobs' })).toHaveCount(0);
    const refusal = search.getByRole('status');
    await expect(refusal).toHaveText('Close this dialog first to open Jobs.');
    await expect(refusal).toBeVisible();
    await expect(input).toBeFocused();
  });

  test('at 400% zoom the refusal is on screen, however the dialog is scrolled', async ({ page }) => {
    await page.setViewportSize({ width: 320, height: 180 });
    await page.goto('/dashboard');
    await page.keyboard.press('ControlOrMeta+k');
    const search = page.getByRole('dialog', { name: 'Search' });
    await expect(search.getByRole('combobox', { name: 'Search' })).toBeFocused();
    await page.keyboard.press('Control+Shift+D');
    const refusal = search.getByRole('status');
    await expect(refusal).toHaveText('Close this dialog first to open Jobs.');
    const box = await refusal.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.y).toBeGreaterThanOrEqual(0);
    expect(box!.y + box!.height).toBeLessThanOrEqual(180);
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(320);
  });

  test('the search field shows where focus is', async ({ page }) => {
    await page.goto('/dashboard');
    await page.keyboard.press('ControlOrMeta+k');
    const input = page.getByRole('dialog', { name: 'Search' }).getByRole('combobox', { name: 'Search' });
    await expect(input).toBeFocused();
    const shadow = await input.evaluate(el => getComputedStyle(el).boxShadow);
    expect(shadow).not.toBe('none');
  });
});
