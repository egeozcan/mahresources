# Cluster 15 — Schema-editor + block-editor polish (BH-010, BH-021)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Two independent bugs in two different files — parallel subagents safe. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Stop the schema-editor Preview Form tab from seeding numeric fields with `0` (BH-010), and expand the block-editor's `renderMarkdown` to recognize `_italic_`, `` `code` ``, and `~~strike~~` (BH-021).

**Architecture:**

- **Group A (BH-010):** `src/components/schemaEditorModal.ts::getPreviewValue` currently calls `getDefaultValue(schema, schema)` from `schema-core.ts`, which returns `0` for numeric types with no explicit `default`. Add a new `getPreviewDefaultValue(schema, rootSchema)` in `schema-core.ts` that returns `undefined` (so the input renders empty) when no `default` is declared for numbers/integers/strings. `getPreviewValue` switches to the new function. Defensive fallback in `_renderNumberInput` (`src/schema-editor/modes/form-mode.ts`): if `data === 0 && !('default' in schema)`, render the input as empty.
- **Group B (BH-021):** Extend `renderMarkdown` in `src/components/blockEditor.js` with three additional token pairs: `_italic_` → `<em>`, `` `code` `` → `<code>`, `~~strike~~` → `<s>`. Preserve the existing `**bold**` / `*italic*` / `[link](url)` support. No headings/lists — the block editor has dedicated heading blocks and the scope is tight token parity only.

**Tech Stack:** TypeScript (schema-editor LitElement components), plain ES modules (blockEditor Alpine component), existing schema-editor unit tests (`*.test.ts`), Playwright E2E.

**Worktree branch:** `bugfix/c15-schema-block-editor`

**Top-level spec:** `docs/superpowers/specs/2026-04-22-bughunt-batch-c9-c18-design.md` — Cluster 15.

---

## File structure

**Modified:**
- `src/schema-editor/schema-core.ts` — add `getPreviewDefaultValue`
- `src/components/schemaEditorModal.ts` — switch `getPreviewValue` to use the new function
- `src/schema-editor/modes/form-mode.ts:1016-1080` — defensive fallback in `_renderNumberInput`
- `src/components/blockEditor.js:30-56` — expand `renderMarkdown`

**Created:**
- `src/schema-editor/bh010-preview-zero.test.ts` — unit test for getPreviewValue with numeric no-default
- `src/components/blockEditor-render-markdown.test.ts` — unit test for the three new tokens (use the same test harness as `src/schema-editor/*.test.ts`)
- `e2e/tests/c15-bh010-preview-form-numeric.spec.ts`
- `e2e/tests/c15-bh021-markdown-tokens.spec.ts`

---

## Task 0: Worktree + baseline

- [ ] **Step 1: Worktree**

```bash
cd /Users/egecan/Code/mahresources
git worktree add -b bugfix/c15-schema-block-editor ../mahresources-c15 master
cd ../mahresources-c15
```

- [ ] **Step 2: Baseline tests**

```bash
go test --tags 'json1 fts5' ./... -count=1
# Schema-editor has a JS test runner — check package.json for "test" script
npm test 2>&1 | tail -30
```

If the TS tests were absent or flaky on master, note the baseline and proceed. c15 does not break any new tests.

---

## Task Group A: BH-010 — Preview Form numeric seeding

### Task A1: Write failing unit test for `getPreviewDefaultValue`

**Files:**
- Create: `src/schema-editor/bh010-preview-zero.test.ts`

- [ ] **Step 1: Write the failing test**

```typescript
import { describe, it, expect } from 'vitest'; // or the repo's test runner (check existing test files' imports)
import { getPreviewDefaultValue } from './schema-core';

describe('BH-010: getPreviewDefaultValue', () => {
  it('returns undefined for numeric field with no default', () => {
    expect(getPreviewDefaultValue({ type: 'number' })).toBeUndefined();
    expect(getPreviewDefaultValue({ type: 'integer' })).toBeUndefined();
  });

  it('honors explicit numeric default', () => {
    expect(getPreviewDefaultValue({ type: 'number', default: 42 })).toBe(42);
    expect(getPreviewDefaultValue({ type: 'integer', default: 7 })).toBe(7);
  });

  it('still returns false for boolean and empty string for string with no default', () => {
    // Preview semantics: empty state should be obvious, not zero-like
    expect(getPreviewDefaultValue({ type: 'boolean' })).toBe(false);
    expect(getPreviewDefaultValue({ type: 'string' })).toBe(undefined);
  });

  it('for object schemas, recurses with preview semantics (numeric props stay undefined)', () => {
    const schema = {
      type: 'object',
      properties: {
        year: { type: 'integer', minimum: 1900, maximum: 2100 },
        title: { type: 'string' },
        active: { type: 'boolean' },
      },
    };
    const got = getPreviewDefaultValue(schema);
    expect(got.year).toBeUndefined();
    expect(got.title).toBeUndefined();
    expect(got.active).toBe(false);
  });
});
```

