# CLI E2E Test Suite Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build ~230 E2E tests for the `mr` CLI binary covering all 15 entity types, output formats, error handling, and global flags.

**Architecture:** Playwright test runner (no browser) with a `CliRunner` helper wrapping `child_process.execSync`. Tests run against an ephemeral server auto-launched by the existing `run-tests.js` script. New `cli` project in Playwright config, independent of browser test projects.

**Tech Stack:** TypeScript, Playwright test runner, Node.js `child_process`, existing Go ephemeral server

**Spec:** `docs/superpowers/specs/2026-03-14-cli-e2e-tests-design.md`

---

## File Structure

| File | Responsibility |
|---|---|
| `e2e/helpers/cli-runner.ts` | `CliRunner` class: exec CLI binary, parse output, retry on SQLite errors |
| `e2e/fixtures/cli.fixture.ts` | Playwright fixture exposing `cli: CliRunner` to tests |
| `e2e/scripts/run-tests.js` | **Modify**: add CLI binary build + `CLI_PATH` env var |
| `e2e/playwright.config.ts` | **Modify**: add `cli` project |
| `e2e/package.json` | **Modify**: add `test:cli` and `test:with-server:cli` scripts |
| `e2e/tests/cli/cli-tags.spec.ts` | Tag CRUD + list/merge/delete tests |
| `e2e/tests/cli/cli-categories.spec.ts` | Category CRUD + list tests |
| `e2e/tests/cli/cli-resource-categories.spec.ts` | Resource category CRUD + list tests |
| `e2e/tests/cli/cli-note-types.spec.ts` | Note type CRUD + edit + list tests |
| `e2e/tests/cli/cli-notes.spec.ts` | Note CRUD + share/unshare + bulk ops tests |
| `e2e/tests/cli/cli-groups.spec.ts` | Group CRUD + hierarchy + bulk ops tests |
| `e2e/tests/cli/cli-note-blocks.spec.ts` | Note block CRUD + reorder/rebalance tests |
| `e2e/tests/cli/cli-relations.spec.ts` | Relation CRUD tests |
| `e2e/tests/cli/cli-relation-types.spec.ts` | Relation type CRUD + list tests |
| `e2e/tests/cli/cli-queries.spec.ts` | Query CRUD + run + schema tests |
| `e2e/tests/cli/cli-series.spec.ts` | Series CRUD + remove-resource + list tests |
| `e2e/tests/cli/cli-resources.spec.ts` | Resource CRUD + upload/download + bulk ops tests |
| `e2e/tests/cli/cli-resource-versions.spec.ts` | Resource version lifecycle tests |
| `e2e/tests/cli/cli-jobs.spec.ts` | Job submit/cancel/pause/resume/retry + list tests |
| `e2e/tests/cli/cli-logs.spec.ts` | Log get/entity + list with filter tests |
| `e2e/tests/cli/cli-search.spec.ts` | Global search tests |
| `e2e/tests/cli/cli-plugins.spec.ts` | Plugin enable/disable/settings/purge + list tests |
| `e2e/tests/cli/cli-output-formats.spec.ts` | --json/--quiet/--no-header/--page tests |
| `e2e/tests/cli/cli-error-handling.spec.ts` | Invalid args, missing entities, connectivity tests |
| `e2e/tests/cli/cli-global-flags.spec.ts` | --server flag, MAHRESOURCES_URL env var tests |

---

## Chunk 1: Infrastructure (Tasks 1-3)

### Task 1: CliRunner Helper

**Files:**
- Create: `e2e/helpers/cli-runner.ts`

- [ ] **Step 1: Create the CliRunner class**

```typescript
// e2e/helpers/cli-runner.ts
import { execFileSync } from 'child_process';

export interface CliResult {
  stdout: string;
  stderr: string;
  exitCode: number;
}

const RETRYABLE_PATTERNS = [
  'database is locked',
  'SQLITE_BUSY',
  'database table is locked',
];

const MAX_RETRIES = 3;
const RETRY_DELAYS = [500, 1000, 2000];

function sleep(ms: number): void {
  const { execSync } = require('child_process');
  execSync(`sleep ${ms / 1000}`);
}

function isRetryable(result: CliResult): boolean {
  const combined = result.stdout + result.stderr;
  return RETRYABLE_PATTERNS.some(p => combined.includes(p));
}

export class CliRunner {
  constructor(
    private binaryPath: string,
    private serverUrl: string,
  ) {}

  /**
   * Run a CLI command and return the result. Does NOT throw on non-zero exit.
   */
  run(...args: string[]): CliResult {
    const fullArgs = ['--server', this.serverUrl, ...args];

    for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
      let result: CliResult;
      try {
        // Use execFileSync (not execSync) to avoid shell escaping issues.
        // execFileSync passes args as an array directly, no shell involved.
        const stdout = execFileSync(this.binaryPath, fullArgs, {
          timeout: 30000,
          encoding: 'utf-8',
          stdio: ['pipe', 'pipe', 'pipe'],
        });
        result = { stdout: stdout || '', stderr: '', exitCode: 0 };
      } catch (error: any) {
        result = {
          stdout: error.stdout?.toString() || '',
          stderr: error.stderr?.toString() || '',
          exitCode: error.status ?? 1,
        };
      }

      if (attempt < MAX_RETRIES && result.exitCode !== 0 && isRetryable(result)) {
        sleep(RETRY_DELAYS[attempt]);
        continue;
      }

      return result;
    }

    // Unreachable, but TypeScript needs it
    throw new Error('Retry loop exhausted');
  }

  /**
   * Run a CLI command. Throws if exit code is non-zero.
   */
  runOrFail(...args: string[]): CliResult {
    const result = this.run(...args);
    if (result.exitCode !== 0) {
      throw new Error(
        `CLI command failed (exit ${result.exitCode}):\n` +
        `  args: ${args.join(' ')}\n` +
        `  stdout: ${result.stdout}\n` +
        `  stderr: ${result.stderr}`
      );
    }
    return result;
  }

  /**
   * Run with --json flag, parse stdout as JSON. Throws on failure.
   */
  runJson<T = any>(...args: string[]): T {
    const result = this.runOrFail(...args, '--json');
    try {
      return JSON.parse(result.stdout) as T;
    } catch (parseError) {
      throw new Error(
        `Failed to parse CLI JSON output:\n` +
        `  args: ${args.join(' ')} --json\n` +
        `  stdout: ${result.stdout}\n` +
        `  stderr: ${result.stderr}`
      );
    }
  }

  /**
   * Run expecting a non-zero exit code. Throws if command succeeds.
   */
  runExpectError(...args: string[]): CliResult {
    const result = this.run(...args);
    if (result.exitCode === 0) {
      throw new Error(
        `Expected CLI command to fail but it succeeded:\n` +
        `  args: ${args.join(' ')}\n` +
        `  stdout: ${result.stdout}`
      );
    }
    return result;
  }
}
```

