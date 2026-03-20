import { test, expect, getWorkerBaseUrl } from '../fixtures/base.fixture';

/**
 * Bug: When creating a relation fails with a validation error (e.g. "both
 * groups and the relation type must have categories assigned"), the server
 * redirects back to /relation/new with FromGroupId, ToGroupId, and
 * GroupRelationTypeId preserved in the URL query parameters. However, the
 * Name and Description text fields are NOT included in the redirect URL,
 * so the user's typed text is silently lost.
 *
 * Expected: After a relation creation error, the Name and Description values
 * the user typed should be preserved so they can fix the issue and resubmit
 * without retyping.
 */
test.describe('Relation creation error preserves Name field', () => {
  let groupAId: number;
  let groupBId: number;

  test.beforeAll(async ({ request }) => {
    const baseUrl = getWorkerBaseUrl();
    const runId = Date.now();

    // Create two groups WITHOUT categories via direct API calls.
    // The apiClient.createGroup helper requires categoryId, but we need
    // uncategorized groups to trigger "must have categories assigned" error.
    const resA = await request.post(`${baseUrl}/v1/group`, {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: `name=NoCatGroupA-${runId}&Description=Group+without+category`,
    });
    const gA = await resA.json();
    groupAId = gA.ID;

    const resB = await request.post(`${baseUrl}/v1/group`, {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: `name=NoCatGroupB-${runId}&Description=Group+without+category`,
    });
    const gB = await resB.json();
    groupBId = gB.ID;
  });

  test('Name field should be preserved after relation creation error', async ({
    page,
  }) => {
    // Navigate to the create relation page pre-populated with the "from" group
    await page.goto(`/relation/new?FromGroupId=${groupAId}`);
    await page.waitForLoadState('load');

    // Fill in the Name field
    const nameInput = page.getByRole('textbox', { name: 'Name' });
    await nameInput.fill('My Important Relation');

    // Fill in the Description field
    const descInput = page.getByRole('textbox', { name: 'Description' });
    await descInput.fill('Detailed description of this relation');

    // Select a relation type via the autocomplete
    const typeInput = page.getByRole('combobox', { name: 'Type' });
    await typeInput.fill('Addr');
    await page
      .getByRole('option')
      .filter({ hasText: 'Address' })
      .first()
      .click();

    // Select the "To Group" via the autocomplete
    const toGroupInput = page.getByRole('combobox', { name: 'To Group' });
    await toGroupInput.fill('NoCatGroup');
    await page
      .getByRole('option')
      .filter({ hasText: 'NoCatGroupB' })
      .first()
      .click();

    // Submit the form - should fail because groups have no categories
    await page.getByRole('button', { name: 'Save' }).click();

    // Wait for the redirect back to the form with the error
    await page.waitForURL(/relation\/new.*Error=/);

    // The error message should be visible
    await expect(page.getByText(/must have categories/)).toBeVisible();

    // BUG: The Name field should still contain the user's input.
    // Currently the redirect URL does not include Name/Description params,
    // so these values are lost on page reload.
    await expect(nameInput).toHaveValue('My Important Relation');

    // The Description field should also be preserved
    await expect(descInput).toHaveValue(
      'Detailed description of this relation'
    );
  });

  test.afterAll(async ({ request }) => {
    const baseUrl = getWorkerBaseUrl();
    try {
      await request.delete(`${baseUrl}/v1/group?id=${groupAId}`);
    } catch {
      /* ignore */
    }
    try {
      await request.delete(`${baseUrl}/v1/group?id=${groupBId}`);
    } catch {
      /* ignore */
    }
  });
});