- [ ] **Step 2: Run 3× to verify fails**

```bash
npm test -- bh010-preview-zero --run 2>&1 | tail -30
# OR if the test runner is different:
npx vitest run src/schema-editor/bh010-preview-zero.test.ts
```

Expected: FAIL — `getPreviewDefaultValue` is not exported.

### Task A2: Add `getPreviewDefaultValue` to `schema-core.ts`

**Files:**
- Modify: `src/schema-editor/schema-core.ts`

- [ ] **Step 1: Add the new function after `getDefaultValue`**

```typescript
// ─── Preview-specific defaults ──────────────────────────────────────────────

/**
 * BH-010: getPreviewDefaultValue mirrors getDefaultValue but returns `undefined`
 * for number/integer/string when no explicit `default` is declared. This is
 * the right semantics for the Preview Form tab — the user should see an empty
 * input, not `0` or an empty string that then triggers a range-error onBlur.
 *
 * Booleans still default to `false` (they can't be "empty"). Objects still
 * recurse so nested numeric props flow the preview semantics. Arrays default
 * to [] (no items to seed). enum/const still return their explicit value.
 */
export function getPreviewDefaultValue(schema: JSONSchema, rootSchema?: JSONSchema): any {
  if (schema.$ref) {
    const resolved = resolveRef(schema.$ref, rootSchema || schema);
    if (resolved) {
      const siblings: JSONSchema = { ...schema };
      delete siblings.$ref;
      return getPreviewDefaultValue(mergeSchemas(resolved, siblings), rootSchema);
    }
  }

  if (schema.allOf && Array.isArray(schema.allOf)) {
    let merged: JSONSchema = { ...schema };
    delete merged.allOf;
    for (const sub of schema.allOf) {
      let resolved: JSONSchema;
      if (sub.$ref) {
        const refResult = resolveRef(sub.$ref, rootSchema || schema);
        const siblings: JSONSchema = { ...sub };
        delete siblings.$ref;
        resolved = refResult ? mergeSchemas(refResult, siblings) : siblings;
      } else {
        resolved = sub;
      }
      if (resolved) merged = mergeSchemas(merged, resolved);
    }
    return getPreviewDefaultValue(merged, rootSchema);
  }

  if (schema.default !== undefined) return schema.default;
  if (schema.const !== undefined) return schema.const;
  if (schema.enum && Array.isArray(schema.enum) && schema.enum.length > 0) {
    return schema.enum[0];
  }

  if (schema.type === 'object') {
    if (!schema.properties) return {};
    const obj: any = {};
    for (const [key, propSchema] of Object.entries(schema.properties)) {
      obj[key] = getPreviewDefaultValue(propSchema as JSONSchema, rootSchema);
    }
    return obj;
  }
  if (schema.type === 'array') return [];
  if (schema.type === 'boolean') return false;
  if (schema.type === 'number' || schema.type === 'integer') return undefined;
  if (schema.type === 'string') return undefined;
  if (schema.type === 'null') return null;

  if (Array.isArray(schema.type)) {
    // Preview: prefer the least-surprising empty state.
    if (schema.type.includes('null')) return null;
    if (schema.type.includes('boolean')) return false;
    if (schema.type.includes('object')) return {};
    if (schema.type.includes('array')) return [];
    // string/number/integer → undefined
    return undefined;
  }

  if (schema.oneOf && schema.oneOf.length > 0) {
    if (isLabeledEnum(schema)) return schema.oneOf[0].const;
    return getPreviewDefaultValue(schema.oneOf[0], rootSchema);
  }
  if (schema.anyOf && schema.anyOf.length > 0) return getPreviewDefaultValue(schema.anyOf[0], rootSchema);

  return undefined;
}
```

- [ ] **Step 2: Run 3× to verify pass**

```bash
npx vitest run src/schema-editor/bh010-preview-zero.test.ts
```

Expected: PASS × 3.

### Task A3: Switch `getPreviewValue` to use the new function

**Files:**
- Modify: `src/components/schemaEditorModal.ts:20-28`

- [ ] **Step 1: Change the import + call**

Find:

