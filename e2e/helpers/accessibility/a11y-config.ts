/**
 * Configuration for accessibility tests
 */

/**
 * Static pages that can be tested without any pre-existing data
 * These pages either show empty states or are forms
 */
export const STATIC_PAGES = [
  // Dashboard
  { path: '/dashboard', name: 'Dashboard' },
  { path: '/admin/overview', name: 'Admin overview' },
  { path: '/admin/shares', name: 'Admin shares dashboard' }, // BH-035
  // List pages (show empty state if no data)
  { path: '/notes', name: 'Notes list' },
  { path: '/groups', name: 'Groups list' },
  { path: '/resources', name: 'Resources list' },
  { path: '/tags', name: 'Tags list' },
  { path: '/categories', name: 'Categories list' },
  { path: '/queries', name: 'Queries list' },
  { path: '/noteTypes', name: 'Note types list' },
  { path: '/relationTypes', name: 'Relation types list' },
  { path: '/relations', name: 'Relations list' },
  { path: '/resourceCategories', name: 'Resource categories list' },
  { path: '/templatePartials', name: 'Template partials list' },
  { path: '/logs', name: 'Logs list' },
  { path: '/downloads', name: 'Downloads list' },

  // Pages the 2026-07-29 UI bug hunt touched that this sweep did not cover.
  // /search was added by Batch 6 (finding 32) and had never been audited at all;
  // /group/tree is finding 8's subject; /plugins/manage is finding 136's;
  // /admin/export and /admin/import carry the comboboxes finding 36/105 rebuilt;
  // /admin/users is finding 34/107/109's; /account and /mrql are the two
  // remaining top-level pages with no entry here.
  { path: '/search?q=a', name: 'Search results' },
  { path: '/group/tree', name: 'Group tree' },
  { path: '/plugins/manage', name: 'Plugin management' },
  { path: '/admin/export', name: 'Admin export' },
  { path: '/admin/import', name: 'Admin import' },
  { path: '/admin/users', name: 'Admin users' },
  { path: '/account', name: 'Account' },
  { path: '/mrql', name: 'MRQL console' },

  // Create/new forms
  { path: '/note/new', name: 'Create note form' },
  { path: '/group/new', name: 'Create group form' },
  { path: '/resource/new', name: 'Create resource form' },
  { path: '/tag/new', name: 'Create tag form' },
  { path: '/category/new', name: 'Create category form' },
  { path: '/query/new', name: 'Create query form' },
  { path: '/noteType/new', name: 'Create note type form' },
  { path: '/templatePartial/new', name: 'Create template partial form' },
  { path: '/relationType/new', name: 'Create relation type form' },
  { path: '/relation/new', name: 'Create relation form' },
  { path: '/resourceCategory/new', name: 'Create resource category form' },

  // Alternative views
  { path: '/resources/details', name: 'Resources details view' },
  { path: '/resources/simple', name: 'Resources simple view' },
  { path: '/groups/text', name: 'Groups text view' },
] as const;

/**
 * Dynamic pages that require entity IDs to display meaningful content
 * These use placeholders like {noteId} that need to be replaced with actual IDs
 */
export const DYNAMIC_PAGES = [
  // Display pages
  { path: '/note?id={noteId}', name: 'Note detail', requiredData: ['noteId'] },
  { path: '/group?id={groupId}', name: 'Group detail', requiredData: ['groupId'] },
  { path: '/tag?id={tagId}', name: 'Tag detail', requiredData: ['tagId'] },
  { path: '/category?id={categoryId}', name: 'Category detail', requiredData: ['categoryId'] },
  { path: '/query?id={queryId}', name: 'Query detail', requiredData: ['queryId'] },
  { path: '/noteType?id={noteTypeId}', name: 'Note type detail', requiredData: ['noteTypeId'] },
  { path: '/relationType?id={relationTypeId}', name: 'Relation type detail', requiredData: ['relationTypeId'] },
  { path: '/relation?id={relationId}', name: 'Relation detail', requiredData: ['relationId'] },
  { path: '/resourceCategory?id={resourceCategoryId}', name: 'Resource category detail', requiredData: ['resourceCategoryId'] },

  // Edit forms
  { path: '/note/edit?id={noteId}', name: 'Edit note form', requiredData: ['noteId'] },
  { path: '/group/edit?id={groupId}', name: 'Edit group form', requiredData: ['groupId'] },
  { path: '/tag/edit?id={tagId}', name: 'Edit tag form', requiredData: ['tagId'] },
  { path: '/category/edit?id={categoryId}', name: 'Edit category form', requiredData: ['categoryId'] },
  { path: '/query/edit?id={queryId}', name: 'Edit query form', requiredData: ['queryId'] },
  { path: '/noteType/edit?id={noteTypeId}', name: 'Edit note type form', requiredData: ['noteTypeId'] },
  { path: '/relationType/edit?id={relationTypeId}', name: 'Edit relation type form', requiredData: ['relationTypeId'] },
  { path: '/relation/edit?id={relationId}', name: 'Edit relation form', requiredData: ['relationId'] },
  { path: '/resourceCategory/edit?id={resourceCategoryId}', name: 'Edit resource category form', requiredData: ['resourceCategoryId'] },

  // Product decision 107. In DYNAMIC_PAGES and not STATIC_PAGES because it needs a
  // user id — STATIC_PAGES is explicitly the set that needs no pre-existing data.
  // The id is read rather than created: the root-admin invariant guarantees at least
  // one account exists, so the sweep does not have to mint one and clean it up.
  { path: '/admin/users/edit?id={userId}', name: 'Edit user form', requiredData: ['userId'] },
] as const;