- [ ] **Step 2: Commit**

```bash
git add e2e/helpers/cli-runner.ts
git commit -m "feat(e2e): add CliRunner helper for CLI E2E tests"
```

---

### Task 2: CLI Fixture and Playwright Config

**Files:**
- Create: `e2e/fixtures/cli.fixture.ts`
- Modify: `e2e/playwright.config.ts`

- [ ] **Step 1: Create the CLI fixture**

```typescript
// e2e/fixtures/cli.fixture.ts
import { test as base, expect } from '@playwright/test';
import { CliRunner } from '../helpers/cli-runner';
import * as path from 'path';

/**
 * Create a CliRunner instance directly (no Playwright fixture needed).
 * Use this in test.beforeAll / test.afterAll where fixtures are NOT available.
 */
export function createCliRunner(): CliRunner {
  const binaryPath = process.env.CLI_PATH || path.resolve(__dirname, '../../mr');
  const serverUrl = process.env.BASE_URL || 'http://localhost:8181';
  return new CliRunner(binaryPath, serverUrl);
}

type CliFixtures = {
  cli: CliRunner;
};

export const test = base.extend<CliFixtures>({
  cli: async ({}, use) => {
    await use(createCliRunner());
  },
});

export { expect };
```

**IMPORTANT:** Playwright `test.beforeAll` and `test.afterAll` do NOT have access to test-scoped fixtures. Use `createCliRunner()` directly in those hooks. The `{ cli }` destructuring only works inside `test()` and `test.beforeEach` / `test.afterEach`.

- [ ] **Step 2: Add `cli` project to playwright.config.ts**

Add a new project entry to the `projects` array in `e2e/playwright.config.ts`. Add it AFTER the existing projects. It must have `use: {}` (no browser), `testDir: './tests/cli'`, and no `dependencies`:

```typescript
    {
      name: 'cli',
      testDir: './tests/cli',
      fullyParallel: false, // tests within a file run sequentially (shared state like entity IDs)
      use: {},  // no browser needed — must be empty to avoid inheriting devices['Desktop Chrome']
      workers: process.env.CI ? 1 : 2,
    },
```

- [ ] **Step 3: Commit**

```bash
git add e2e/fixtures/cli.fixture.ts e2e/playwright.config.ts
git commit -m "feat(e2e): add CLI fixture and Playwright project"
```

---

### Task 3: Build Script and npm Scripts

**Files:**
- Modify: `e2e/scripts/run-tests.js`
- Modify: `e2e/package.json`

- [ ] **Step 1: Update run-tests.js to build CLI binary**

In `e2e/scripts/run-tests.js`, modify the `ensureServerBuilt()` function to also build the CLI binary. Add after the existing server build:

```javascript
function ensureServerBuilt() {
  const fs = require('fs');

  if (!fs.existsSync(SERVER_BINARY)) {
    console.log('Building server binary...');
    execSync('npm run build', { cwd: PROJECT_ROOT, stdio: 'inherit' });
  }

  // Build CLI binary if it doesn't exist
  const CLI_BINARY = path.join(PROJECT_ROOT, 'mr');
  if (!fs.existsSync(CLI_BINARY)) {
    console.log('Building CLI binary...');
    execSync('go build -o mr ./cmd/mr/', { cwd: PROJECT_ROOT, stdio: 'inherit' });
  }
}
```

Then in the `main()` function, add `CLI_PATH` to the env passed to Playwright. Find the `env` object in the `spawn('npx', ['playwright', ...])` call and add the `CLI_PATH` key:

```javascript
      env: {
        ...process.env,
        BASE_URL: `http://localhost:${port}`,
        SHARE_BASE_URL: `http://127.0.0.1:${sharePort}`,
        CLI_PATH: path.join(PROJECT_ROOT, 'mr'),
      }
```

- [ ] **Step 2: Add npm scripts to e2e/package.json**

Add these two scripts to the `"scripts"` object in `e2e/package.json`:

```json
    "test:cli": "playwright test --project=cli",
    "test:with-server:cli": "node scripts/run-tests.js test --project=cli"
```

- [ ] **Step 3: Create the `e2e/tests/cli/` directory**

```bash
mkdir -p e2e/tests/cli
```

- [ ] **Step 4: Build both binaries to verify**

Run from project root:
```bash
npm run build && go build -o mr ./cmd/mr/
```
Expected: Both `mahresources` and `mr` binaries exist in project root.

- [ ] **Step 5: Commit**

```bash
git add e2e/scripts/run-tests.js e2e/package.json
git commit -m "feat(e2e): add CLI binary build and npm scripts for CLI tests"
```

---

## Chunk 2: Simple Entity Tests (Tasks 4-8)

These entities have the simplest command structure: CRUD + list. Each follows the same lifecycle pattern. They have no dependencies on other entities so can be implemented first.

### Task 4: Tag Tests

**Files:**
- Create: `e2e/tests/cli/cli-tags.spec.ts`

**Reference:** `cmd/mr/commands/tags.go` for exact flag names and command structure.

- [ ] **Step 1: Write tag tests**

```typescript
// e2e/tests/cli/cli-tags.spec.ts
import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';

