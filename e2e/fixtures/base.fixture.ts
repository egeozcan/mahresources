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
import { ServerInfo, startServer, stopServer } from './server-manager';

// Module-level cache — safe because each Playwright worker is a separate process.
// Set by the auto workerServer fixture so getWorkerBaseUrl() works in beforeAll hooks.
let _workerBaseUrl: string | null = null;

/**
 * Returns the base URL of the current worker's ephemeral server.
 * Works in test.beforeAll hooks and standalone helpers.
 */
export function getWorkerBaseUrl(): string {
  return _workerBaseUrl || process.env.BASE_URL || 'http://localhost:8181';
}

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

type WorkerFixtures = {
  workerServer: ServerInfo;
};

export const test = base.extend<TestFixtures, WorkerFixtures>({
  // ---- Worker-scoped: one ephemeral server per Playwright worker ----
  // auto:true ensures it runs before any beforeAll hooks.
  workerServer: [async ({}, use) => {
    const externalUrl = process.env.BASE_URL;
    if (externalUrl) {
      // External server mode (manual testing) — use provided URL
      _workerBaseUrl = externalUrl;
      const url = new URL(externalUrl);
      const shareUrl = process.env.SHARE_BASE_URL;
      await use({
        port: parseInt(url.port) || 8181,
        sharePort: shareUrl ? parseInt(new URL(shareUrl).port) || 8183 : 8183,
        proc: null,
      });
      _workerBaseUrl = null;
      return;
    }

    // Auto mode — start a dedicated ephemeral server for this worker
    const server = await startServer();
    _workerBaseUrl = `http://127.0.0.1:${server.port}`;
    await use(server);
    _workerBaseUrl = null;
    await stopServer(server.proc);
  }, { scope: 'worker', auto: true }],

  // ---- Override baseURL so page.goto() and request use this worker's server ----
  baseURL: async ({ workerServer }, use) => {
    await use(`http://127.0.0.1:${workerServer.port}`);
  },

  // ---- Test-scoped fixtures (unchanged) ----
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

  shareBaseUrl: async ({ workerServer }, use) => {
    await use(`http://127.0.0.1:${workerServer.sharePort}`);
  },
});

export { expect };