```typescript
import { getDefaultValue } from '../schema-editor/schema-core';
// ...
export function getPreviewValue(schemaStr: string): string {
  try {
    const schema = JSON.parse(schemaStr);
    const defaultVal = getDefaultValue(schema, schema);
    return JSON.stringify(defaultVal);
  } catch {
    return JSON.stringify({});
  }
}
```

Change to:

```typescript
import { getPreviewDefaultValue } from '../schema-editor/schema-core';
// ...
export function getPreviewValue(schemaStr: string): string {
  try {
    const schema = JSON.parse(schemaStr);
    const defaultVal = getPreviewDefaultValue(schema, schema);
    return JSON.stringify(defaultVal);
  } catch {
    return JSON.stringify({});
  }
}
```

Note: `JSON.stringify(undefined)` returns `undefined` (not `"undefined"`) — the previewed numeric field receives an undefined property value, which the form-mode's `_renderNumberInput` already treats as empty (line 1043: `} else if (val === '' || val === undefined || val === null) { ... }`).

### Task A4: Defensive fallback in `_renderNumberInput`

**Files:**
- Modify: `src/schema-editor/modes/form-mode.ts:1016-1040` area

- [ ] **Step 1: Adjust the initial value rendering**

Find the `.value=` binding inside `_renderNumberInput` (search for `.value=\${data`):

```typescript
.value=${data !== undefined && data !== null ? String(data) : ''}
```

Change to:

```typescript
.value=${formatNumericForInput(data, schema)}
```

Add a small helper near the top of the file (below imports):

```typescript
// BH-010: treat `data === 0` as "empty" when the schema has no explicit
// `default` — otherwise the preview harness shows `0` and the onBlur
// validator fires a bogus "must be at least N" error for the user.
function formatNumericForInput(data: any, schema: JSONSchema): string {
  if (data === undefined || data === null) return '';
  if (data === 0 && !('default' in schema)) return '';
  return String(data);
}
```

### Task A5: Write failing E2E for Preview Form

**Files:**
- Create: `e2e/tests/c15-bh010-preview-form-numeric.spec.ts`

- [ ] **Step 1: Write the failing test**

```typescript
/**
 * BH-010: Schema-editor Preview Form seeds numeric fields with 0 instead
 * of leaving them empty. This makes the onBlur validator fire "Must be at
 * least 1900" even though the user typed nothing.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('BH-010: preview form numeric fields', () => {
  test('numeric field with min/max constraint renders empty in Preview tab', async ({ page, apiClient }) => {
    const nt = await apiClient.createNoteType({
      name: `BH010-${Date.now()}`,
      metaSchema: JSON.stringify({
        type: 'object',
        properties: { year: { type: 'integer', minimum: 1900, maximum: 2100 } },
      }),
    });

    await page.goto(`/noteType?id=${nt.ID}`);
    await page.getByRole('button', { name: /visual.*editor/i }).click();
    await page.getByRole('tab', { name: /preview form/i }).click();

    // Input should render empty, NOT "0"
    const yearInput = page.locator('#field-year');
    await expect(yearInput).toHaveValue('');

    // Blurring the empty field should not surface a range error
    await yearInput.focus();
    await yearInput.blur();
    const errorSpan = page.locator('#field-year-error');
    const errorText = (await errorSpan.textContent())?.trim() ?? '';
    expect(errorText).not.toMatch(/at least|at most|1900|2100/);
  });
});
```

- [ ] **Step 2: Run 3× to verify fails**

```bash
cd e2e && npx playwright test c15-bh010-preview-form-numeric --reporter=line --repeat-each=3
```

Expected: FAIL with input value `0` or range error visible.

### Task A6: Build + run + commit

```bash
npm run build
cd e2e && npx playwright test c15-bh010-preview-form-numeric --reporter=line
```

Expected: PASS.

```bash
git add src/schema-editor/schema-core.ts src/schema-editor/bh010-preview-zero.test.ts \
  src/components/schemaEditorModal.ts \
  src/schema-editor/modes/form-mode.ts \
  e2e/tests/c15-bh010-preview-form-numeric.spec.ts \
  public/dist/ public/tailwind.css
git commit -m "fix(schema-editor): BH-010 — Preview Form no longer seeds numeric fields with 0

getPreviewValue now calls new getPreviewDefaultValue(), which returns
undefined for number/integer/string when no explicit default is declared.
Previously getDefaultValue returned 0 for numerics, making the preview
fire a bogus 'Must be at least N' onBlur for empty fields.

Defensive fallback in _renderNumberInput: data===0 && !('default' in
schema) renders the input empty, protecting against any code path that
slips a 0 through.

Unit: src/schema-editor/bh010-preview-zero.test.ts.
E2E: e2e/tests/c15-bh010-preview-form-numeric.spec.ts."
```

