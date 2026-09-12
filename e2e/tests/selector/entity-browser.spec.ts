import { test, expect } from '../../fixtures/base.fixture';

test('group browsing retains pending choices across pages and nested category filters', async ({ page, apiClient }) => {
 const prefix = `Browse-${Date.now()}`;
 const category = await apiClient.createCategory(prefix);
 const groups = [];
 for (let i = 0; i < 51; i++) groups.push(await apiClient.createGroup({ name: `${prefix} duplicate`, categoryId: category.ID }));
 await page.goto('/resource/new');
 const field = page.locator('[data-selector-field="groups" i]').first();
 await field.getByRole('button', { name: /Browse/ }).click();
 const dialog = page.getByRole('dialog', { name: 'Browse Groups', exact: true });
 await expect(dialog).toBeVisible();
 await dialog.getByLabel('Name', { exact: true }).fill(prefix);
 await expect(dialog.locator('[data-picker-id]')).toHaveCount(50);
 const first = dialog.locator(`[data-picker-id="${groups[0].ID}"]`);
 const popupPromise = page.waitForEvent('popup');await first.getByRole('link').click();
 const popup = await popupPromise;await popup.close();
 await expect(first.getByRole('checkbox')).not.toBeChecked();
 await first.getByRole('checkbox').check();
 await dialog.getByRole('button', { name: 'Next page', exact: true }).click();
 await expect(dialog.locator('[data-picker-id]')).toHaveCount(1);
 await dialog.locator('[data-picker-id]').getByRole('checkbox').check();
 await dialog.getByRole('button', { name: 'Browse Categories', exact: true }).click();
 await expect(page.getByRole('dialog')).toHaveCount(1);
 await page.getByRole('dialog').getByRole('textbox', { name: 'Name', exact: true }).fill(prefix);
 await page.getByRole('dialog').getByRole('checkbox', { name: `Select ${prefix}`, exact: true }).check();
 await page.getByRole('dialog').getByRole('button', { name: 'Confirm selection', exact: true }).click();
 await expect(dialog).toBeVisible();
 await expect(dialog.getByText('2 pending', { exact: true })).toBeVisible();
 await dialog.getByRole('button', { name: 'Confirm selection', exact: true }).click();
 await expect(dialog).not.toBeVisible();
 for (const id of [groups[0].ID, groups[50].ID]) await expect(field.locator(`input[type=hidden][value="${id}"]`)).toHaveCount(1);
});

test('nested Escape restores focus, outer Escape cancels, and single selection requires confirmation', async ({ page, apiClient }) => {
 const category = await apiClient.createCategory(`Browse-cancel-${Date.now()}`);
 const group = await apiClient.createGroup({ name: category.Name, categoryId: category.ID });
 await page.goto('/group/new');
 const owner = page.locator('[data-selector-field="ownerId" i]').first();
 const opener = owner.getByRole('button', { name: /Browse/ });await opener.click();
 const dialog = page.getByRole('dialog');
 const categoryOpener = dialog.getByRole('button', { name: 'Browse Categories', exact: true });await categoryOpener.click();
 await page.keyboard.press('Escape');await expect(categoryOpener).toBeFocused();
 await dialog.getByLabel('Name', { exact: true }).fill(group.Name);
 await dialog.locator(`[data-picker-id="${group.ID}"]`).getByRole('radio').check();
 await expect(owner.locator(`input[type=hidden][value="${group.ID}"]`)).toHaveCount(0);
 await page.keyboard.press('Escape');await expect(dialog).not.toBeVisible();await expect(opener).toBeFocused();
});