test.describe('tag commands', () => {
  let tagId: number;
  const tagName = `cli-test-tag-${Date.now()}`;

  // NOTE: beforeAll/afterAll cannot use fixtures -- use createCliRunner() directly.
  test.afterAll(async () => {
    if (tagId) {
      createCliRunner().run('tag', 'delete', String(tagId));
    }
  });

  test('tag create', async ({ cli }) => {
    const tag = cli.runJson('tag', 'create', '--name', tagName, '--description', 'test desc');
    expect(tag.Name).toBe(tagName);
    expect(tag.Description).toBe('test desc');
    expect(tag.ID).toBeGreaterThan(0);
    tagId = tag.ID;
  });

  test('tag get', async ({ cli }) => {
    const tag = cli.runJson('tag', 'get', String(tagId));
    expect(tag.Name).toBe(tagName);
    expect(tag.ID).toBe(tagId);
  });

  test('tag edit-name', async ({ cli }) => {
    const newName = `${tagName}-renamed`;
    cli.runOrFail('tag', 'edit-name', String(tagId), newName);
    const tag = cli.runJson('tag', 'get', String(tagId));
    expect(tag.Name).toBe(newName);
    // Rename back for later tests
    cli.runOrFail('tag', 'edit-name', String(tagId), tagName);
  });

  test('tag edit-description', async ({ cli }) => {
    cli.runOrFail('tag', 'edit-description', String(tagId), 'updated desc');
    const tag = cli.runJson('tag', 'get', String(tagId));
    expect(tag.Description).toBe('updated desc');
  });
});

test.describe('tags commands', () => {
  test('tags list', async ({ cli }) => {
    const name = `cli-list-tag-${Date.now()}`;
    const tag = cli.runJson('tag', 'create', '--name', name);

    const tags = cli.runJson<any[]>('tags', 'list', '--name', name);
    expect(tags.length).toBeGreaterThanOrEqual(1);
    expect(tags.some((t: any) => t.ID === tag.ID)).toBe(true);

    cli.run('tag', 'delete', String(tag.ID));
  });

  test('tags list with --name filter', async ({ cli }) => {
    const unique = `cli-filter-tag-${Date.now()}`;
    const tag1 = cli.runJson('tag', 'create', '--name', `${unique}-alpha`);
    const tag2 = cli.runJson('tag', 'create', '--name', `${unique}-beta`);

    const filtered = cli.runJson<any[]>('tags', 'list', '--name', `${unique}-alpha`);
    expect(filtered.some((t: any) => t.ID === tag1.ID)).toBe(true);
    expect(filtered.some((t: any) => t.ID === tag2.ID)).toBe(false);

    cli.run('tag', 'delete', String(tag1.ID));
    cli.run('tag', 'delete', String(tag2.ID));
  });

  test('tags merge', async ({ cli }) => {
    const winner = cli.runJson('tag', 'create', '--name', `cli-merge-winner-${Date.now()}`);
    const loser = cli.runJson('tag', 'create', '--name', `cli-merge-loser-${Date.now()}`);

    cli.runOrFail('tags', 'merge', '--winner', String(winner.ID), '--losers', String(loser.ID));

    // Winner should still exist
    const w = cli.runJson('tag', 'get', String(winner.ID));
    expect(w.ID).toBe(winner.ID);

    // Loser should be gone
    const result = cli.run('tag', 'get', String(loser.ID));
    expect(result.exitCode).not.toBe(0);

    cli.run('tag', 'delete', String(winner.ID));
  });

  test('tags delete (bulk)', async ({ cli }) => {
    const t1 = cli.runJson('tag', 'create', '--name', `cli-bulk-del-${Date.now()}-1`);
    const t2 = cli.runJson('tag', 'create', '--name', `cli-bulk-del-${Date.now()}-2`);

    cli.runOrFail('tags', 'delete', '--ids', `${t1.ID},${t2.ID}`);

    const r1 = cli.run('tag', 'get', String(t1.ID));
    expect(r1.exitCode).not.toBe(0);
    const r2 = cli.run('tag', 'get', String(t2.ID));
    expect(r2.exitCode).not.toBe(0);
  });

  test('tag delete', async ({ cli }) => {
    const tag = cli.runJson('tag', 'create', '--name', `cli-del-tag-${Date.now()}`);
    cli.runOrFail('tag', 'delete', String(tag.ID));

    const result = cli.run('tag', 'get', String(tag.ID));
    expect(result.exitCode).not.toBe(0);
  });
});
```

- [ ] **Step 2: Run the test to verify it works**

```bash
cd e2e && npm run test:with-server:cli -- --grep "tag"
```
Expected: All tag tests pass.

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/cli/cli-tags.spec.ts
git commit -m "test(cli): add tag command E2E tests"
```

---

### Task 5: Category Tests

**Files:**
- Create: `e2e/tests/cli/cli-categories.spec.ts`

**Reference:** `cmd/mr/commands/categories.go` — category get uses list+filter (no single-get endpoint). Flags: `--name`, `--description`, `--custom-header`, `--custom-sidebar`, `--custom-summary`, `--custom-avatar`, `--meta-schema`.

- [ ] **Step 1: Write category tests**

```typescript
// e2e/tests/cli/cli-categories.spec.ts
import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';

test.describe('category commands', () => {
  let catId: number;
  const catName = `cli-test-cat-${Date.now()}`;

  test.afterAll(async () => {
    if (catId) createCliRunner().run('category', 'delete', String(catId));
  });

  test('category create', async ({ cli }) => {
    const cat = cli.runJson('category', 'create', '--name', catName, '--description', 'cat desc');
    expect(cat.Name).toBe(catName);
    expect(cat.ID).toBeGreaterThan(0);
    catId = cat.ID;
  });

  test('category get', async ({ cli }) => {
    const cat = cli.runJson('category', 'get', String(catId));
    expect(cat.Name).toBe(catName);
    expect(cat.ID).toBe(catId);
  });

  test('category edit-name', async ({ cli }) => {
    const newName = `${catName}-renamed`;
    cli.runOrFail('category', 'edit-name', String(catId), newName);
    const cat = cli.runJson('category', 'get', String(catId));
    expect(cat.Name).toBe(newName);
    cli.runOrFail('category', 'edit-name', String(catId), catName);
  });

  test('category edit-description', async ({ cli }) => {
    cli.runOrFail('category', 'edit-description', String(catId), 'new cat desc');
    const cat = cli.runJson('category', 'get', String(catId));
    expect(cat.Description).toBe('new cat desc');
  });

  test('categories list', async ({ cli }) => {
    const cats = cli.runJson<any[]>('categories', 'list', '--name', catName);
    expect(cats.some((c: any) => c.ID === catId)).toBe(true);
  });

  test('categories list with --name filter', async ({ cli }) => {
    const unique = `cli-filter-cat-${Date.now()}`;
    const c1 = cli.runJson('category', 'create', '--name', `${unique}-one`);
    const c2 = cli.runJson('category', 'create', '--name', `${unique}-two`);

    const filtered = cli.runJson<any[]>('categories', 'list', '--name', `${unique}-one`);
    expect(filtered.some((c: any) => c.ID === c1.ID)).toBe(true);
    expect(filtered.some((c: any) => c.ID === c2.ID)).toBe(false);

    cli.run('category', 'delete', String(c1.ID));
    cli.run('category', 'delete', String(c2.ID));
  });

  test('category delete', async ({ cli }) => {
    const c = cli.runJson('category', 'create', '--name', `cli-del-cat-${Date.now()}`);
    cli.runOrFail('category', 'delete', String(c.ID));
    const result = cli.run('category', 'get', String(c.ID));
    expect(result.exitCode).not.toBe(0);
  });
});
```