---

## Task Group B: BH-021 — Extra markdown tokens

### Task B1: Write failing unit test for new tokens

**Files:**
- Create: `src/components/blockEditor-render-markdown.test.ts`

- [ ] **Step 1: Write the failing test**

```typescript
/**
 * BH-021: blockEditor's renderMarkdown only recognized **bold**, *italic*,
 * and [link](url). Users expect _italic_, `code`, ~~strike~~ too —
 * common GFM tokens.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'fs';
import { resolve } from 'path';

// The blockEditor module is Alpine data — exported as a factory. Parse the
// source and extract renderMarkdown for unit testing without spinning up
// Alpine. (Existing schema-editor tests use this pattern.)
const src = readFileSync(resolve(__dirname, './blockEditor.js'), 'utf-8');
const match = src.match(/renderMarkdown\(text\)\s*\{[\s\S]*?\n\s{4}\},/);
if (!match) throw new Error('could not extract renderMarkdown from blockEditor.js');
const renderMarkdown = new Function(`${match[0].replace('renderMarkdown(text)', 'return function renderMarkdown(text)')}`)() as (t: string) => string;
// Fallback: if the dynamic extraction is brittle, import the built bundle
// via a headless test page (see playwright equivalent below).

describe('BH-021: renderMarkdown extended tokens', () => {
  it('renders `_italic_` as <em>', () => {
    expect(renderMarkdown('hello _world_')).toMatch(/hello <em>world<\/em>/);
  });

  it('renders `` `code` `` as <code>', () => {
    expect(renderMarkdown('call `foo()` please')).toMatch(/call <code>foo\(\)<\/code> please/);
  });

  it('renders `~~strike~~` as <s>', () => {
    expect(renderMarkdown('~~gone~~')).toMatch(/<s>gone<\/s>/);
  });

  it('preserves existing **bold** behavior', () => {
    expect(renderMarkdown('**hi**')).toMatch(/<strong>hi<\/strong>/);
  });

  it('preserves existing *italic* behavior', () => {
    expect(renderMarkdown('*hi*')).toMatch(/<em>hi<\/em>/);
  });

  it('renders [text](url) as anchor', () => {
    expect(renderMarkdown('[mahr](https://example.com)')).toMatch(/<a href="https:\/\/example\.com"/);
  });

  it('escapes HTML in user text', () => {
    expect(renderMarkdown('<script>alert(1)</script>')).not.toMatch(/<script>/);
  });
});
```

If the dynamic function-extraction approach fails, simplify: put an E2E test instead using `page.evaluate` to call the Alpine component's method, matching the pattern in `e2e/tests/c13-bh002-json-table-null.spec.ts`.

- [ ] **Step 2: Run 3× to verify fails**

```bash
npx vitest run src/components/blockEditor-render-markdown.test.ts
```

Expected: FAIL — the three new-token tests fail; the preserve-existing tests pass.

### Task B2: Expand `renderMarkdown` with the three tokens

**Files:**
- Modify: `src/components/blockEditor.js:30-56`

- [ ] **Step 1: Add the three regex passes**

Find the existing function (line 31-56) and extend:

```javascript
    // Simple markdown-like rendering: escapes HTML, converts newlines to <br>, and handles basic formatting
    renderMarkdown(text) {
      if (!text) return '';
      // Escape HTML entities
      let escaped = text
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#039;');
      // Convert newlines to <br>
      escaped = escaped.replace(/\n/g, '<br>');

      // Inline code: `text` -> <code>text</code>
      // Run BEFORE other inline tokens so we don't double-format inside code.
      // BH-021: common GFM token users expect.
      escaped = escaped.replace(/`([^`]+)`/g, '<code>$1</code>');

      // Basic bold: **text** -> <strong>text</strong>
      escaped = escaped.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
      // Basic italic (asterisk form): *text* -> <em>text</em>
      escaped = escaped.replace(/\*([^*]+)\*/g, '<em>$1</em>');
      // Italic (underscore form): _text_ -> <em>text</em>
      // BH-021: matches GFM. Boundaries: underscore must not be preceded/followed
      // by an alphanumeric to avoid breaking snake_case_identifiers.
      escaped = escaped.replace(/(^|[^A-Za-z0-9_])_([^_]+)_(?=$|[^A-Za-z0-9_])/g, '$1<em>$2</em>');

      // Strikethrough: ~~text~~ -> <s>text</s>
      // BH-021: common GFM token.
      escaped = escaped.replace(/~~([^~]+)~~/g, '<s>$1</s>');

      // Basic links: [text](url) -> <a href="url">text</a>
      escaped = escaped.replace(/\[([^\]]+)\]\(([^)]+)\)/g, (match, text, href) => {
          const trimmed = href.trim().toLowerCase();
          if (trimmed.startsWith('javascript:') || trimmed.startsWith('data:') || trimmed.startsWith('vbscript:')) {
              return text;
          }
          return `<a href="${href}" class="text-blue-600 hover:underline" target="_blank" rel="noopener">${text}</a>`;
      });
      return escaped;
    },
