import { test, expect } from '../fixtures/base.fixture';

test.describe('B1: Duplicate error message on 404 pages', () => {
  test('should show the error message exactly once on a 404 page', async ({ page }) => {
    const response = await page.goto('/group?id=99999');
    expect(response?.status()).toBe(404);

    // "record not found" should appear exactly once, not twice
    const errorElements = page.locator('text=/record not found/i');
    await expect(errorElements).toHaveCount(1);
  });

  test('should show the error message exactly once on a 404 tag page', async ({ page }) => {
    const response = await page.goto('/tag?id=99999');
    expect(response?.status()).toBe(404);

    const errorElements = page.locator('text=/record not found/i');
    await expect(errorElements).toHaveCount(1);
  });

  test('should show the error message exactly once on a 404 note page', async ({ page }) => {
    const response = await page.goto('/note?id=99999');
    expect(response?.status()).toBe(404);

    const errorElements = page.locator('text=/record not found/i');
    await expect(errorElements).toHaveCount(1);
  });
});

test.describe('B2: Edit pages for non-existent entities return 404', () => {
  test('tag edit with non-existent ID should return 404', async ({ page }) => {
    const response = await page.goto('/tag/edit?id=99999');
    expect(response?.status()).toBe(404);
    await expect(page.getByText(/record not found/i).first()).toBeVisible();
  });

  test('category edit with non-existent ID should return 404', async ({ page }) => {
    const response = await page.goto('/category/edit?id=99999');
    expect(response?.status()).toBe(404);
    await expect(page.getByText(/record not found/i).first()).toBeVisible();
  });

  test('query edit with non-existent ID should return 404', async ({ page }) => {
    const response = await page.goto('/query/edit?id=99999');
    expect(response?.status()).toBe(404);
    await expect(page.getByText(/record not found/i).first()).toBeVisible();
  });

  test('noteType edit with non-existent ID should return 404', async ({ page }) => {
    const response = await page.goto('/noteType/edit?id=99999');
    expect(response?.status()).toBe(404);
    await expect(page.getByText(/record not found/i).first()).toBeVisible();
  });

  test('resourceCategory edit with non-existent ID should return 404', async ({ page }) => {
    const response = await page.goto('/resourceCategory/edit?id=99999');
    expect(response?.status()).toBe(404);
    await expect(page.getByText(/record not found/i).first()).toBeVisible();
  });

  test('tag edit with non-existent ID should NOT show create form', async ({ page }) => {
    await page.goto('/tag/edit?id=99999');
    // The save/submit button for a create form should NOT be present
    await expect(page.locator('input[type="submit"], button[type="submit"]')).toHaveCount(0);
  });
});

test.describe('B3: Detail pages return 400 (not 500) for non-numeric ID', () => {
  test('group detail with id=abc should return 400', async ({ page }) => {
    const response = await page.goto('/group?id=abc');
    expect(response?.status()).toBe(400);
  });

  test('tag detail with id=abc should return 400', async ({ page }) => {
    const response = await page.goto('/tag?id=abc');
    expect(response?.status()).toBe(400);
  });

  test('note detail with id=abc should return 400', async ({ page }) => {
    const response = await page.goto('/note?id=abc');
    expect(response?.status()).toBe(400);
  });

  test('resource detail with id=abc should return 400', async ({ page }) => {
    const response = await page.goto('/resource?id=abc');
    expect(response?.status()).toBe(400);
  });

  test('category detail with id=abc should return 400', async ({ page }) => {
    const response = await page.goto('/category?id=abc');
    expect(response?.status()).toBe(400);
  });

  test('query detail with id=abc should return 400', async ({ page }) => {
    const response = await page.goto('/query?id=abc');
    expect(response?.status()).toBe(400);
  });

  test('non-numeric ID should show error page, not crash', async ({ page }) => {
    const response = await page.goto('/group?id=abc');
    expect(response?.status()).toBe(400);
    // Should have navigation (styled error page, not a raw 500)
    await expect(page.locator('nav')).toBeVisible();
  });
});

test.describe('B4: List pages return 400 (not 500) for non-numeric filter params', () => {
  test('groups list with Tags=abc should return 400', async ({ page }) => {
    const response = await page.goto('/groups?Tags=abc');
    expect(response?.status()).toBe(400);
  });

  test('notes list with Tags=abc should return 400', async ({ page }) => {
    const response = await page.goto('/notes?Tags=abc');
    expect(response?.status()).toBe(400);
  });

  test('resources list with Tags=abc should return 400', async ({ page }) => {
    const response = await page.goto('/resources?Tags=abc');
    expect(response?.status()).toBe(400);
  });

  test('groups list with OwnerId=abc should return 400', async ({ page }) => {
    const response = await page.goto('/groups?OwnerId=abc');
    expect(response?.status()).toBe(400);
  });

  test('notes list with Groups=abc should return 400', async ({ page }) => {
    const response = await page.goto('/notes?Groups=abc');
    expect(response?.status()).toBe(400);
  });

  test('non-numeric filter should show error page, not crash', async ({ page }) => {
    const response = await page.goto('/groups?Tags=abc');
    expect(response?.status()).toBe(400);
    await expect(page.locator('nav')).toBeVisible();
  });
});