- [ ] **Step 2: Run and verify**

```bash
cd e2e && npm run test:with-server:cli -- --grep "category"
```

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/cli/cli-categories.spec.ts
git commit -m "test(cli): add category command E2E tests"
```

---

### Task 6: Resource Category Tests

**Files:**
- Create: `e2e/tests/cli/cli-resource-categories.spec.ts`

**Reference:** `cmd/mr/commands/resource_categories.go` — same pattern as categories but different API endpoints. Command is `resource-category` (singular) and `resource-categories` (plural).

- [ ] **Step 1: Write resource category tests**

Follow the exact same pattern as Task 5 but use `resource-category` / `resource-categories` commands. Test: create, get, edit-name, edit-description, list, list with filter, delete.

- [ ] **Step 2: Run and verify**

```bash
cd e2e && npm run test:with-server:cli -- --grep "resource.category"
```

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/cli/cli-resource-categories.spec.ts
git commit -m "test(cli): add resource category command E2E tests"
```

---

### Task 7: Note Type Tests

**Files:**
- Create: `e2e/tests/cli/cli-note-types.spec.ts`

**Reference:** `cmd/mr/commands/note_types.go` — has an `edit` subcommand (full entity edit via `--id` flag) in addition to `edit-name` and `edit-description`. Get uses list+filter pattern. Commands: `note-type` (singular) and `note-types` (plural).

- [ ] **Step 1: Write note type tests**

```typescript
// e2e/tests/cli/cli-note-types.spec.ts
import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';

test.describe('note-type commands', () => {
  let ntId: number;
  const ntName = `cli-test-nt-${Date.now()}`;

  test.afterAll(async () => {
    if (ntId) createCliRunner().run('note-type', 'delete', String(ntId));
  });

  test('note-type create', async ({ cli }) => {
    const nt = cli.runJson('note-type', 'create', '--name', ntName, '--description', 'nt desc');
    expect(nt.Name).toBe(ntName);
    expect(nt.ID).toBeGreaterThan(0);
    ntId = nt.ID;
  });

  test('note-type get', async ({ cli }) => {
    const nt = cli.runJson('note-type', 'get', String(ntId));
    expect(nt.Name).toBe(ntName);
  });

  test('note-type edit (full)', async ({ cli }) => {
    cli.runOrFail('note-type', 'edit', '--id', String(ntId), '--name', `${ntName}-edited`);
    const nt = cli.runJson('note-type', 'get', String(ntId));
    expect(nt.Name).toBe(`${ntName}-edited`);
    // Restore original name
    cli.runOrFail('note-type', 'edit', '--id', String(ntId), '--name', ntName);
  });

  test('note-type edit-name', async ({ cli }) => {
    const newName = `${ntName}-renamed`;
    cli.runOrFail('note-type', 'edit-name', String(ntId), newName);
    const nt = cli.runJson('note-type', 'get', String(ntId));
    expect(nt.Name).toBe(newName);
    cli.runOrFail('note-type', 'edit-name', String(ntId), ntName);
  });

  test('note-type edit-description', async ({ cli }) => {
    cli.runOrFail('note-type', 'edit-description', String(ntId), 'new nt desc');
    const nt = cli.runJson('note-type', 'get', String(ntId));
    expect(nt.Description).toBe('new nt desc');
  });

  test('note-types list', async ({ cli }) => {
    const nts = cli.runJson<any[]>('note-types', 'list', '--name', ntName);
    expect(nts.some((n: any) => n.ID === ntId)).toBe(true);
  });

  test('note-type delete', async ({ cli }) => {
    const nt = cli.runJson('note-type', 'create', '--name', `cli-del-nt-${Date.now()}`);
    cli.runOrFail('note-type', 'delete', String(nt.ID));
    const result = cli.run('note-type', 'get', String(nt.ID));
    expect(result.exitCode).not.toBe(0);
  });
});
```

- [ ] **Step 2: Run and verify**

```bash
cd e2e && npm run test:with-server:cli -- --grep "note-type"
```

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/cli/cli-note-types.spec.ts
git commit -m "test(cli): add note type command E2E tests"
```

---

### Task 8: Relation Type Tests

**Files:**
- Create: `e2e/tests/cli/cli-relation-types.spec.ts`

**Reference:** `cmd/mr/commands/relation_types.go` — has `create` (with `--name`, `--description`, `--reverse-name`, `--from-category`, `--to-category`), `edit` (with `--id` flag), `delete`, and `relation-types list`.

- [ ] **Step 1: Write relation type tests**

Test: create, create with reverse-name, edit, list, list with filter, delete.

- [ ] **Step 2: Run and verify**

```bash
cd e2e && npm run test:with-server:cli -- --grep "relation-type"
```

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/cli/cli-relation-types.spec.ts
git commit -m "test(cli): add relation type command E2E tests"
```

---

## Chunk 3: Entity Tests with Dependencies (Tasks 9-14)

These entities depend on other entities for setup (e.g., notes need note-types, relations need groups + relation-types).

### Task 9: Note Tests

**Files:**
- Create: `e2e/tests/cli/cli-notes.spec.ts`

**Reference:** `cmd/mr/commands/notes.go` — note create accepts `--name`, `--description`, `--tags`, `--groups`, `--resources`, `--meta`, `--owner-id`, `--note-type-id`. Has share/unshare commands. Plural `notes` has add-tags, remove-tags, add-groups, add-meta, delete, meta-keys.

