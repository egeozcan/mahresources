import { test as base, expect } from '@playwright/test';
import { ApiClient } from '../helpers/api-client';
import { TagPage } from '../pages/TagPage';
import { CategoryPage } from '../pages/CategoryPage';
import { NoteTypePage } from '../pages/NoteTypePage';
import { GroupPage } from '../pages/GroupPage';
import { NotePage } from '../pages/NotePage';
import { QueryPage } from '../pages/QueryPage';
import { ResourcePage } from '../pages/ResourcePage';
import { RelationTypePage } from '../pages/RelationTypePage';
import { RelationPage } from '../pages/RelationPage';
import { ResourceCategoryPage } from '../pages/ResourceCategoryPage';

type TestFixtures = {
  apiClient: ApiClient;
  tagPage: TagPage;
  categoryPage: CategoryPage;
  noteTypePage: NoteTypePage;
  groupPage: GroupPage;
  notePage: NotePage;
  queryPage: QueryPage;
  resourcePage: ResourcePage;
  relationTypePage: RelationTypePage;
  relationPage: RelationPage;
  resourceCategoryPage: ResourceCategoryPage;
  shareBaseUrl: string;
};

export const test = base.extend<TestFixtures>({
  apiClient: async ({ request, baseURL }, use) => {
    if (!baseURL) {
      throw new Error('baseURL must be configured in playwright.config.ts');
    }
    const client = new ApiClient(request, baseURL);
    await use(client);
  },

  tagPage: async ({ page }, use) => {
    await use(new TagPage(page));
  },

  categoryPage: async ({ page }, use) => {
    await use(new CategoryPage(page));
  },

  noteTypePage: async ({ page }, use) => {
    await use(new NoteTypePage(page));
  },

  groupPage: async ({ page }, use) => {
    await use(new GroupPage(page));
  },

  notePage: async ({ page }, use) => {
    await use(new NotePage(page));
  },

  queryPage: async ({ page }, use) => {
    await use(new QueryPage(page));
  },

  resourcePage: async ({ page }, use) => {
    await use(new ResourcePage(page));
  },

  relationTypePage: async ({ page }, use) => {
    await use(new RelationTypePage(page));
  },

  relationPage: async ({ page }, use) => {
    await use(new RelationPage(page));
  },

  resourceCategoryPage: async ({ page }, use) => {
    await use(new ResourceCategoryPage(page));
  },

  shareBaseUrl: async ({}, use) => {
    const shareBaseUrl = process.env.SHARE_BASE_URL || 'http://127.0.0.1:8183';
    await use(shareBaseUrl);
  },
});

export { expect };
