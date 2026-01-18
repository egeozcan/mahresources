/**
 * Configuration for accessibility tests
 */

/**
 * Static pages that can be tested without any pre-existing data
 * These pages either show empty states or are forms
 */
export const STATIC_PAGES = [
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

  // Create/new forms
  { path: '/note/new', name: 'Create note form' },
  { path: '/group/new', name: 'Create group form' },
  { path: '/resource/new', name: 'Create resource form' },
  { path: '/tag/new', name: 'Create tag form' },
  { path: '/category/new', name: 'Create category form' },
  { path: '/query/new', name: 'Create query form' },
  { path: '/noteType/new', name: 'Create note type form' },
  { path: '/relationType/new', name: 'Create relation type form' },
  { path: '/relation/new', name: 'Create relation form' },

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

  // Edit forms
  { path: '/note/edit?id={noteId}', name: 'Edit note form', requiredData: ['noteId'] },
  { path: '/group/edit?id={groupId}', name: 'Edit group form', requiredData: ['groupId'] },
  { path: '/tag/edit?id={tagId}', name: 'Edit tag form', requiredData: ['tagId'] },
  { path: '/category/edit?id={categoryId}', name: 'Edit category form', requiredData: ['categoryId'] },
  { path: '/query/edit?id={queryId}', name: 'Edit query form', requiredData: ['queryId'] },
  { path: '/noteType/edit?id={noteTypeId}', name: 'Edit note type form', requiredData: ['noteTypeId'] },
  { path: '/relationType/edit?id={relationTypeId}', name: 'Edit relation type form', requiredData: ['relationTypeId'] },
  { path: '/relation/edit?id={relationId}', name: 'Edit relation form', requiredData: ['relationId'] },
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
    selector: '[x-data*="dropdown"], [x-data*="autocompleter"]',
    waitFor: '[x-data*="dropdown"], [x-data*="autocompleter"]',
  },
  {
    name: 'Autocompleter - open with options',
    pagePath: '/note/new',
    setup: async (page: import('@playwright/test').Page) => {
      // Find and focus an autocompleter input
      const input = page.locator('[x-data*="dropdown"] input, [x-data*="autocompleter"] input').first();
      await input.focus();
      await input.click();
    },
    selector: '[x-data*="dropdown"], [x-data*="autocompleter"]',
    waitFor: '[x-data*="dropdown"], [x-data*="autocompleter"]',
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
  tagId: number;
  noteTypeId: number;
  relationTypeId: number;
  groupId: number;
  group2Id: number;
  noteId: number;
  queryId: number;
  relationId: number;
}

/**
 * Replace placeholders in a path with actual data
 */
export function buildPath(pathTemplate: string, data: Partial<A11yTestData>): string {
  let path = pathTemplate;

  const replacements: Record<string, keyof A11yTestData> = {
    '{categoryId}': 'categoryId',
    '{tagId}': 'tagId',
    '{noteTypeId}': 'noteTypeId',
    '{relationTypeId}': 'relationTypeId',
    '{groupId}': 'groupId',
    '{group2Id}': 'group2Id',
    '{noteId}': 'noteId',
    '{queryId}': 'queryId',
    '{relationId}': 'relationId',
  };

  for (const [placeholder, key] of Object.entries(replacements)) {
    if (path.includes(placeholder) && data[key] !== undefined) {
      path = path.replace(placeholder, String(data[key]));
    }
  }

  return path;
}