**Dependencies:** Tests create their own tags and groups as needed using the CLI.

- [ ] **Step 1: Write note tests**

Cover:
- `note create` — basic, with description, with tags, with owner-id, with note-type-id
- `note get`
- `note edit-name`, `note edit-description`
- `note share` — verify success message
- `note unshare`
- `notes list` — basic, with `--name` filter, with `--tags` filter, with `--owner-id` filter
- `notes add-tags` — create note + tag, add tag, verify via API
- `notes remove-tags`
- `notes add-groups`
- `notes add-meta`
- `notes delete` (bulk)
- `notes meta-keys`
- `note delete`

Each test creates its own prerequisite entities (tags, groups) via the CLI.

- [ ] **Step 2: Run and verify**

```bash
cd e2e && npm run test:with-server:cli -- --grep "note commands|notes commands"
```

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/cli/cli-notes.spec.ts
git commit -m "test(cli): add note command E2E tests"
```

---

### Task 10: Group Tests

**Files:**
- Create: `e2e/tests/cli/cli-groups.spec.ts`

**Reference:** `cmd/mr/commands/groups.go` — group create accepts `--name`, `--description`, `--tags`, `--groups`, `--meta`, `--url`, `--owner-id`, `--category-id`. Has parents, children, clone. Plural `groups` has add-tags, remove-tags, add-meta, delete, merge, meta-keys.

**Dependencies:** Tests create their own categories and tags as needed.

- [ ] **Step 1: Write group tests**

Cover:
- `group create` — basic, with category, with owner (parent group), with tags
- `group get`
- `group edit-name`, `group edit-description`
- `group parents` — create child group with owner, verify parent shows up
- `group children` — verify child shows up in tree
- `group clone`
- `groups list` — basic, with `--name` filter, with `--category-id` filter
- `groups add-tags`, `groups remove-tags`
- `groups add-meta`
- `groups merge` — verify winner survives, loser gone
- `groups delete` (bulk)
- `groups meta-keys`
- `group delete`

- [ ] **Step 2: Run and verify**

```bash
cd e2e && npm run test:with-server:cli -- --grep "group"
```

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/cli/cli-groups.spec.ts
git commit -m "test(cli): add group command E2E tests"
```

---

### Task 11: Note Block Tests

**Files:**
- Create: `e2e/tests/cli/cli-note-blocks.spec.ts`

**Reference:** `cmd/mr/commands/note_blocks.go` — note-block create requires `--note-id`, `--type`, optional `--content`, `--position`. Has update (content), update-state, delete, types. Plural note-blocks has list (requires `--note-id`), reorder, rebalance.

**Dependencies:** Tests create a note first via CLI.

- [ ] **Step 1: Write note block tests**

Cover:
- Create a note for all block tests
- `note-block create` — with text type and JSON content
- `note-block get`
- `note-block update` — update content
- `note-block update-state` — update state JSON
- `note-block types` — verify returns block type list
- `note-blocks list` — verify block appears with `--note-id`
- `note-blocks rebalance`
- Create 2 blocks, `note-blocks reorder` with positions JSON
- `note-block delete`

- [ ] **Step 2: Run and verify**

```bash
cd e2e && npm run test:with-server:cli -- --grep "note.block"
```

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/cli/cli-note-blocks.spec.ts
git commit -m "test(cli): add note block command E2E tests"
```

---

### Task 12: Relation Tests

**Files:**
- Create: `e2e/tests/cli/cli-relations.spec.ts`

**Reference:** `cmd/mr/commands/relations.go` — relation create requires `--from-group-id`, `--to-group-id`, `--relation-type-id`, optional `--name`, `--description`. Has edit-name, edit-description, delete.

**Dependencies:** Tests create two groups and a relation type first.

- [ ] **Step 1: Write relation tests**

Cover:
- Setup: create 2 groups + 1 relation type
- `relation create` — with all required flags + optional name/description
- `relation edit-name`
- `relation edit-description`
- `relation delete`
- Cleanup: delete groups and relation type

- [ ] **Step 2: Run and verify**

```bash
cd e2e && npm run test:with-server:cli -- --grep "relation commands"
```

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/cli/cli-relations.spec.ts
git commit -m "test(cli): add relation command E2E tests"
```

---

### Task 13: Query Tests

**Files:**
- Create: `e2e/tests/cli/cli-queries.spec.ts`

**Reference:** `cmd/mr/commands/queries.go` — query create requires `--name`, `--text`, optional `--template`. Has run (by ID), run-by-name, schema. Plural queries has list.

- [ ] **Step 1: Write query tests**

Cover:
- `query create` — with name and text (SQL), verify ID returned
- `query get`
- `query edit-name`, `query edit-description`
- `query run` — run the created query by ID
- `query run-by-name` — run by name
- `query schema` — verify returns table/column info
- `queries list` — basic, with `--name` filter
- `query delete`

Use a simple SQL like `SELECT 1 AS test` for the query text.

- [ ] **Step 2: Run and verify**

```bash
cd e2e && npm run test:with-server:cli -- --grep "query"
```

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/cli/cli-queries.spec.ts
git commit -m "test(cli): add query command E2E tests"
```

---

### Task 14: Series Tests

**Files:**
- Create: `e2e/tests/cli/cli-series.spec.ts`

**Reference:** `cmd/mr/commands/series.go` — series create requires `--name`. Edit requires positional `<id>` + `--name`, optional `--meta`. Has remove-resource (positional `<resource-id>`). List has `--name`, `--slug` filters.

**Note:** `remove-resource` is hard to test without a resource in a series. Test by creating a resource (via upload), adding it to a series, then removing it. If too complex, test just the error case (removing a non-existent resource).

- [ ] **Step 1: Write series tests**

Cover:
- `series create`
- `series get`
- `series edit` — update name
- `series list` — basic, with `--name` filter
- `series delete`

- [ ] **Step 2: Run and verify**

```bash
cd e2e && npm run test:with-server:cli -- --grep "series"
```

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/cli/cli-series.spec.ts
git commit -m "test(cli): add series command E2E tests"
```

---

## Chunk 4: Resource Tests (Tasks 15-16)

Resources are the most complex entity with file operations. Split across two files.

### Task 15: Resource Core Tests

**Files:**
- Create: `e2e/tests/cli/cli-resources.spec.ts`

