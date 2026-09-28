/**
 * A resource card's meta line names the resource's owner and its category, and
 * either name can be one long unbroken string: a URL, a hash, a generated id.
 * The meta items do not wrap, so such a name pushed the card past a phone's
 * viewport, where html/body clip it away. Which cards a list shows depends on
 * what else a worker's server holds, so the mobile-overflow sweeps caught this
 * only when another spec had left such a name behind.
 */
import path from 'path';
import { test, expect } from '../../fixtures/base.fixture';

test.use({ viewport: { width: 390, height: 844 } });

test('a long unbroken owner or category name stays inside a resource card at 390px', async ({ page, apiClient }) => {
  const stamp = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  const category = await apiClient.createCategory(`card-meta-category-${stamp}`);
  const owner = await apiClient.createGroup({ name: `owner-${'a'.repeat(72)}-${stamp}`, categoryId: category.ID });
  const resourceCategory = await apiClient.createResourceCategory(`rc-${'b'.repeat(72)}-${stamp}`);
  const name = `card-meta-wrap-${stamp}`;
  const resource = await apiClient.createResource({
    filePath: path.join(__dirname, '../../test-assets/sample-image-10.png'),
    name,
    ownerId: owner.ID,
    resourceCategoryId: resourceCategory.ID,
  });

  try {
    await page.goto(`/resources?Name=${encodeURIComponent(name)}`);
    const card = page.locator('.card', { has: page.locator(`a[href="/resource?id=${resource.ID}"]`) }).first();
    await expect(card.locator('.card-meta-link')).toHaveCount(2);

    const measured = await page.evaluate(() => ({
      bodyScroll: document.body.scrollWidth,
      inner: window.innerWidth,
      links: Array.from(document.querySelectorAll<HTMLElement>('.card-meta-link'))
        .map((el) => Math.round(el.getBoundingClientRect().right)),
      offenders: Array.from(document.querySelectorAll<HTMLElement>('body *'))
        .filter((el) => el.getBoundingClientRect().right > window.innerWidth + 1 && el.getBoundingClientRect().width > 0)
        .slice(0, 8)
        .map((el) => `${el.tagName}.${(el.className || '').toString().split(' ').join('.')} right=${Math.round(el.getBoundingClientRect().right)}`),
    }));
    expect(measured.bodyScroll, `the page is wider than the viewport: ${measured.offenders.join(' | ')}`).toBeLessThanOrEqual(measured.inner);
    for (const right of measured.links) {
      expect(right, 'a meta link ends past the viewport').toBeLessThanOrEqual(measured.inner);
    }
  } finally {
    await apiClient.deleteResource(resource.ID).catch(() => {});
    await apiClient.deleteGroup(owner.ID).catch(() => {});
    await apiClient.deleteResourceCategory(resourceCategory.ID).catch(() => {});
    await apiClient.deleteCategory(category.ID).catch(() => {});
  }
});
