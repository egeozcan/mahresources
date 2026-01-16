# Mahresources E2E Tests

End-to-end tests for the Mahresources application using Playwright.

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

### Running Locally

For faster iteration during development:

```bash
# Install dependencies (first time only)
cd e2e
npm install
npx playwright install chromium

# Start the app (from project root, in another terminal)
./mahresources

# Run all tests
npm test

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

- **Sequential execution**: Tests run one at a time (`workers: 1`, `fullyParallel: false`)
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
  // Create prerequisite data via API
  const category = await apiClient.createCategory('Test Category');
  categoryId = category.ID;
});

test.afterAll(async ({ apiClient }) => {
  // Clean up via API
  await apiClient.deleteCategory(categoryId);
});
```

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

## CI Integration

The tests are designed to run in CI via Docker Compose. Example GitHub Actions workflow:

```yaml
- name: Run E2E tests
  run: docker-compose up --build --abort-on-container-exit
```
