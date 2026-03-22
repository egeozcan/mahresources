# Mahresources E2E Tests

End-to-end tests for the Mahresources application using Playwright.

## Test Coverage

- **107 spec files** across browser, CLI, accessibility, and plugin tests
- **Browser tests** (75 spec files): Tags, Categories, NoteTypes, Queries, RelationTypes, Groups, Notes, Resources, Relations, Bulk Operations, Global Search, Edge Cases, Lightbox, Versioning, Version Compare, Blocks, Note Sharing, Entity Picker, Series, and many regression tests
- **CLI tests** (20 spec files): Full coverage of the `mr` CLI binary against an ephemeral server
- **Accessibility tests** (4 spec files): axe-core WCAG compliance checks on pages and components
- **Plugin tests** (8 spec files): Plugin actions, API, blocks, hooks, injection, KV store, management, and pages

## Prerequisites

- Node.js 20+
- Docker and Docker Compose (for CI-style runs)
- The Mahresources app running locally (for local development)

## Quick Start

### Running with Docker (CI-style)

The recommended way to run tests in a reproducible environment:

```bash
# From the project root directory
docker-compose up --build

# Run and exit
docker-compose up --build --abort-on-container-exit
```

This will:
1. Build the Mahresources app container
2. Wait for it to be healthy
3. Run all Playwright tests
4. Output results to the console

### Running Locally (Automatic Server)

The easiest way to run tests locally - the script handles starting/stopping the server automatically:

```bash
# Install dependencies (first time only)
cd e2e
npm install
npx playwright install chromium

# Run all browser tests (starts server automatically on an available port)
npm run test:with-server

# Run tests with browser visible
npm run test:with-server:headed

# Run tests in debug mode
npm run test:with-server:debug

# Run accessibility tests only
npm run test:with-server:a11y

# Run CLI tests only
npm run test:with-server:cli

# Run both browser and CLI tests in parallel (separate ephemeral servers)
npm run test:with-server:all
```

The `test:with-server` scripts will:
1. Build the server binary if needed
2. Find an available port (starting from 8181)
3. Start the server in ephemeral mode with `-max-db-connections=2` to reduce SQLite lock contention
4. Wait for the server to be ready
5. Run the tests
6. Clean up the server process

### Running Locally (Manual Server)

For more control, you can start the server manually:

```bash
# Install dependencies (first time only)
cd e2e
npm install
npx playwright install chromium

# Start the app (from project root, in another terminal)
./mahresources -ephemeral -bind-address=:8181 -max-db-connections=2

# Run all browser tests
npm test

# Run CLI tests only
npm run test:cli

# Run accessibility tests only
npm run test:a11y

# Run tests with browser visible
npm run test:headed

# Run tests in debug mode (step through)
npm run test:debug

# Run tests with Playwright UI
npm run test:ui

# Run specific test file
npx playwright test tests/01-tag.spec.ts

# Run tests matching a pattern
npx playwright test --grep "should create"
```

## Test Structure

