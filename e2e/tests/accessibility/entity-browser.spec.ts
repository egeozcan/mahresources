import { test, expect } from '../../fixtures/a11y.fixture';

test('entity browse icons, nested filters and long result names are accessible', async ({ page, apiClient, checkA11y }) => {
 const name = `Accessible picker ${Date.now()} ${'long identifying name '.repeat(8)}`.trimEnd();
 const category = await apiClient.createCategory(name);
 const group = await apiClient.createGroup({ name, categoryId: category.ID });
 await page.goto('/resource/new');
 const opener = page.locator('[data-selector-field="groups" i]').getByRole('button', { name: 'Browse Groups', exact: true });
 await opener.focus();await page.keyboard.press('Enter');
 const dialog = page.locator('[aria-labelledby="entity-picker-title"]');
 const search = dialog.getByRole('textbox', { name: 'Name', exact: true });await expect(search).toBeFocused();await search.fill(name);
 const choice = dialog.locator(`[data-picker-id="${group.ID}"]`).getByRole('checkbox');await choice.check();await expect(choice).toBeChecked();
 await dialog.getByRole('button', { name: 'More filters', exact: true }).click();
 await dialog.getByRole('button', { name: 'Add metadata filter', exact: true }).click();
 await checkA11y({ include: ['[aria-labelledby="entity-picker-title"]'] });
 const child = dialog.getByRole('button', { name: 'Browse Categories', exact: true });await child.focus();await page.keyboard.press('Enter');
 await expect(dialog.getByRole('heading', { name: 'Browse Categories', exact: true })).toBeVisible();
 await checkA11y({ include: ['[aria-labelledby="entity-picker-title"]'] });
 await page.keyboard.press('Escape');await expect(child).toBeFocused();
 await page.keyboard.press('Escape');await expect(opener).toBeFocused();
});

test('single-selection radio and pending state are accessible without changing the origin', async ({ page, apiClient, checkA11y }) => {
 const name = `Accessible single picker ${Date.now()}`;
 const category = await apiClient.createCategory(name);
 const group = await apiClient.createGroup({ name, categoryId: category.ID });
 await page.goto('/group/new');
 const origin = page.locator('[data-selector-field="ownerId" i]');await origin.getByRole('button', { name: 'Browse Owner', exact: true }).click();
 const dialog = page.locator('[aria-labelledby="entity-picker-title"]');await dialog.getByRole('textbox', { name: 'Name', exact: true }).fill(name);
 const radio = dialog.getByRole('radio', { name: `Select ${name}`, exact: true });await radio.check();await expect(radio).toBeChecked();
 await expect(origin.locator(`input[type=hidden][value="${group.ID}"]`)).toHaveCount(0);
 await checkA11y({ include: ['[aria-labelledby="entity-picker-title"]'] });
 await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
});