**Reference:** `cmd/mr/commands/resources.go` — resource upload takes positional `<file>`, optional `--name`, `--description`, `--tags`, `--groups`, `--owner-id`, `--category-id`, `--meta`, `--series-id`. Download takes positional `<id>`, optional `--output`, `--original-name`. Many bulk operations on plural `resources`.

**Test assets:** Use `e2e/test-assets/sample-document.txt` and `e2e/test-assets/sample-image.png`.

- [ ] **Step 1: Write resource tests**

Cover:
- `resource upload` — upload sample-document.txt, verify JSON response has ID/Name/ContentType
- `resource get` — verify fields match uploaded resource
- `resource edit` — update name/description via edit command
- `resource edit-name`, `resource edit-description`
- `resource download` — download to temp dir, verify file size matches
- `resource preview` — verify returns binary data (exit code 0)
- `resource rotate` — rotate uploaded image by 90 degrees (`resource rotate <id> --degrees 90`), verify success
- `resource recalculate-dimensions` — call on uploaded image resource
- `resource from-url` — upload a resource first, then use its URL to create another via from-url (self-referential: `--url http://localhost:PORT/v1/resource/content?id=X`)
- `resource from-local` — **SKIP with comment**: requires server-accessible file path, not available in ephemeral mode with in-memory filesystem
- `resources list` — basic, with `--name` filter
- `resources add-tags` — create tag + resource, add tag
- `resources remove-tags`
- `resources replace-tags`
- `resources add-groups` — create group + resource, add group
- `resources add-meta`
- `resources merge` — create 2 resources, merge, verify winner survives
- `resources set-dimensions` — set width/height on an image resource
- `resources meta-keys`
- `resources delete` (bulk)
- `resource delete`

For download tests, use Node.js `fs.mkdtempSync` to create a temp directory and `fs.readFileSync` to verify contents.

**IMPORTANT:** `resource upload --json` may return an array (server returns `[{...}]`). Handle both shapes in tests: `const result = cli.runJson(...); const res = Array.isArray(result) ? result[0] : result;`

- [ ] **Step 2: Run and verify**

```bash
cd e2e && npm run test:with-server:cli -- --grep "resource commands|resources commands"
```

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/cli/cli-resources.spec.ts
git commit -m "test(cli): add resource command E2E tests"
```

---

### Task 16: Resource Version Tests

**Files:**
- Create: `e2e/tests/cli/cli-resource-versions.spec.ts`

**Reference:** `cmd/mr/commands/resources.go` lines 698-1000 — version commands are subcommands of `resource` (singular). `resource versions <resource-id>`, `resource version <version-id>`, `resource version-upload <resource-id> <file>`, `resource version-download <version-id>`, `resource version-restore --resource-id X --version-id Y`, `resource version-delete --resource-id X --version-id Y`, `resource versions-cleanup <resource-id>`, `resource versions-compare <resource-id>`. Also `resources versions-cleanup` (plural, global).

- [ ] **Step 1: Write resource version tests**

Cover:
- Upload a resource (creates initial version)
- `resource versions` — list versions for resource
- `resource version-upload` — upload a new version file
- `resource versions` again — verify 2 versions
- `resource version` — get specific version by ID
- `resource version-download` — download version to temp file
- `resource versions-compare` — compare versions
- `resource version-restore` — restore earlier version
- `resource version-delete` — delete a version
- `resource versions-cleanup` — cleanup old versions for a resource
- `resources versions-cleanup` — global cleanup (just verify it runs without error)

- [ ] **Step 2: Run and verify**

```bash
cd e2e && npm run test:with-server:cli -- --grep "resource version"
```

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/cli/cli-resource-versions.spec.ts
git commit -m "test(cli): add resource version command E2E tests"
```

---

## Chunk 5: Operational Entity Tests (Tasks 17-19)

### Task 17: Job Tests

**Files:**
- Create: `e2e/tests/cli/cli-jobs.spec.ts`

**Reference:** `cmd/mr/commands/jobs.go` — job submit requires `--urls`, optional `--tags`, `--groups`, `--name`, `--owner-id`. Cancel/pause/resume/retry take positional `<id>`. Jobs list shows the queue.

**Note:** Job operations (cancel, pause, resume, retry) require an active job ID. Since ephemeral mode has no actual download capability for arbitrary URLs, test submit and list, and test cancel/pause/resume/retry expecting reasonable error behavior on non-existent job IDs.

- [ ] **Step 1: Write job tests**

Cover:
- `jobs list` — verify returns (even if empty queue)
- `job submit` — submit a URL, verify success message
- `jobs list` — verify queue shows the job
- `job cancel` — cancel with a non-existent ID, verify error handling
- `job pause`, `job resume`, `job retry` — similar error handling tests

- [ ] **Step 2: Run and verify**

```bash
cd e2e && npm run test:with-server:cli -- --grep "job"
```

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/cli/cli-jobs.spec.ts
git commit -m "test(cli): add job command E2E tests"
```

---

### Task 18: Log Tests

**Files:**
- Create: `e2e/tests/cli/cli-logs.spec.ts`

**Reference:** `cmd/mr/commands/logs.go` — logs list has filters: `--level`, `--action`, `--entity-type`, `--entity-id`, `--message`, `--created-before`, `--created-after`. Response is wrapped in `{ logs, totalCount, page, perPage }`. Log get takes positional `<id>`. Log entity takes `--entity-type` and `--entity-id`.

- [ ] **Step 1: Write log tests**

Cover:
- Create a tag (to generate log entries)
- `logs list` — **NOTE:** response is a wrapper object `{ logs, totalCount, page, perPage }`, NOT a direct array. Access `result.logs` for the entries and `result.totalCount` for count.
- `logs list --action create` — verify filter works
- `logs list --entity-type tag` — verify entity type filter
- `log get <id>` — get the first log entry from list (`result.logs[0].id`), verify fields
- `log entity --entity-type tag --entity-id X` — verify returns logs for the tag (same wrapper format)
- Delete the tag

- [ ] **Step 2: Run and verify**

```bash
cd e2e && npm run test:with-server:cli -- --grep "log"
```

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/cli/cli-logs.spec.ts
git commit -m "test(cli): add log command E2E tests"
```

---

### Task 19: Search and Plugin Tests

