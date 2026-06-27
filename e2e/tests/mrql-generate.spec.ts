import { test, expect } from '../fixtures/base.fixture';
import { MRQLPage } from '../pages/MRQLPage';

test.describe('MRQL generation', () => {
  test('generates a valid query into the editor with explanation', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await page.route('/v1/mrql/generate', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          query: 'type = resource AND contentType ~ "image/*" LIMIT 50',
          explanation: 'Finds up to 50 image resources.',
          valid: true,
          errors: [],
        }),
      });
    });

    await mrql.navigate();
    await mrql.enterGenerationPrompt('show image resources');
    await mrql.generateMRQL();

    await expect(mrql.generationExplanation).toContainText('Finds up to 50 image resources.');
    await expect.poll(() => mrql.getEditorContent()).toBe('type = resource AND contentType ~ "image/*" LIMIT 50');
  });

  test('invalid generation stays out of the editor until explicitly applied', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await page.route('/v1/mrql/generate', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          query: 'type = resource LIMIT 1000000',
          explanation: 'Too broad.',
          valid: false,
          errors: [{ message: 'LIMIT must be between 1 and 500' }],
        }),
      });
    });

    await mrql.navigate();
    await mrql.enterQuery('name ~ "keep-me"');
    await mrql.enterGenerationPrompt('all resources');
    await mrql.generateMRQL();

    await expect(mrql.generationError).toContainText('LIMIT must be between 1 and 500');
    await expect.poll(() => mrql.getEditorContent()).toBe('name ~ "keep-me"');

    await mrql.useGeneratedQuery();
    await expect.poll(() => mrql.getEditorContent()).toBe('type = resource LIMIT 1000000');
  });

  test('provider error leaves editor content unchanged', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await page.route('/v1/mrql/generate', async (route) => {
      await route.fulfill({
        status: 503,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'MRQL generation is not configured' }),
      });
    });

    await mrql.navigate();
    await mrql.enterQuery('name ~ "keep-me"');
    await mrql.enterGenerationPrompt('show resources');
    await mrql.generateMRQL();

    await expect(mrql.generationError).toContainText('MRQL generation is not configured');
    await expect.poll(() => mrql.getEditorContent()).toBe('name ~ "keep-me"');
  });

  test('generated query clears saved-query update affordance and stale results', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await page.route('/v1/mrql/generate', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          query: 'type = note LIMIT 50',
          explanation: 'Finds notes.',
          valid: true,
          errors: [],
        }),
      });
    });

    await mrql.navigate();
    const queryName = `Generation Reset ${Date.now()}`;
    await mrql.enterQuery('name ~ "test"');
    await mrql.saveQuery(queryName);
    await mrql.loadSavedQuery(queryName);
    await mrql.executeQuery();
    await expect(mrql.resultsSection.locator('h2')).toBeVisible();

    await mrql.enterGenerationPrompt('show notes');
    await mrql.generateMRQL();

    await expect(page.locator('[data-testid="mrql-update-button"]')).toBeHidden();
    await expect(mrql.resultsSection.locator('h2')).toHaveCount(0);
  });
});
