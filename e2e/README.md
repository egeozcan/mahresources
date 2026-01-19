# Mahresources E2E Tests

End-to-end tests for the Mahresources application using Playwright.

## Test Coverage

- **90 tests total** (89 passing, 1 intentionally skipped)
- Tests cover: Tags, Categories, NoteTypes, Queries, RelationTypes, Groups, Notes, Resources, Relations, Bulk Operations, Global Search, and Edge Cases
- **Skipped test**: "Resource from URL" - depends on external service (via.placeholder.com)

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

# Run all tests (starts server automatically on an available port)
npm run test:with-server

# Run tests with browser visible
npm run test:with-server:headed

# Run tests in debug mode
npm run test:with-server:debug

# Run accessibility tests only
npm run test:with-server:a11y
```

The `test:with-server` scripts will:
1. Build the server binary if needed
2. Find an available port (starting from 8181)
3. Start the server in ephemeral mode with `-max-db-connections=2` to reduce SQLite lock contention
4. Wait for the server to be ready
5. Run the tests with 2 workers (to further reduce database contention)
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

# Run all tests (use --workers=2 to reduce SQLite lock contention)
npm test -- --workers=2

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
├── fixtures/           # Playwright test fixtures
│   └── base.fixture.ts # Provides page objects and API client
├── helpers/
│   └── api-client.ts   # API client for test setup/teardown
├── pages/              # Page Object Model classes
│   ├── BasePage.ts     # Common page operations
│   ├── TagPage.ts
│   ├── CategoryPage.ts
│   ├── GroupPage.ts
│   ├── NotePage.ts
│   ├── ResourcePage.ts
│   ├── QueryPage.ts
│   ├── NoteTypePage.ts
│   ├── RelationTypePage.ts
│   └── RelationPage.ts
├── tests/              # Test specifications
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
│   └── 12-edge-cases.spec.ts
├── test-assets/        # Test files (images, etc.)
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

- **Workers**: 4 locally, 1 in CI (configurable via `--workers` flag)
- **Sequential within file**: `fullyParallel: false` ensures tests in a file run sequentially
- **Retries**: 2 in CI, 1 locally for flaky UI interactions
- **Artifacts**: Screenshots on failure, traces on first retry, videos retained on failure

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

## Resource Upload Tests

The resource tests (`08-resource.spec.ts`) test file upload functionality:

```typescript
test('should upload a file resource', async ({ resourcePage, page }) => {
  const testFilePath = path.join(__dirname, '../test-assets/sample-image.png');
  await resourcePage.gotoNew();

  // Set file via Playwright's setInputFiles
  const fileInput = page.locator('input[type="file"]');
  await fileInput.setInputFiles(testFilePath);

  // Fill other fields and submit
  await page.locator('input[name="Name"]').fill('My Image');
  await page.locator('button[type="submit"]').click();
});
```

**Note**: Resource tests clean up existing resources in `beforeAll` to handle test retries gracefully.

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