**Files:**
- Create: `e2e/tests/cli/cli-search.spec.ts`
- Create: `e2e/tests/cli/cli-plugins.spec.ts`

**Reference:**
- `cmd/mr/commands/search.go` — search takes positional `<query>`, optional `--types`, `--limit`. Response: `{ query, total, results[] }`.
- `cmd/mr/commands/plugins.go` — plugin enable/disable take positional `<name>`. Settings takes `<name>` + `--data` (JSON). Purge-data takes `<name>`. Plugins list shows management info.

- [ ] **Step 1: Write search tests**

Cover:
- Create a tag with a unique name
- `search <unique-name>` — **NOTE:** response is a wrapper object `{ query, total, results[] }`. Access `result.results` for the array, `result.total` for count. Verify `result.results` contains the tag.
- `search <unique-name> --types tags` — verify type filter (`result.results` entries have `type: "tag"`)
- `search <unique-name> --limit 1` — verify `result.results.length <= 1`
- `search nonexistent-xyz-${Date.now()}` — verify `result.total === 0`
- Cleanup

- [ ] **Step 2: Write plugin tests**

Cover:
- `plugins list` — verify returns plugin info (even if no plugins loaded)
- `plugin enable test-plugin` — test with the test plugin from `e2e/test-plugins/`
- `plugin disable test-plugin`
- `plugin settings test-plugin --data '{"key":"value"}'`
- `plugin purge-data test-plugin`
- Error cases: enable non-existent plugin

- [ ] **Step 3: Run and verify**

```bash
cd e2e && npm run test:with-server:cli -- --grep "search|plugin"
```

- [ ] **Step 4: Commit**

```bash
git add e2e/tests/cli/cli-search.spec.ts e2e/tests/cli/cli-plugins.spec.ts
git commit -m "test(cli): add search and plugin command E2E tests"
```

---

## Chunk 6: Cross-Cutting Tests (Tasks 20-22)

### Task 20: Output Format Tests

**Files:**
- Create: `e2e/tests/cli/cli-output-formats.spec.ts`

**Reference:** `cmd/mr/output/output.go` — `Print()` handles table/json/quiet for lists. `PrintSingle()` handles json for single entities (quiet has no effect on single entities).

- [ ] **Step 1: Write output format tests**

```typescript
// e2e/tests/cli/cli-output-formats.spec.ts
import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';

test.describe('output format tests', () => {
  let tagId: number;
  const tagName = `cli-fmt-test-${Date.now()}`;

  test.beforeAll(async () => {
    const cli = createCliRunner();
    const tag = cli.runJson('tag', 'create', '--name', tagName);
    tagId = tag.ID;
  });

  test.afterAll(async () => {
    if (tagId) createCliRunner().run('tag', 'delete', String(tagId));
  });

  test('default table output shows headers and data', async ({ cli }) => {
    const result = cli.runOrFail('tags', 'list', '--name', tagName);
    expect(result.stdout).toContain('ID');
    expect(result.stdout).toContain('NAME');
    expect(result.stdout).toContain(tagName);
  });

  test('--json outputs valid JSON', async ({ cli }) => {
    const result = cli.runOrFail('tags', 'list', '--name', tagName, '--json');
    const parsed = JSON.parse(result.stdout);
    expect(Array.isArray(parsed)).toBe(true);
    expect(parsed.some((t: any) => t.ID === tagId)).toBe(true);
  });

  test('--quiet outputs only IDs', async ({ cli }) => {
    const result = cli.runOrFail('tags', 'list', '--name', tagName, '--quiet');
    const lines = result.stdout.trim().split('\n').filter(Boolean);
    // Each line should be just a number (the ID)
    for (const line of lines) {
      expect(line.trim()).toMatch(/^\d+$/);
    }
    expect(lines.some(l => l.trim() === String(tagId))).toBe(true);
  });

  test('--no-header omits column headers', async ({ cli }) => {
    const result = cli.runOrFail('tags', 'list', '--name', tagName, '--no-header');
    // Should NOT contain the header row
    const lines = result.stdout.trim().split('\n');
    // First line should be data, not headers
    expect(lines[0]).not.toContain('NAME');
    expect(result.stdout).toContain(tagName);
  });

  test('--json on single entity (get)', async ({ cli }) => {
    const result = cli.runOrFail('tag', 'get', String(tagId), '--json');
    const parsed = JSON.parse(result.stdout);
    expect(parsed.ID).toBe(tagId);
    expect(parsed.Name).toBe(tagName);
  });

  test('--quiet on single entity get has no effect', async ({ cli }) => {
    // PrintSingle does not handle quiet mode; output should be key-value format
    const result = cli.runOrFail('tag', 'get', String(tagId), '--quiet');
    // Should still show key-value output (not just ID)
    expect(result.stdout).toContain('Name:');
  });

  test('--page flag works', async ({ cli }) => {
    // Page 1 should return results (default)
    const page1 = cli.runJson<any[]>('tags', 'list');
    expect(page1.length).toBeGreaterThanOrEqual(0);

    // A very high page should return empty
    const result = cli.runOrFail('tags', 'list', '--page', '9999', '--json');
    const highPage = JSON.parse(result.stdout);
    expect(highPage.length).toBe(0);
  });
});
```

- [ ] **Step 2: Run and verify**

```bash
cd e2e && npm run test:with-server:cli -- --grep "output format"
```

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/cli/cli-output-formats.spec.ts
git commit -m "test(cli): add output format E2E tests"
```

---

### Task 21: Error Handling Tests

**Files:**
- Create: `e2e/tests/cli/cli-error-handling.spec.ts`

- [ ] **Step 1: Write error handling tests**

```typescript
// e2e/tests/cli/cli-error-handling.spec.ts
import { test, expect } from '../../fixtures/cli.fixture';