```
e2e/
├── fixtures/                    # Playwright test fixtures
│   ├── base.fixture.ts          # Provides page objects and API client for browser tests
│   ├── a11y.fixture.ts          # Extends base with axe-core accessibility testing
│   ├── cli.fixture.ts           # CliRunner helper for CLI tests
│   └── server-manager.ts        # Ephemeral server lifecycle management
├── helpers/
│   ├── api-client.ts            # API client for test setup/teardown
│   ├── cli-runner.ts            # CLI binary executor with retry logic for SQLite contention
│   └── accessibility/
│       ├── a11y-config.ts       # axe-core configuration
│       └── axe-helper.ts        # Accessibility assertion helpers
├── pages/                       # Page Object Model classes
│   ├── BasePage.ts              # Common page operations
│   ├── TagPage.ts
│   ├── CategoryPage.ts
│   ├── GroupPage.ts
│   ├── NotePage.ts
│   ├── ResourcePage.ts
│   ├── ResourceCategoryPage.ts
│   ├── QueryPage.ts
│   ├── NoteTypePage.ts
│   ├── RelationTypePage.ts
│   └── RelationPage.ts
├── scripts/
│   ├── run-tests.js             # Single-project test runner with auto server
│   └── run-all-tests.js         # Parallel runner for browser + CLI tests
├── tests/                       # Test specifications
│   ├── 01-tag.spec.ts
│   ├── 02-category.spec.ts
│   ├── 03-note-type.spec.ts
│   ├── 04-query.spec.ts
│   ├── 05-relation-type.spec.ts
│   ├── 06-group.spec.ts
│   ├── 07-note.spec.ts
│   ├── 08-resource.spec.ts
│   ├── 09-relation.spec.ts
│   ├── 10-bulk-operations.spec.ts
│   ├── 11-global-search.spec.ts
│   ├── 12-edge-cases.spec.ts
│   ├── 13-lightbox.spec.ts
│   ├── 14-resource-versioning.spec.ts
│   ├── 15-version-compare.spec.ts
│   ├── 16-blocks.spec.ts
│   ├── 17-block-state.spec.ts
│   ├── 18-block-backward-compat.spec.ts
│   ├── 19-block-calendar.spec.ts
│   ├── 19-note-sharing.spec.ts
│   ├── 20-entity-picker.spec.ts
│   ├── 21-resource-category.spec.ts
│   ├── 22-series.spec.ts
│   ├── 23-group-delete-preserves-resources.spec.ts
│   ├── 24-json-table-copy.spec.ts
│   ├── 25-tag-merge.spec.ts
│   ├── 26-paste-upload.spec.ts
│   ├── 27-autocompleter-remove-aria-label.spec.ts
│   ├── ...                      # (75 browser spec files total)
│   ├── dashboard.spec.ts
│   ├── accessibility/           # axe-core accessibility tests
│   │   ├── 01-a11y-pages.spec.ts
│   │   ├── 02-a11y-components.spec.ts
│   │   ├── 03-a11y-heading-and-pagination.spec.ts
│   │   └── 04-a11y-heading-level-skip.spec.ts
│   ├── cli/                     # CLI binary tests
│   │   ├── cli-categories.spec.ts
│   │   ├── cli-error-handling.spec.ts
│   │   ├── cli-global-flags.spec.ts
│   │   ├── cli-groups.spec.ts
│   │   ├── cli-jobs.spec.ts
│   │   ├── cli-logs.spec.ts
│   │   ├── cli-note-blocks.spec.ts
│   │   ├── cli-note-types.spec.ts
│   │   ├── cli-notes.spec.ts
│   │   ├── cli-output-formats.spec.ts
│   │   ├── cli-plugins.spec.ts
│   │   ├── cli-queries.spec.ts
│   │   ├── cli-relation-types.spec.ts
│   │   ├── cli-relations.spec.ts
│   │   ├── cli-resource-categories.spec.ts
│   │   ├── cli-resource-versions.spec.ts
│   │   ├── cli-resources.spec.ts
│   │   ├── cli-search.spec.ts
│   │   ├── cli-series.spec.ts
│   │   └── cli-tags.spec.ts
│   └── plugins/                 # Plugin system tests
│       ├── plugin-actions.spec.ts
│       ├── plugin-api.spec.ts
│       ├── plugin-blocks.spec.ts
│       ├── plugin-hooks.spec.ts
│       ├── plugin-injection.spec.ts
│       ├── plugin-kvstore.spec.ts
│       ├── plugin-manage.spec.ts
│       └── plugin-pages.spec.ts
├── test-assets/                 # Test files (images, etc.)
├── playwright.config.ts
├── tsconfig.json
└── package.json
```

## Test Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BASE_URL` | `http://localhost:8181` | App URL for tests |
| `CI` | - | Set in CI environments for stricter behavior |

### Playwright Config

Key settings in `playwright.config.ts`:

- **Projects**: `default` (browser tests, excludes `cli/`) and `cli` (CLI tests under `tests/cli/`)
- **Workers**: 4 locally, 1 in CI for browser tests; 2 locally, 1 in CI for CLI tests
- **Sequential within file**: `fullyParallel: false` ensures tests in a file run sequentially
- **Retries**: 4 in CI, 2 locally
- **Timeout**: 60 seconds per test
- **Artifacts**: Screenshots on failure, traces on first retry, videos retained on failure

### npm Scripts

