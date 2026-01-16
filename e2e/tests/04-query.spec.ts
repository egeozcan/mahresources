import { test, expect } from '../fixtures/base.fixture';

test.describe('Query CRUD Operations', () => {
  let createdQueryId: number;

  test('should create a new query', async ({ queryPage }) => {
    createdQueryId = await queryPage.create({
      name: 'E2E Test Query',
      text: 'SELECT * FROM tags LIMIT 10',
      description: 'Query created by E2E tests',
    });
    expect(createdQueryId).toBeGreaterThan(0);
  });

  test('should display the created query', async ({ queryPage, page }) => {
    expect(createdQueryId, 'Query must be created first').toBeGreaterThan(0);
    await queryPage.gotoDisplay(createdQueryId);
    await expect(page.locator('h1, .title')).toContainText('E2E Test Query');
    await expect(page.locator('text=SELECT * FROM tags LIMIT 10')).toBeVisible();
  });

  test('should update the query', async ({ queryPage, page }) => {
    expect(createdQueryId, 'Query must be created first').toBeGreaterThan(0);
    await queryPage.update(createdQueryId, {
      name: 'Updated E2E Query',
      text: 'SELECT * FROM categories LIMIT 5',
      description: 'Updated query description',
    });
    await expect(page.locator('h1, .title')).toContainText('Updated E2E Query');
  });

  test('should list the query', async ({ queryPage }) => {
    await queryPage.verifyQueryInList('Updated E2E Query');
  });

  test('should delete the query', async ({ queryPage }) => {
    expect(createdQueryId, 'Query must be created first').toBeGreaterThan(0);
    await queryPage.delete(createdQueryId);
    await queryPage.verifyQueryNotInList('Updated E2E Query');
  });
});

test.describe('Query with Template', () => {
  let queryWithTemplateId: number;

  test('should create query with template', async ({ queryPage }) => {
    queryWithTemplateId = await queryPage.create({
      name: 'Templated Query',
      text: 'SELECT id, name, description FROM groups WHERE id = {{ id }}',
      description: 'Query with parameter template',
      template: '{"id": 1}',
    });
    expect(queryWithTemplateId).toBeGreaterThan(0);
  });

  test.afterAll(async ({ apiClient }) => {
    if (queryWithTemplateId) {
      await apiClient.deleteQuery(queryWithTemplateId);
    }
  });
});

test.describe('Query Validation', () => {
  test('should require name and text fields', async ({ queryPage, page }) => {
    await queryPage.gotoNew();
    await queryPage.save();
    // HTML5 required validation prevents submission
    await expect(page).toHaveURL(/\/query\/new/);
  });
});
