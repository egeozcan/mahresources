import { test, expect } from '../fixtures/base.fixture';

test.describe.serial('Group Compare', () => {
  const testRunId = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;

  let categoryId: number;
  let relationTypeId: number;
  let tagLeftId: number;
  let tagSharedId: number;
  let tagRightId: number;

  let leftGroupId: number;
  let rightGroupId: number;
  let thirdGroupId: number;

  let relatedSharedId: number;
  let relatedLeftId: number;
  let relatedRightId: number;
  let relationTargetSharedId: number;
  let relationTargetLeftId: number;
  let relationTargetRightId: number;
  let leftChildId: number;
  let rightChildId: number;

  const leftGroupName = `Compare Left ${testRunId}`;
  const rightGroupName = `Compare Right ${testRunId}`;
  const thirdGroupName = `Compare Third ${testRunId}`;

  async function selectGroupByName(page: import('@playwright/test').Page, groupName: string) {
    const checkbox = page.locator(`article.group-card:has(a:has-text("${groupName}")) input[type="checkbox"]`).first();
    await expect(checkbox).toBeVisible({ timeout: 10000 });
    await checkbox.check();
  }

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(`Compare Category ${testRunId}`, 'Category for group compare tests');
    categoryId = category.ID;

    const tagLeft = await apiClient.createTag(`Compare Tag Left ${testRunId}`);
    const tagShared = await apiClient.createTag(`Compare Tag Shared ${testRunId}`);
    const tagRight = await apiClient.createTag(`Compare Tag Right ${testRunId}`);
    tagLeftId = tagLeft.ID;
    tagSharedId = tagShared.ID;
    tagRightId = tagRight.ID;

    const relatedShared = await apiClient.createGroup({
      name: `Compare Related Shared ${testRunId}`,
      categoryId,
    });
    const relatedLeft = await apiClient.createGroup({
      name: `Compare Related Left ${testRunId}`,
      categoryId,
    });
    const relatedRight = await apiClient.createGroup({
      name: `Compare Related Right ${testRunId}`,
      categoryId,
    });
    relatedSharedId = relatedShared.ID;
    relatedLeftId = relatedLeft.ID;
    relatedRightId = relatedRight.ID;

    const relationTargetShared = await apiClient.createGroup({
      name: `Compare Relation Target Shared ${testRunId}`,
      categoryId,
    });
    const relationTargetLeft = await apiClient.createGroup({
      name: `Compare Relation Target Left ${testRunId}`,
      categoryId,
    });
    const relationTargetRight = await apiClient.createGroup({
      name: `Compare Relation Target Right ${testRunId}`,
      categoryId,
    });
    relationTargetSharedId = relationTargetShared.ID;
    relationTargetLeftId = relationTargetLeft.ID;
    relationTargetRightId = relationTargetRight.ID;

    const leftGroup = await apiClient.createGroup({
      name: leftGroupName,
      description: 'Left side description',
      categoryId,
      url: 'https://left.example.com/group-compare',
      meta: JSON.stringify({ side: 'left', shared: true }),
      groups: [relatedSharedId, relatedLeftId],
    });
    const rightGroup = await apiClient.createGroup({
      name: rightGroupName,
      description: 'Right side description',
      categoryId,
      url: 'https://right.example.com/group-compare',
      meta: JSON.stringify({ side: 'right', shared: true }),
      groups: [relatedSharedId, relatedRightId],
    });
    const thirdGroup = await apiClient.createGroup({
      name: thirdGroupName,
      description: 'Third group for toolbar picker updates',
      categoryId,
      meta: JSON.stringify({ side: 'third' }),
    });
    leftGroupId = leftGroup.ID;
    rightGroupId = rightGroup.ID;
    thirdGroupId = thirdGroup.ID;

    await apiClient.addTagsToGroups([leftGroupId], [tagLeftId, tagSharedId]);
    await apiClient.addTagsToGroups([rightGroupId], [tagSharedId, tagRightId]);

    const leftChild = await apiClient.createGroup({
      name: `Compare Left Child ${testRunId}`,
      categoryId,
      ownerId: leftGroupId,
    });
    const rightChild = await apiClient.createGroup({
      name: `Compare Right Child ${testRunId}`,
      categoryId,
      ownerId: rightGroupId,
    });
    leftChildId = leftChild.ID;
    rightChildId = rightChild.ID;

    const relationType = await apiClient.createRelationType({
      name: `Compare Relation ${testRunId}`,
      description: 'Relation type for group compare tests',
      fromCategoryId: categoryId,
      toCategoryId: categoryId,
    });
    relationTypeId = relationType.ID;

    await apiClient.createRelation({
      fromGroupId: leftGroupId,
      toGroupId: relationTargetSharedId,
      relationTypeId,
      name: `Shared compare relation ${testRunId}`,
    });
    await apiClient.createRelation({
      fromGroupId: rightGroupId,
      toGroupId: relationTargetSharedId,
      relationTypeId,
      name: `Shared compare relation ${testRunId}`,
    });
    await apiClient.createRelation({
      fromGroupId: leftGroupId,
      toGroupId: relationTargetLeftId,
      relationTypeId,
      name: `Left compare relation ${testRunId}`,
    });
    await apiClient.createRelation({
      fromGroupId: rightGroupId,
      toGroupId: relationTargetRightId,
      relationTypeId,
      name: `Right compare relation ${testRunId}`,
    });
  });

  test('shows compare bulk action for exactly 2 groups on the list view', async ({ page }) => {
    await page.goto(`/groups?Name=${encodeURIComponent(testRunId)}`);
    await page.waitForLoadState('load');

    await selectGroupByName(page, leftGroupName);
    await selectGroupByName(page, rightGroupName);
    await page.waitForTimeout(300);

    const compareLink = page.locator('.bulk-editors a:has-text("Compare")');
    await expect(compareLink).toBeVisible({ timeout: 5000 });

    const href = await compareLink.getAttribute('href');
    expect(href).toContain('/group/compare');
    expect(href).toContain(`g1=${leftGroupId}`);
    expect(href).toContain(`g2=${rightGroupId}`);
  });

  test('shows compare bulk action on the text view too', async ({ page }) => {
    await page.goto(`/groups/text?Name=${encodeURIComponent(testRunId)}`);
    await page.waitForLoadState('load');

    await selectGroupByName(page, leftGroupName);
    await selectGroupByName(page, rightGroupName);
    await page.waitForTimeout(300);

    await expect(page.locator('.bulk-editors a:has-text("Compare")')).toBeVisible({ timeout: 5000 });
  });

  test('renders the compare page with metadata and diff sections', async ({ page }) => {
    await page.goto(`/group/compare?g1=${leftGroupId}&g2=${rightGroupId}`);
    await page.waitForLoadState('load');

    await expect(page.locator('summary:has-text("Metadata")')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('.compare-meta-card-label:has-text("Name")').first()).toBeVisible();
    await expect(page.locator('.compare-meta-card-label:has-text("Category")').first()).toBeVisible();
    await expect(page.locator('.compare-meta-card-label:has-text("Owner")').first()).toBeVisible();
    await expect(page.locator('.compare-meta-card-label:has-text("URL")').first()).toBeVisible();
    await expect(page.locator('.compare-meta-card-label:has-text("Created")').first()).toBeVisible();
    await expect(page.locator('.compare-meta-card-label:has-text("Updated")').first()).toBeVisible();

    await expect(page.locator('summary:has-text("Tags")')).toBeVisible();
    await expect(page.locator('summary:has-text("Own Entities")')).toBeVisible();
    await expect(page.locator('summary:has-text("Related Entities")')).toBeVisible();
    await expect(page.locator('summary:has-text("Relations")')).toBeVisible();
    await expect(page.locator('summary:has-text("Description")')).toBeVisible();
    await expect(page.locator('summary:has-text("Meta JSON")')).toBeVisible();
    await expect(page.locator('.compare-summary')).toContainText('Groups differ');
  });

  test('swap button updates the compare URL', async ({ page }) => {
    await page.goto(`/group/compare?g1=${leftGroupId}&g2=${rightGroupId}`);
    await page.waitForLoadState('load');

    await Promise.all([
      page.waitForURL(new RegExp(`/group/compare\\?g1=${rightGroupId}.*g2=${leftGroupId}`)),
      page.locator('.compare-swap-btn').click(),
    ]);
  });

  test('toolbar pickers update the URL for left and right selections', async ({ page }) => {
    await page.goto(`/group/compare?g1=${leftGroupId}&g2=${rightGroupId}`);
    await page.waitForLoadState('load');

    const leftInput = page.getByLabel('Search left group');
    await leftInput.fill(thirdGroupName);
    const leftOption = page.locator(`div[role="option"]:visible:has-text("${thirdGroupName}")`).first();
    await expect(leftOption).toBeVisible({ timeout: 10000 });
    await Promise.all([
      page.waitForURL(new RegExp(`/group/compare\\?g1=${thirdGroupId}.*g2=${rightGroupId}`)),
      leftOption.click(),
    ]);

    const rightInput = page.getByLabel('Search right group');
    await rightInput.fill(leftGroupName);
    const rightOption = page.locator(`div[role="option"]:visible:has-text("${leftGroupName}")`).first();
    await expect(rightOption).toBeVisible({ timeout: 10000 });
    await Promise.all([
      page.waitForURL(new RegExp(`/group/compare\\?g1=${thirdGroupId}.*g2=${leftGroupId}`)),
      rightOption.click(),
    ]);
  });

  test('no JS errors on /group/compare with no params', async ({ page }) => {
    const jsErrors: string[] = [];
    page.on('pageerror', error => jsErrors.push(error.message));

    await page.goto('/group/compare');
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(1000);

    expect(jsErrors).toEqual([]);
    await expect(page.getByText(/Group 1 ID \(g1\) is required/i).first()).toBeVisible();
  });

  test.afterAll(async ({ apiClient }) => {
    const groupIds = [
      leftChildId,
      rightChildId,
      leftGroupId,
      rightGroupId,
      thirdGroupId,
      relatedSharedId,
      relatedLeftId,
      relatedRightId,
      relationTargetSharedId,
      relationTargetLeftId,
      relationTargetRightId,
    ];

    for (const id of groupIds) {
      if (!id) continue;
      try {
        await apiClient.deleteGroup(id);
      } catch (error) {
        console.warn(`Cleanup: failed to delete group ${id}`, error);
      }
    }

    if (relationTypeId) {
      try {
        await apiClient.deleteRelationType(relationTypeId);
      } catch (error) {
        console.warn(`Cleanup: failed to delete relation type ${relationTypeId}`, error);
      }
    }

    for (const tagId of [tagLeftId, tagSharedId, tagRightId]) {
      if (!tagId) continue;
      try {
        await apiClient.deleteTag(tagId);
      } catch (error) {
        console.warn(`Cleanup: failed to delete tag ${tagId}`, error);
      }
    }

    if (categoryId) {
      try {
        await apiClient.deleteCategory(categoryId);
      } catch (error) {
        console.warn(`Cleanup: failed to delete category ${categoryId}`, error);
      }
    }
  });
});
