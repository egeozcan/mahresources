/**
 * E2E tests for schema-driven metadata display on detail views.
 *
 * Tests that when a category has a MetaSchema and the entity has Meta data,
 * a structured metadata panel appears below the description on detail pages.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Schema metadata display on group detail', () => {
  let categoryId: number;
  let groupId: number;

  const schema = JSON.stringify({
    type: 'object',
    properties: {
      name: { type: 'string', title: 'Full Name', description: 'Legal name of the person' },
      age: { type: 'integer', title: 'Age' },
      status: { type: 'string', enum: ['active', 'inactive', 'pending'] },
      bio: { type: 'string', title: 'Biography' },
      email: { type: 'string', format: 'email', title: 'Email' },
      website: { type: 'string', format: 'uri', title: 'Website' },
      active: { type: 'boolean', title: 'Is Active' },
    },
  });

  const meta = JSON.stringify({
    name: 'Jane Doe',
    age: 30,
    status: 'active',
    bio: 'A photographer and content creator based in Berlin, known for urban landscape photography and creative visual storytelling across multiple platforms.',
    email: 'jane@example.com',
    website: 'https://janedoe.com',
    active: true,
  });

  test.beforeAll(async ({ apiClient }) => {
    const cat = await apiClient.createCategory(
      `Display Test ${Date.now()}`,
      'Category for metadata display tests',
      { MetaSchema: schema },
    );
    categoryId = cat.ID;
    const group = await apiClient.createGroup({
      name: `Display Group ${Date.now()}`,
      categoryId: cat.ID,
      meta,
    });
    groupId = group.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupId) await apiClient.deleteGroup(groupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test('renders metadata panel below description', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const displayEditor = page.locator('schema-editor[mode="display"]');
    await expect(displayEditor).toBeVisible({ timeout: 5000 });
    await expect(displayEditor).toContainText('Metadata');
  });

  test('shows field values from meta data', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const displayEditor = page.locator('schema-editor[mode="display"]');
    await expect(displayEditor).toBeVisible({ timeout: 5000 });
    await expect(displayEditor).toContainText('Jane Doe');
    await expect(displayEditor).toContainText('30');
    await expect(displayEditor).toContainText('active');
  });

  test('uses schema title as label with description tooltip', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const displayEditor = page.locator('schema-editor[mode="display"]');
    await expect(displayEditor).toBeVisible({ timeout: 5000 });
    await expect(displayEditor).toContainText('Full Name');

    const label = displayEditor.locator('[title="Legal name of the person"]');
    await expect(label).toBeVisible();
  });

  test('renders long text in full-width row', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const displayEditor = page.locator('schema-editor[mode="display"]');
    await expect(displayEditor).toBeVisible({ timeout: 5000 });
    await expect(displayEditor).toContainText('photographer and content creator');
  });

  test('renders email as mailto link', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const displayEditor = page.locator('schema-editor[mode="display"]');
    const emailLink = displayEditor.locator('a[href="mailto:jane@example.com"]');
    await expect(emailLink).toBeVisible({ timeout: 5000 });
  });

  test('renders URI as clickable link', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const displayEditor = page.locator('schema-editor[mode="display"]');
    const uriLink = displayEditor.locator('a[href="https://janedoe.com"]');
    await expect(uriLink).toBeVisible({ timeout: 5000 });
  });

  test('renders boolean as Yes/No', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const displayEditor = page.locator('schema-editor[mode="display"]');
    await expect(displayEditor).toContainText('Yes');
  });
});

test.describe('Schema metadata display — empty/missing meta', () => {
  test('no panel when category has schema but group has no meta', async ({ page, apiClient }) => {
    const cat = await apiClient.createCategory(
      `Empty Meta Test ${Date.now()}`,
      'Category with schema but no meta data',
      { MetaSchema: JSON.stringify({ type: 'object', properties: { x: { type: 'string' } } }) },
    );
    const group = await apiClient.createGroup({
      name: `No Meta Group ${Date.now()}`,
      categoryId: cat.ID,
    });

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');

      const displayEditor = page.locator('schema-editor[mode="display"]');
      await expect(displayEditor).not.toBeVisible({ timeout: 3000 });
    } finally {
      await apiClient.deleteGroup(group.ID);
      await apiClient.deleteCategory(cat.ID);
    }
  });

  test('no panel when category has no schema', async ({ page, apiClient }) => {
    const cat = await apiClient.createCategory(
      `No Schema Test ${Date.now()}`,
      'Category without MetaSchema',
    );
    const group = await apiClient.createGroup({
      name: `No Schema Group ${Date.now()}`,
      categoryId: cat.ID,
      meta: JSON.stringify({ foo: 'bar' }),
    });

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');

      const displayEditor = page.locator('schema-editor[mode="display"]');
      await expect(displayEditor).not.toBeVisible({ timeout: 3000 });
    } finally {
      await apiClient.deleteGroup(group.ID);
      await apiClient.deleteCategory(cat.ID);
    }
  });
});

test.describe('Schema metadata display — show/hide empty fields', () => {
  test('hides empty fields by default and shows them on toggle', async ({ page, apiClient }) => {
    const schema = JSON.stringify({
      type: 'object',
      properties: {
        filled: { type: 'string', title: 'Filled Field' },
        empty: { type: 'string', title: 'Empty Field' },
      },
    });
    const cat = await apiClient.createCategory(
      `Toggle Test ${Date.now()}`,
      'Testing show/hide toggle',
      { MetaSchema: schema },
    );
    const group = await apiClient.createGroup({
      name: `Toggle Group ${Date.now()}`,
      categoryId: cat.ID,
      meta: JSON.stringify({ filled: 'has value' }),
    });

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');

      const displayEditor = page.locator('schema-editor[mode="display"]');
      await expect(displayEditor).toBeVisible({ timeout: 5000 });

      await expect(displayEditor).toContainText('has value');
      await expect(displayEditor).not.toContainText('Empty Field');

      const toggleBtn = displayEditor.locator('button', { hasText: /hidden field/ });
      await expect(toggleBtn).toBeVisible();
      await toggleBtn.click();

      await expect(displayEditor).toContainText('Empty Field');
      await expect(displayEditor).toContainText('\u2014');
    } finally {
      await apiClient.deleteGroup(group.ID);
      await apiClient.deleteCategory(cat.ID);
    }
  });
});

test.describe('Schema metadata display — nested objects', () => {
  test('flattens nested objects with dot notation', async ({ page, apiClient }) => {
    const schema = JSON.stringify({
      type: 'object',
      properties: {
        name: { type: 'string' },
        address: {
          type: 'object',
          properties: {
            city: { type: 'string', title: 'City' },
            zip: { type: 'string', title: 'ZIP Code' },
          },
        },
      },
    });
    const cat = await apiClient.createCategory(
      `Nested Test ${Date.now()}`,
      'Testing nested object display',
      { MetaSchema: schema },
    );
    const group = await apiClient.createGroup({
      name: `Nested Group ${Date.now()}`,
      categoryId: cat.ID,
      meta: JSON.stringify({ name: 'Alice', address: { city: 'Berlin', zip: '10115' } }),
    });

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');

      const displayEditor = page.locator('schema-editor[mode="display"]');
      await expect(displayEditor).toBeVisible({ timeout: 5000 });

      await expect(displayEditor).toContainText('Berlin');
      await expect(displayEditor).toContainText('10115');
      await expect(displayEditor).toContainText('City');
    } finally {
      await apiClient.deleteGroup(group.ID);
      await apiClient.deleteCategory(cat.ID);
    }
  });
});
