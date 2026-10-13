import { test, expect } from '../../fixtures/base.fixture';
import type { Locator, Page } from '@playwright/test';
import * as path from 'path';

type CardEntity = { ID: number; Name: string; Description: string };
type Variant = 'hidden' | 'visible' | 'uncategorized';
type Siblings = Record<Variant, CardEntity>;

function card(page: Page, entityType: 'group' | 'resource' | 'note', id: number): Locator {
  return page.locator(`article.${entityType}-card`).filter({
    has: page.locator(`.card-title a[href="/${entityType}?id=${id}"]`),
  });
}

test.describe('Section Visibility on native cards', () => {
  let parentId: number;
  let tagId: number;
  let tagName: string;
  let hiddenCategoryId: number;
  let visibleCategoryId: number;
  let parentName: string;
  let groups: Siblings;
  let resources: Siblings;
  let notes: Siblings;
  let categoryIds: number[];
  let resourceCategoryIds: number[];
  let noteTypeIds: number[];
  let groupIds: number[];
  let resourceIds: number[];
  let noteIds: number[];

  test.beforeAll(async ({ apiClient }) => {
    categoryIds = [];
    resourceCategoryIds = [];
    noteTypeIds = [];
    groupIds = [];
    resourceIds = [];
    noteIds = [];
    groups = {} as Siblings;
    resources = {} as Siblings;
    notes = {} as Siblings;
    const stamp = Date.now();
    const customSlots = {
      CustomSummary: '<p data-testid="card-custom-summary">[property path="Description"]</p>',
      CustomAvatar: '<span data-testid="card-custom-avatar">Custom avatar</span>',
    };

    await apiClient.enablePlugin('test-actions');
    const tag = await apiClient.createTag(`Card visibility tag ${stamp}`);
    tagId = tag.ID;
    tagName = tag.Name;
    const hiddenCategory = await apiClient.createCategory(`Hidden cards ${stamp}`, '', {
      ...customSlots,
      SectionConfig: JSON.stringify({ description: false, tags: false, timestamps: false }),
    });
    hiddenCategoryId = hiddenCategory.ID;
    categoryIds.push(hiddenCategory.ID);
    const visibleCategory = await apiClient.createCategory(`Default cards ${stamp}`, '', {
      SectionConfig: JSON.stringify({ ownEntities: { state: 'collapsed' } }),
    });
    visibleCategoryId = visibleCategory.ID;
    categoryIds.push(visibleCategory.ID);
    const hiddenResourceCategory = await apiClient.createResourceCategory(`Hidden resource cards ${stamp}`, '', {
      ...customSlots,
      SectionConfig: JSON.stringify({
        description: false, tags: false, owner: false, fileSize: false,
        categoryLink: false, previewImage: false,
      }),
    });
    resourceCategoryIds.push(hiddenResourceCategory.ID);
    const visibleResourceCategory = await apiClient.createResourceCategory(`Default resource cards ${stamp}`, '', {
      SectionConfig: JSON.stringify({ technicalDetails: { state: 'collapsed' } }),
    });
    resourceCategoryIds.push(visibleResourceCategory.ID);
    const hiddenNoteType = await apiClient.createNoteType(`Hidden note cards ${stamp}`, '', {
      ...customSlots,
      SectionConfig: JSON.stringify({ content: false, tags: false }),
    });
    noteTypeIds.push(hiddenNoteType.ID);
    const visibleNoteType = await apiClient.createNoteType(`Default note cards ${stamp}`, '', {
      SectionConfig: JSON.stringify({ groups: false }),
    });
    noteTypeIds.push(visibleNoteType.ID);

    parentName = `Card visibility parent ${stamp}`;
    const parent = await apiClient.createGroup({ name: parentName, categoryId: hiddenCategoryId });
    parentId = parent.ID;
    groupIds.push(parent.ID);

    for (const variant of ['hidden', 'visible', 'uncategorized'] as const) {
      const groupDescription = `Group ${variant} description ${stamp}`;
      const group = await apiClient.createGroup({
        name: `Group ${variant} card ${stamp}`,
        description: groupDescription,
        categoryId: variant === 'hidden' ? hiddenCategoryId : variant === 'visible' ? visibleCategoryId : 0,
        ownerId: parentId,
        tags: [tagId],
      });
      groups[variant] = { ...group, Description: groupDescription };
      groupIds.push(group.ID);

      const resourceDescription = `Resource ${variant} description ${stamp}`;
      const resource = await apiClient.createResource({
        filePath: path.resolve(__dirname, '../../test-assets/sample-image-21.png'),
        name: `Resource ${variant} card ${stamp}`,
        description: resourceDescription,
        resourceCategoryId: variant === 'hidden' ? hiddenResourceCategory.ID : variant === 'visible' ? visibleResourceCategory.ID : undefined,
        ownerId: parentId,
      });
      resources[variant] = { ...resource, Description: resourceDescription };
      resourceIds.push(resource.ID);
      await apiClient.addTagsToResources([resource.ID], [tagId]);

      const noteDescription = `Note ${variant} description ${stamp}`;
      const note = await apiClient.createNote({
        name: `Note ${variant} card ${stamp}`,
        description: noteDescription,
        noteTypeId: variant === 'hidden' ? hiddenNoteType.ID : variant === 'visible' ? visibleNoteType.ID : undefined,
        ownerId: parentId,
        tags: [tagId],
      });
      notes[variant] = { ...note, Description: noteDescription };
      noteIds.push(note.ID);
    }
  });

  test.afterAll(async ({ apiClient }) => {
    for (const id of resourceIds) await apiClient.deleteResource(id).catch(() => {});
    for (const id of noteIds) await apiClient.deleteNote(id).catch(() => {});
    for (const id of [...groupIds].reverse()) await apiClient.deleteGroup(id).catch(() => {});
    for (const id of resourceCategoryIds) await apiClient.deleteResourceCategory(id).catch(() => {});
    for (const id of noteTypeIds) await apiClient.deleteNoteType(id).catch(() => {});
    for (const id of categoryIds) await apiClient.deleteCategory(id).catch(() => {});
    if (tagId) await apiClient.deleteTag(tagId).catch(() => {});
    await apiClient.disablePlugin('test-actions');
  });

  for (const route of ['/groups', '/groups/text']) {
    test(`group cards respect description, tags and timestamps on ${route}`, async ({ page }) => {
      await page.goto(`${route}?ownerId=${parentId}`);
      const hidden = card(page, 'group', groups.hidden.ID);
      await expect(hidden).toHaveCount(1);
      await expect(hidden.getByRole('link', { name: groups.hidden.Name, exact: true })).toBeVisible();
      await expect(hidden.getByRole('checkbox', { name: `Select ${groups.hidden.Name}`, exact: true })).toBeVisible();
      await expect(hidden.getByTestId('card-custom-summary')).toHaveText(groups.hidden.Description);
      if (route === '/groups') await expect(hidden.getByTestId('card-custom-avatar')).toBeVisible();

      for (const variant of ['visible', 'uncategorized'] as const) {
        const sibling = card(page, 'group', groups[variant].ID);
        await expect(sibling).toHaveCount(1);
        await expect(sibling.locator('.card-description')).toContainText(groups[variant].Description);
        await expect(sibling.getByRole('link', { name: tagName, exact: true })).toBeVisible();
        if (route === '/groups') await expect(sibling.locator('.card-meta-label')).toHaveCount(2);
      }

      await expect(hidden.locator('.card-description')).toHaveCount(0);
      await expect(hidden.getByText(groups.hidden.Description, { exact: true })).toHaveCount(1);
      await expect(hidden.getByRole('link', { name: tagName, exact: true })).toHaveCount(0);
      await expect(hidden.locator('.card-meta-label')).toHaveCount(0);
    });
  }

  for (const route of ['/resources', '/resources/simple']) {
    test(`resource cards hide native sections but preserve custom slots and plugin actions on ${route}`, async ({ page }) => {
      await page.goto(`${route}?ownerId=${parentId}`);
      const hidden = card(page, 'resource', resources.hidden.ID);
      await expect(hidden).toHaveCount(1);
      await expect(hidden.locator(`.card-title a[href="/resource?id=${resources.hidden.ID}"]`)).toHaveCount(1);
      await expect(hidden.getByTestId('card-custom-summary')).toHaveText(resources.hidden.Description);
      await expect(hidden.getByTestId('card-custom-avatar')).toHaveCount(1);
      const pluginActions = hidden.locator('button[aria-haspopup="menu"]');
      await expect(pluginActions).toHaveCount(1);
      if (route === '/resources') {
        await expect(hidden.getByTestId('card-custom-avatar')).toBeVisible();
        const selection = hidden.getByRole('checkbox', { name: `Select ${resources.hidden.Name}`, exact: true });
        await selection.check();
        await expect(selection).toBeChecked();
        await pluginActions.click();
        await expect(hidden.getByRole('menuitem', { name: 'Greet Resource', exact: true })).toBeVisible();
        await page.keyboard.press('Escape');
      }

      for (const variant of ['visible', 'uncategorized'] as const) {
        const sibling = card(page, 'resource', resources[variant].ID);
        await expect(sibling).toHaveCount(1);
        await expect(sibling.locator('.card-description')).toContainText(resources[variant].Description);
        await expect(sibling.locator('.card-tags a').filter({ hasText: tagName })).toHaveCount(1);
        await expect(sibling.locator('button.edit-in-list')).toHaveCount(1);
        await expect(sibling.locator('.card-image img')).toHaveCount(1);
        await expect(sibling.locator(`.card-meta a[href="/group?id=${parentId}"]`)).toHaveCount(1);
        await expect(sibling.locator('.card-meta-item').filter({ hasText: /^\s*\d/ })).toHaveCount(1);
      }
      await expect(card(page, 'resource', resources.visible.ID).locator('a[href^="/resourceCategory?id="]')).toHaveCount(1);

      await expect(hidden.locator('.card-description')).toHaveCount(0);
      await expect(hidden.getByText(resources.hidden.Description, { exact: true })).toHaveCount(1);
      await expect(hidden.locator('.card-tags a').filter({ hasText: tagName })).toHaveCount(0);
      await expect(hidden.locator('button.edit-in-list')).toHaveCount(0);
      await expect(hidden.locator('.card-image')).toHaveCount(0);
      await expect(hidden.locator(`.card-meta a[href="/group?id=${parentId}"]`)).toHaveCount(0);
      await expect(hidden.locator('.card-meta-item').filter({ hasText: /^\s*\d/ })).toHaveCount(0);
      await expect(hidden.locator('a[href^="/resourceCategory?id="]')).toHaveCount(0);
    });
  }

  test('note cards hide native content and tags while keeping custom summary, avatar and selection', async ({ page }) => {
    await page.goto(`/notes?ownerId=${parentId}`);
    const hidden = card(page, 'note', notes.hidden.ID);
    await expect(hidden).toHaveCount(1);
    await expect(hidden.getByRole('link', { name: notes.hidden.Name, exact: true })).toBeVisible();
    await expect(hidden.getByTestId('card-custom-summary')).toHaveText(notes.hidden.Description);
    await expect(hidden.getByTestId('card-custom-avatar')).toBeVisible();
    const selection = hidden.getByRole('checkbox', { name: `Select ${notes.hidden.Name}`, exact: true });
    await selection.check();
    await expect(selection).toBeChecked();

    for (const variant of ['visible', 'uncategorized'] as const) {
      const sibling = card(page, 'note', notes[variant].ID);
      await expect(sibling).toHaveCount(1);
      await expect(sibling.locator('.card-description')).toContainText(notes[variant].Description);
      await expect(sibling.locator('[data-description-display]')).toHaveAttribute('title', 'Double-click to edit');
      await expect(sibling.getByRole('link', { name: tagName, exact: true })).toBeVisible();
    }

    await expect(hidden.locator('.card-description')).toHaveCount(0);
    await expect(hidden.locator('[data-description-display]')).toHaveCount(0);
    await expect(hidden.getByText(notes.hidden.Description, { exact: true })).toHaveCount(1);
    await expect(hidden.getByRole('link', { name: tagName, exact: true })).toHaveCount(0);
  });

  test('nested mixed-entity cards use their own configuration instead of the parent detail configuration', async ({ page }) => {
    await page.goto(`/group?id=${parentId}`);
    const ownEntities = page.locator('details').filter({ has: page.locator('summary:text-is("Own Entities")') });
    await expect(ownEntities).toHaveAttribute('open', '');

    for (const [entityType, siblings] of [['group', groups], ['resource', resources], ['note', notes]] as const) {
      const hidden = card(page, entityType, siblings.hidden.ID);
      await expect(hidden).toHaveCount(1);
      await expect(hidden.getByTestId('card-custom-summary')).toHaveText(siblings.hidden.Description);
      await expect(hidden.getByTestId('card-custom-avatar')).toBeVisible();
      for (const variant of ['visible', 'uncategorized'] as const) {
        const sibling = card(page, entityType, siblings[variant].ID);
        await expect(sibling).toHaveCount(1);
        await expect(sibling.locator('.card-description')).toContainText(siblings[variant].Description);
      }
    }
    await expect(card(page, 'group', groups.visible.ID).locator('.card-meta-label')).toHaveCount(2);
    await expect(card(page, 'resource', resources.visible.ID).locator('.card-image img')).toHaveCount(1);

    for (const [entityType, siblings] of [['group', groups], ['resource', resources], ['note', notes]] as const) {
      const hidden = card(page, entityType, siblings.hidden.ID);
      await expect(hidden.locator('.card-description')).toHaveCount(0);
      await expect(hidden.getByText(siblings.hidden.Description, { exact: true })).toHaveCount(1);
    }
  });

  test('category and tag detail cards retain their carrier visibility', async ({ page }) => {
    for (const route of [`/category?id=${hiddenCategoryId}`, `/tag?id=${tagId}`]) {
      await page.goto(route);
      const carriers = route.startsWith('/category')
        ? [['group', groups]] as const
        : [['group', groups], ['resource', resources], ['note', notes]] as const;
      for (const [entityType, siblings] of carriers) {
        const hidden = card(page, entityType, siblings.hidden.ID);
        await expect(hidden).toHaveCount(1);
        await expect.soft(hidden.getByTestId('card-custom-summary')).toHaveText(siblings.hidden.Description, { timeout: 1000 });
        await expect.soft(hidden.locator('.card-description')).toHaveCount(0, { timeout: 1000 });
        if (route.startsWith('/tag')) {
          for (const variant of ['visible', 'uncategorized'] as const) {
            const sibling = card(page, entityType, siblings[variant].ID);
            await expect(sibling).toHaveCount(1);
            await expect(sibling.locator('.card-description')).toContainText(siblings[variant].Description);
          }
        }
      }
    }
  });

  test('saving Section Visibility in the category form persists and changes that category\'s cards', async ({ page, categoryPage }) => {
    await categoryPage.gotoEdit(visibleCategoryId);
    await categoryPage.fillCodeEditor('CustomSummary', '<p data-testid="card-custom-summary">[property path="Description"]</p>');
    const visibility = page.locator('fieldset').filter({ has: page.locator('legend:text-is("Section Visibility")') });
    const description = visibility.getByLabel('Description', { exact: true });
    await expect(description).toBeChecked();
    await description.uncheck();
    await visibility.getByLabel('Tags', { exact: true }).uncheck();
    await visibility.getByLabel('Timestamps', { exact: true }).uncheck();
    await categoryPage.save();
    await expect(page).toHaveURL(new RegExp(`/category\\?id=${visibleCategoryId}$`));

    await categoryPage.gotoEdit(visibleCategoryId);
    await expect(visibility.getByLabel('Description', { exact: true })).not.toBeChecked();
    await expect(visibility.getByLabel('Tags', { exact: true })).not.toBeChecked();
    await expect(visibility.getByLabel('Timestamps', { exact: true })).not.toBeChecked();

    await page.goto(`/groups?ownerId=${parentId}`);
    const changed = card(page, 'group', groups.visible.ID);
    await expect(changed).toHaveCount(1);
    await expect(changed.getByTestId('card-custom-summary')).toHaveText(groups.visible.Description);
    await expect(card(page, 'group', groups.uncategorized.ID).locator('.card-description')).toContainText(groups.uncategorized.Description);
    await expect(changed.locator('.card-description')).toHaveCount(0);
    await expect(changed.getByText(groups.visible.Description, { exact: true })).toHaveCount(1);
    await expect(changed.getByRole('link', { name: tagName, exact: true })).toHaveCount(0);
    await expect(changed.locator('.card-meta-label')).toHaveCount(0);
  });
});