```

Key ordering decisions:
1. Backtick code BEFORE bold/italic/strike so inline-code content is immune to further transformation (standard GFM-ish behavior).
2. `_italic_` AFTER `*italic*` because the regex anchors on non-alphanumeric — doesn't conflict with asterisk form.
3. Boundary in `_italic_` regex: prevents matching `some_snake_case` variable names (common in block-editor text discussing code).

### Task B3: Run unit test to verify pass

```bash
npx vitest run src/components/blockEditor-render-markdown.test.ts
```

Expected: all 7 tests PASS.

### Task B4: Write failing E2E (belt-and-braces)

**Files:**
- Create: `e2e/tests/c15-bh021-markdown-tokens.spec.ts`

- [ ] **Step 1: Write the failing test**

```typescript
/**
 * BH-021: confirm the expanded markdown tokens render correctly in an
 * actual block-editor text block.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('BH-021: markdown tokens', () => {
  test('text block renders _italic_, `code`, ~~strike~~', async ({ page, apiClient }) => {
    const note = await apiClient.createNote({ name: `BH021-${Date.now()}` });
    await page.goto(`/note?id=${note.ID}`);

    // Add a text block
    await page.getByTestId('add-block-button').click();
    await page.getByRole('option', { name: /text/i }).click();
    await page.locator('[data-block-type="text"] textarea').first().fill('hello _world_ and `code` and ~~strike~~');
    await page.keyboard.press('Tab'); // blur → save

    // Reload and verify the rendered HTML
    await page.reload();
    const rendered = page.locator('[data-block-type="text"] .prose, [data-block-type="text"]').first();
    const html = await rendered.innerHTML();
    expect(html).toMatch(/<em>world<\/em>/);
    expect(html).toMatch(/<code>code<\/code>/);
    expect(html).toMatch(/<s>strike<\/s>/);
  });
});
```

Note: selectors may need adjusting — check existing block-editor E2E specs (`e2e/tests/36-block-text-saves-content.spec.ts` etc.) for current conventions. Use the closest-matching pattern.

- [ ] **Step 2: Build + run**

```bash
npm run build
cd e2e && npx playwright test c15-bh021-markdown-tokens --reporter=line
```

Expected: PASS.

### Task B5: Commit

```bash
git add src/components/blockEditor.js src/components/blockEditor-render-markdown.test.ts \
  e2e/tests/c15-bh021-markdown-tokens.spec.ts \
  public/dist/ public/tailwind.css
git commit -m "feat(block-editor): BH-021 — renderMarkdown recognises _italic_, \`code\`, ~~strike~~

Previously only **bold**, *italic*, and [link](url) worked. Users typed
GFM-standard tokens and saw them render literally. Add three token passes
in renderMarkdown (src/components/blockEditor.js):

- \`code\` → <code> (evaluated first so content is immune to later passes)
- _italic_ → <em> (with word-boundary anchors so snake_case_names survive)
- ~~strike~~ → <s>

Unit: src/components/blockEditor-render-markdown.test.ts.
E2E: e2e/tests/c15-bh021-markdown-tokens.spec.ts."
```

---

## Task C: Update `tasks/bug-hunt-log.md`

Mark BH-010 and BH-021 FIXED. Append to Fixed/closed table.

---

## Task D: Full test matrix + PR + merge + log backfill + cleanup

Standard pattern. PR title: `fix(bughunt c15): BH-010/021 schema-editor + block-editor polish`.

---

## Self-review checklist

- [ ] `getPreviewDefaultValue` exported from `schema-core.ts`, unit-tested
- [ ] `_renderNumberInput` handles `data === 0 && !('default' in schema)` → empty
- [ ] `renderMarkdown` covers _italic_, `code`, ~~strike~~ plus existing tokens
- [ ] snake_case_identifiers not mangled by `_italic_` regex (word-boundary test)
- [ ] Inline code protects its contents from other inline passes
- [ ] HTML still escaped for safety (existing behavior)
- [ ] Existing schema-editor tests (`bugfix-batch*`) still pass