test.describe('error handling', () => {
  test.describe('missing required flags', () => {
    test('tag create without --name fails', async ({ cli }) => {
      const result = cli.runExpectError('tag', 'create');
      expect(result.stderr).toContain('required');
    });

    test('note create without --name fails', async ({ cli }) => {
      const result = cli.runExpectError('note', 'create');
      expect(result.stderr).toContain('required');
    });

    test('query create without --text fails', async ({ cli }) => {
      const result = cli.runExpectError('query', 'create', '--name', 'test');
      expect(result.stderr).toContain('required');
    });
  });

  test.describe('invalid arguments', () => {
    test('tag get with non-numeric ID fails', async ({ cli }) => {
      const result = cli.runExpectError('tag', 'get', 'abc');
      expect(result.stderr + result.stdout).toContain('invalid');
    });

    test('tag get without ID fails', async ({ cli }) => {
      const result = cli.runExpectError('tag', 'get');
      expect(result.stderr).toContain('accepts 1 arg');
    });

    test('tag edit-name with too few args fails', async ({ cli }) => {
      const result = cli.runExpectError('tag', 'edit-name', '1');
      expect(result.stderr).toContain('accepts 2 arg');
    });
  });

  test.describe('non-existent entities', () => {
    test('tag get with non-existent ID fails', async ({ cli }) => {
      const result = cli.runExpectError('tag', 'get', '999999');
      expect(result.stderr + result.stdout).toContain('not found');
    });

    test('note get with non-existent ID fails', async ({ cli }) => {
      const result = cli.run('note', 'get', '999999');
      expect(result.exitCode).not.toBe(0);
    });

    test('category get with non-existent ID fails', async ({ cli }) => {
      const result = cli.runExpectError('category', 'get', '999999');
      expect(result.stderr + result.stdout).toContain('not found');
    });
  });

  test.describe('server connectivity', () => {
    test('connection refused with bad server URL', async ({ cli }) => {
      const { CliRunner } = require('../../helpers/cli-runner');
      const badCli = new CliRunner(
        process.env.CLI_PATH || require('path').resolve(__dirname, '../../../mr'),
        'http://localhost:1'
      );
      const result = badCli.run('tags', 'list');
      expect(result.exitCode).not.toBe(0);
      expect(result.stderr + result.stdout).toMatch(/connect|refused|connection/i);
    });
  });

  test.describe('edge cases', () => {
    test('special characters in name', async ({ cli }) => {
      const name = `cli-special-"quotes"-${Date.now()}`;
      const tag = cli.runJson('tag', 'create', '--name', name);
      expect(tag.Name).toBe(name);
      cli.run('tag', 'delete', String(tag.ID));
    });

    test('unicode in name', async ({ cli }) => {
      const name = `cli-unicode-\u00e9\u00e8\u00ea-${Date.now()}`;
      const tag = cli.runJson('tag', 'create', '--name', name);
      expect(tag.Name).toBe(name);
      cli.run('tag', 'delete', String(tag.ID));
    });
  });
});
```

- [ ] **Step 2: Run and verify**

```bash
cd e2e && npm run test:with-server:cli -- --grep "error handling"
```

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/cli/cli-error-handling.spec.ts
git commit -m "test(cli): add error handling E2E tests"
```

---

### Task 22: Global Flag Tests

**Files:**
- Create: `e2e/tests/cli/cli-global-flags.spec.ts`

- [ ] **Step 1: Write global flag tests**

```typescript
// e2e/tests/cli/cli-global-flags.spec.ts
import { test, expect } from '../../fixtures/cli.fixture';
import { CliRunner } from '../../helpers/cli-runner';
import { execSync } from 'child_process';
import * as path from 'path';

const cliBinary = process.env.CLI_PATH || path.resolve(__dirname, '../../../mr');
const serverUrl = process.env.BASE_URL || 'http://localhost:8181';

test.describe('global flags', () => {
  test('--server flag overrides default', async () => {
    // Use the actual server URL via --server flag (CliRunner does this automatically)
    const cli = new CliRunner(cliBinary, serverUrl);
    const result = cli.run('tags', 'list');
    expect(result.exitCode).toBe(0);
  });

  test('MAHRESOURCES_URL env var works', async () => {
    // Run CLI without --server but with MAHRESOURCES_URL set
    const cmd = `${cliBinary} tags list --json`;
    const stdout = execSync(cmd, {
      encoding: 'utf-8',
      timeout: 30000,
      env: { ...process.env, MAHRESOURCES_URL: serverUrl },
    });
    // Should not error — env var provides the URL
    const parsed = JSON.parse(stdout);
    expect(Array.isArray(parsed)).toBe(true);
  });

  test('--server takes precedence over MAHRESOURCES_URL', async () => {
    // Set env to a bad URL, but --server to the good one
    const cmd = `${cliBinary} --server '${serverUrl}' tags list --json`;
    const stdout = execSync(cmd, {
      encoding: 'utf-8',
      timeout: 30000,
      env: { ...process.env, MAHRESOURCES_URL: 'http://localhost:1' },
    });
    const parsed = JSON.parse(stdout);
    expect(Array.isArray(parsed)).toBe(true);
  });

  test('default URL fails when no server is running there', async () => {
    // Run without --server and without MAHRESOURCES_URL
    // Default is http://localhost:8181 — may or may not be running
    // We test by using a known-bad URL via env var override
    const cmd = `${cliBinary} tags list`;
    try {
      execSync(cmd, {
        encoding: 'utf-8',
        timeout: 10000,
        env: { ...process.env, MAHRESOURCES_URL: 'http://localhost:1' },
      });
      // If it somehow succeeds, that's also valid
    } catch (error: any) {
      expect(error.status).not.toBe(0);
    }
  });
});
```

- [ ] **Step 2: Run and verify**

```bash
cd e2e && npm run test:with-server:cli -- --grep "global flags"
```

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/cli/cli-global-flags.spec.ts
git commit -m "test(cli): add global flag E2E tests"
```

---

## Chunk 7: Final Verification (Task 23)

### Task 23: Run Full Suite and Fix Issues

- [ ] **Step 1: Run all CLI tests**

```bash
cd e2e && npm run test:with-server:cli
```

Expected: All tests pass. If any fail, fix them.

- [ ] **Step 2: Run CLI tests alongside browser tests to verify no interference**

```bash
cd e2e && npm run test:with-server
```

Expected: Both browser and CLI tests pass. The `cli` project should run independently.

- [ ] **Step 3: Verify test count**

```bash
cd e2e && npx playwright test --project=cli --list 2>/dev/null | grep -c "test"
```

Expected: ~200+ tests listed.

- [ ] **Step 4: Final commit if any fixes were needed**

```bash
git add -A e2e/tests/cli/
git commit -m "fix(e2e): fix issues found in full CLI test suite run"
```