| Script | Description |
|--------|-------------|
| `test` | Run all browser tests |
| `test:headed` | Run browser tests with visible browser |
| `test:debug` | Run browser tests in debug mode |
| `test:ui` | Run with Playwright UI |
| `test:a11y` | Run accessibility tests only |
| `test:a11y:headed` | Run accessibility tests with visible browser |
| `test:cli` | Run CLI tests only |
| `report` | Open the HTML test report |
| `test:with-server` | Auto-start server, run browser tests, clean up |
| `test:with-server:headed` | Auto-start server, run browser tests with visible browser |
| `test:with-server:debug` | Auto-start server, run browser tests in debug mode |
| `test:with-server:a11y` | Auto-start server, run accessibility tests |
| `test:with-server:cli` | Auto-start server, run CLI tests |
| `test:with-server:all` | Auto-start two servers, run browser + CLI tests in parallel |

## Writing Tests

### Using Page Objects

```typescript
import { test, expect } from '../fixtures/base.fixture';

test('should create a tag', async ({ tagPage }) => {
  const tagId = await tagPage.create('My Tag', 'Description');
  expect(tagId).toBeGreaterThan(0);
});
```

### Using the API Client

The API client is useful for test setup/teardown:

```typescript
test.beforeAll(async ({ apiClient }) => {
  // Clean up existing resources first (for test retries)
  const existingResources = await apiClient.getResources();
  for (const resource of existingResources) {
    await apiClient.deleteResource(resource.ID);
  }

  // Create prerequisite data via API
  const category = await apiClient.createCategory('Test Category');
  categoryId = category.ID;
});

test.afterAll(async ({ apiClient }) => {
  // Clean up via API
  await apiClient.deleteCategory(categoryId);
});
```

Available API client methods:
- **Tags**: `createTag`, `deleteTag`, `getTags`
- **Categories**: `createCategory`, `deleteCategory`, `getCategories`
- **NoteTypes**: `createNoteType`, `deleteNoteType`, `getNoteTypes`
- **Groups**: `createGroup`, `deleteGroup`, `getGroups`, `getGroup`
- **Notes**: `createNote`, `deleteNote`, `getNotes`
- **Queries**: `createQuery`, `deleteQuery`, `getQueries`
- **RelationTypes**: `createRelationType`, `deleteRelationType`, `getRelationTypes`
- **Relations**: `createRelation`, `deleteRelation`
- **Resources**: `getResources`, `deleteResource`
- **Search**: `search`
- **Bulk**: `addTagsToGroups`, `removeTagsFromGroups`, `bulkDeleteGroups`

### Serial vs Parallel Tests

Use `test.describe.serial()` when tests depend on each other:

```typescript
test.describe.serial('CRUD Operations', () => {
  let createdId: number;

  test('create', async ({ tagPage }) => {
    createdId = await tagPage.create('Test');
  });

  test('read', async ({ tagPage }) => {
    expect(createdId, 'Must be created first').toBeGreaterThan(0);
    await tagPage.gotoDisplay(createdId);
  });
});
```

## Viewing Test Reports

After running tests:

```bash
# Open the HTML report
npm run report

# Or manually
npx playwright show-report playwright-report
```

## Troubleshooting

### Tests fail with timeout errors

- Ensure the app is running and accessible at the configured `BASE_URL`
- Increase timeouts in `playwright.config.ts` if the app is slow to respond

### Tests fail in Docker but pass locally

- Check that the app healthcheck is working correctly
- Verify the `BASE_URL` is set to `http://app:8181` in Docker

### Flaky tests

- Tests have automatic retries configured
- Use `await expect(...).toBeVisible({ timeout: 5000 })` instead of hardcoded waits
- Check for race conditions in UI interactions

### Resource upload tests fail with "chrome-error://chromewebdata/"

This indicates a server-side crash during file upload. Check:
- Server logs for nil pointer dereference errors
- That the `FILE_SAVE_PATH` directory exists and is writable
- Database for existing resources with the same file hash

## CI Integration

The tests are designed to run in CI via Docker Compose. Example GitHub Actions workflow:

```yaml
- name: Run E2E tests
  run: docker-compose up --build --abort-on-container-exit
```
