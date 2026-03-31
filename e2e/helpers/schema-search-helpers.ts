/**
 * Shared helpers for schema-driven search field tests.
 *
 * Used by both the functional spec (tests/schema-search-fields.spec.ts) and
 * the accessibility spec (tests/accessibility/schema-search-a11y.spec.ts).
 */

/**
 * Select a value from the Categories autocompleter on the groups list page.
 * The autocompleter is labelled "Categories".
 */
export async function selectGroupCategory(page: any, searchText: string) {
  const input = page.getByRole('combobox', { name: 'Categories' });
  await input.click();
  await input.fill(searchText);
  const option = page.locator(`div[role="option"]:visible:has-text("${searchText}")`).first();
  await option.waitFor({ timeout: 10000 });
  await option.click();
  await option.waitFor({ state: 'hidden', timeout: 3000 }).catch(() => {});
  // Give Alpine time to propagate the selection and re-render schema fields
  await page.waitForTimeout(300);
}

/**
 * Remove a previously-selected category chip by clicking its remove button.
 */
export async function removeGroupCategory(page: any, categoryName: string) {
  const removeBtn = page
    .locator(`[x-data*="autocompleter"] button[aria-label="Remove ${categoryName}"]`)
    .first();
  await removeBtn.click();
  // Give Alpine time to re-render
  await page.waitForTimeout(200);
}

/**
 * Select a value from the Resource Category autocompleter on the resources list page.
 */
export async function selectResourceCategory(page: any, searchText: string) {
  const input = page.getByRole('combobox', { name: 'Resource Category' });
  await input.click();
  await input.fill(searchText);
  const option = page.locator(`div[role="option"]:visible:has-text("${searchText}")`).first();
  await option.waitFor({ timeout: 10000 });
  await option.click();
  await option.waitFor({ state: 'hidden', timeout: 3000 }).catch(() => {});
  // Give Alpine time to propagate and render schema fields
  await page.waitForTimeout(300);
}

/**
 * Returns a locator for the schema fields container — anchored by
 * role="group" / aria-label="Schema fields".
 */
export function schemaFieldsGroup(page: any) {
  return page.locator('[role="group"][aria-label="Schema fields"]');
}

/**
 * Submit the filter form on a list page. Finds the submit button within the
 * sidebar filter form specifically (using aria-label) to avoid ambiguity.
 */
export async function submitFilterForm(page: any, formAriaLabel = 'Filter groups') {
  const form = page.locator(`form[aria-label="${formAriaLabel}"]`);
  const submitBtn = form.getByRole('button', { name: 'Apply Filters' });
  await submitBtn.scrollIntoViewIfNeeded();
  await submitBtn.click();
  await page.waitForLoadState('load');
}