/**
 * Component scenarios to test
 * Each scenario describes a component state that should be tested for accessibility
 */
export const COMPONENT_SCENARIOS = [
  // Global Search
  {
    name: 'Global Search - closed',
    setup: async () => {}, // No setup needed, default state
    selector: '[x-data*="globalSearch"]',
    waitFor: '[x-data*="globalSearch"]',
  },
  {
    name: 'Global Search - open',
    setup: async (page: import('@playwright/test').Page) => {
      // Trigger search modal with keyboard shortcut
      await page.keyboard.press('Meta+k');
    },
    selector: '[role="dialog"], .search-modal, [x-data*="globalSearch"]',
    waitFor: 'input[type="search"], input[placeholder*="Search"]',
  },
  {
    name: 'Global Search - with results',
    setup: async (page: import('@playwright/test').Page) => {
      await page.keyboard.press('Meta+k');
      await page.waitForSelector('input[type="search"], input[placeholder*="Search"]');
      await page.keyboard.type('test');
      // Wait for results to load
      await page.waitForTimeout(500);
    },
    selector: '[x-data*="globalSearch"]',
    waitFor: 'input[type="search"], input[placeholder*="Search"]',
  },

  // Dropdown / Autocompleter
  {
    name: 'Autocompleter - closed',
    pagePath: '/note/new',
    selector: '[data-selector-field]',
    waitFor: '[data-selector-field]',
  },
  {
    name: 'Autocompleter - open with options',
    pagePath: '/note/new',
    setup: async (page: import('@playwright/test').Page) => {
      // Find and focus an autocompleter input
      const input = page.locator('[data-selector-field] input').first();
      await input.focus();
      await input.click();
    },
    selector: '[data-selector-field]',
    waitFor: '[data-selector-field]',
  },

  // Bulk Selection
  {
    name: 'Bulk Selection - none selected',
    pagePath: '/notes',
    selector: '[x-data*="bulkSelection"]',
    waitFor: '[x-data*="bulkSelection"]',
  },

  // Expandable Text
  {
    name: 'Expandable Text - collapsed',
    selector: 'expandable-text',
    waitFor: 'expandable-text',
    requiresEntity: true,
  },

  // Inline Edit
  {
    name: 'Inline Edit - view mode',
    selector: 'inline-edit',
    waitFor: 'inline-edit',
    requiresEntity: true,
  },

  // Confirm Action
  {
    name: 'Confirm Action button',
    selector: '[x-data*="confirmAction"]',
    waitFor: '[x-data*="confirmAction"]',
    requiresEntity: true,
  },
] as const;

/**
 * Test data IDs placeholder interface
 */
export interface A11yTestData {
  categoryId: number;
  resourceCategoryId: number;
  tagId: number;
  noteTypeId: number;
  relationTypeId: number;
  groupId: number;
  group2Id: number;
  noteId: number;
  queryId: number;
  relationId: number;
  userId: number;
}

/**
 * Replace placeholders in a path with actual data
 */
export function buildPath(pathTemplate: string, data: Partial<A11yTestData>): string {
  let path = pathTemplate;

  const replacements: Record<string, keyof A11yTestData> = {
    '{categoryId}': 'categoryId',
    '{resourceCategoryId}': 'resourceCategoryId',
    '{tagId}': 'tagId',
    '{noteTypeId}': 'noteTypeId',
    '{relationTypeId}': 'relationTypeId',
    '{groupId}': 'groupId',
    '{group2Id}': 'group2Id',
    '{noteId}': 'noteId',
    '{queryId}': 'queryId',
    '{relationId}': 'relationId',
    '{userId}': 'userId',
  };

  for (const [placeholder, key] of Object.entries(replacements)) {
    if (path.includes(placeholder) && data[key] !== undefined) {
      path = path.replace(placeholder, String(data[key]));
    }
  }

  return path;
}
