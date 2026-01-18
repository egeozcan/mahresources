import { test as base, request as playwrightRequest } from '@playwright/test';
import { ApiClient } from '../helpers/api-client';
import { expectNoViolations, expectComponentNoViolations, A11yCheckOptions } from '../helpers/accessibility/axe-helper';
import { A11yTestData } from '../helpers/accessibility/a11y-config';

/**
 * Cache for test data to avoid recreating for each test
 * This is shared across tests in the same worker
 */
let cachedTestData: (A11yTestData & { cleanup: () => Promise<void> }) | null = null;
let setupPromise: Promise<A11yTestData & { cleanup: () => Promise<void> }> | null = null;

/**
 * Extended fixtures for accessibility testing
 */
type A11yFixtures = {
  apiClient: ApiClient;
  a11yTestData: A11yTestData;
  checkA11y: (options?: A11yCheckOptions) => Promise<void>;
  checkComponentA11y: (selector: string, options?: Omit<A11yCheckOptions, 'include'>) => Promise<void>;
};

/**
 * Create test data using the Playwright request context
 */
async function createTestData(baseURL: string): Promise<A11yTestData & { cleanup: () => Promise<void> }> {
  // Create a fresh request context for setup
  const requestContext = await playwrightRequest.newContext({
    baseURL,
  });

  const client = new ApiClient(requestContext, baseURL);
  const createdIds: Partial<Record<string, number>> = {};

  // Use a unique suffix to avoid conflicts when running in parallel workers
  const uniqueSuffix = `${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;

  // Create test entities in dependency order
  // 1. Category (needed for groups)
  const category = await client.createCategory(
    `A11y Test Category ${uniqueSuffix}`,
    'Category for accessibility tests'
  );
  createdIds.categoryId = category.ID;

  // 2. Tag
  const tag = await client.createTag(
    `a11y-test-tag-${uniqueSuffix}`,
    'Tag for accessibility tests'
  );
  createdIds.tagId = tag.ID;

  // 3. Note Type
  const noteType = await client.createNoteType(
    `A11y Test Note Type ${uniqueSuffix}`,
    'Note type for accessibility tests'
  );
  createdIds.noteTypeId = noteType.ID;

  // 4. Relation Type
  const relationType = await client.createRelationType({
    name: `A11y Test Relation ${uniqueSuffix}`,
    description: 'Relation type for accessibility tests',
  });
  createdIds.relationTypeId = relationType.ID;

  // 5. Groups (need 2 for relation testing, without tags to avoid GORM association issues)
  const group1 = await client.createGroup({
    name: `A11y Test Group 1 ${uniqueSuffix}`,
    description: 'First group for accessibility tests',
    categoryId: category.ID,
  });
  createdIds.groupId = group1.ID;

  const group2 = await client.createGroup({
    name: `A11y Test Group 2 ${uniqueSuffix}`,
    description: 'Second group for accessibility tests',
    categoryId: category.ID,
  });
  createdIds.group2Id = group2.ID;

  // 6. Note (without tags/groups to avoid GORM association issues)
  const note = await client.createNote({
    name: `A11y Test Note ${uniqueSuffix}`,
    description: 'This is a note created for accessibility testing. It contains enough text to test expandable text components and other UI elements.',
    noteTypeId: noteType.ID,
  });
  createdIds.noteId = note.ID;

  // 7. Query
  const query = await client.createQuery({
    name: `A11y Test Query ${uniqueSuffix}`,
    text: 'SELECT * FROM notes LIMIT 10',
    description: 'Query for accessibility tests',
  });
  createdIds.queryId = query.ID;

  // 8. Relation
  const relation = await client.createRelation({
    name: `A11y Test Relation Instance ${uniqueSuffix}`,
    description: 'Relation instance for accessibility tests',
    fromGroupId: group1.ID,
    toGroupId: group2.ID,
    relationTypeId: relationType.ID,
  });
  createdIds.relationId = relation.ID;

  return {
    categoryId: category.ID,
    tagId: tag.ID,
    noteTypeId: noteType.ID,
    relationTypeId: relationType.ID,
    groupId: group1.ID,
    group2Id: group2.ID,
    noteId: note.ID,
    queryId: query.ID,
    relationId: relation.ID,
    cleanup: async () => {
      // Delete in reverse dependency order
      try {
        if (createdIds.relationId) await client.deleteRelation(createdIds.relationId);
      } catch { /* ignore cleanup errors */ }
      try {
        if (createdIds.queryId) await client.deleteQuery(createdIds.queryId);
      } catch { /* ignore cleanup errors */ }
      try {
        if (createdIds.noteId) await client.deleteNote(createdIds.noteId);
      } catch { /* ignore cleanup errors */ }
      try {
        if (createdIds.group2Id) await client.deleteGroup(createdIds.group2Id);
      } catch { /* ignore cleanup errors */ }
      try {
        if (createdIds.groupId) await client.deleteGroup(createdIds.groupId);
      } catch { /* ignore cleanup errors */ }
      try {
        if (createdIds.relationTypeId) await client.deleteRelationType(createdIds.relationTypeId);
      } catch { /* ignore cleanup errors */ }
      try {
        if (createdIds.noteTypeId) await client.deleteNoteType(createdIds.noteTypeId);
      } catch { /* ignore cleanup errors */ }
      try {
        if (createdIds.tagId) await client.deleteTag(createdIds.tagId);
      } catch { /* ignore cleanup errors */ }
      try {
        if (createdIds.categoryId) await client.deleteCategory(createdIds.categoryId);
      } catch { /* ignore cleanup errors */ }
      await requestContext.dispose();
    },
  };
}

/**
 * Perform cleanup of cached test data
 */
async function performCleanup(): Promise<void> {
  if (cachedTestData) {
    await cachedTestData.cleanup();
    cachedTestData = null;
    setupPromise = null;
  }
}

export const test = base.extend<A11yFixtures>({
  /**
   * API client for creating test data
   */
  apiClient: async ({ request, baseURL }, use) => {
    if (!baseURL) {
      throw new Error('baseURL must be configured in playwright.config.ts');
    }
    const client = new ApiClient(request, baseURL);
    await use(client);
  },

  /**
   * Test data that is created once per worker and reused.
   * Cleanup happens via test.afterAll() after all tests in each file complete.
   */
  a11yTestData: async ({ baseURL }, use) => {
    if (!baseURL) {
      throw new Error('baseURL must be configured in playwright.config.ts');
    }

    // Use cached data if available, otherwise create new
    if (!cachedTestData) {
      // Use a promise to prevent race conditions when tests run in parallel within same worker
      if (!setupPromise) {
        setupPromise = createTestData(baseURL);
      }
      cachedTestData = await setupPromise;
    }

    // Provide data without cleanup function to tests
    const { cleanup, ...data } = cachedTestData;
    await use(data);
  },

  /**
   * Convenience method for full-page accessibility checks
   */
  checkA11y: async ({ page }, use) => {
    const checkFn = async (options?: A11yCheckOptions) => {
      await expectNoViolations(page, options);
    };
    await use(checkFn);
  },

  /**
   * Convenience method for component-specific accessibility checks
   */
  checkComponentA11y: async ({ page }, use) => {
    const checkFn = async (selector: string, options?: Omit<A11yCheckOptions, 'include'>) => {
      await expectComponentNoViolations(page, selector, options);
    };
    await use(checkFn);
  },
});

export { expect } from '@playwright/test';

// Use test.afterAll for cleanup - this runs after all tests in each file
test.afterAll(async () => {
  await performCleanup();
});
