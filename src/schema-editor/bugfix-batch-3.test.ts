/**
 * Tests for 3 confirmed bugs — written RED-first before any fixes.
 *
 * Bug 1 (P1): Stripped keys rehydrated from currentMeta after schema switch
 * Bug 2 (P1): `not` serialization loses extra variants and removal drops constraint
 * Bug 3 (P2): $ref/composition/conditional nodes lose rename and metadata controls
 */
import { readFileSync } from 'fs';
import { resolve } from 'path';
import { describe, it, expect, beforeEach } from 'vitest';
import { schemaToTree, treeToSchema, resetIdCounter, type SchemaNode } from './schema-tree-model';
import { stripStaleKeys } from './form-mode-helpers';

// ─── Bug 1 (P1): form-mode emits value-change after stripping stale keys ─────

describe('Bug 1 (P1): form-mode emits value-change after stripping stale keys', () => {
  /**
   * We cannot instantiate the full Lit element in a Node-only vitest environment.
   * Instead we verify the source code: after stripStaleKeys runs in willUpdate,
   * the code must dispatch a value-change event so the Alpine wrapper's
   * currentMeta stays in sync with the stripped data.
   */
  it('form-mode.ts dispatches value-change after stripStaleKeys when keys are stripped', () => {
    const src = readFileSync(
      resolve(__dirname, 'modes/form-mode.ts'),
      'utf-8',
    );

    // The willUpdate method should contain stripStaleKeys
    expect(src).toContain('stripStaleKeys');

    // After stripping, there should be a value-change dispatch.
    // Find the willUpdate method and check that after stripStaleKeys there is
    // a dispatchEvent call with 'value-change'.
    const willUpdateMatch = src.match(/willUpdate[\s\S]*?^\s{2}\}/m);
    expect(willUpdateMatch).not.toBeNull();
    const willUpdateBody = willUpdateMatch![0];

    // The stripStaleKeys call must exist
    expect(willUpdateBody).toContain('stripStaleKeys');

    // After stripStaleKeys, there must be a value-change dispatch mechanism.
    // This could be via dispatchEvent or updateComplete.then
    const afterStrip = willUpdateBody.slice(willUpdateBody.indexOf('stripStaleKeys'));
    expect(afterStrip).toMatch(/value-change/);
  });

  it('stripStaleKeys returns indication of whether keys were actually stripped', () => {
    // When stripping changes data, form-mode needs to know so it can emit.
    // Test that the before/after JSON comparison approach works:
    const data = { color: 'red', weight: 42 };
    const schema = {
      properties: { weight: { type: 'number' } },
      additionalProperties: false as const,
    };

    const before = JSON.stringify(data);
    stripStaleKeys(data, schema);
    const after = JSON.stringify(data);

    // Keys were stripped, so before !== after
    expect(before).not.toBe(after);
    expect(data).toEqual({ weight: 42 });
  });

  it('JSON comparison detects no change when no keys need stripping', () => {
    const data = { weight: 42 };
    const schema = {
      properties: { weight: { type: 'number' } },
      additionalProperties: false as const,
    };

    const before = JSON.stringify(data);
    stripStaleKeys(data, schema);
    const after = JSON.stringify(data);

    // No keys stripped, before === after
    expect(before).toBe(after);
  });
});

// ─── Bug 2 (P1): `not` composition — single variant, no empty ────────────────

describe('Bug 2 (P1): not composition must be single-variant', () => {
  it('composition-editor.ts hides Add Variant button for not keyword', () => {
    const src = readFileSync(
      resolve(__dirname, 'tree/node-editors/composition-editor.ts'),
      'utf-8',
    );

    // The source must check for `not` keyword and conditionally hide
    // the Add Variant button. Look for the 'not' keyword check.
    expect(src).toContain("'not'");

    // The render method should conditionally hide the "Add Variant" button
    // when keyword is 'not'
    const renderMatch = src.match(/render\(\)[\s\S]*$/);
    expect(renderMatch).not.toBeNull();
    const renderBody = renderMatch![0];

    // There should be a conditional that prevents rendering Add Variant for `not`
    // This could be via a check like `this.keyword !== 'not'` or `isNot`
    expect(renderBody).toMatch(/not.*Add Variant|Add Variant.*not|isNot/);
  });

  it('composition-editor.ts hides Remove button for not keyword variants', () => {
    const src = readFileSync(
      resolve(__dirname, 'tree/node-editors/composition-editor.ts'),
      'utf-8',
    );

    // The render method should conditionally hide the remove (x) button
    // when keyword is 'not'
    const renderMatch = src.match(/render\(\)[\s\S]*$/);
    expect(renderMatch).not.toBeNull();
    const renderBody = renderMatch![0];

    // There should be a conditional that prevents removing the sole not variant
    expect(renderBody).toMatch(/not.*Remove|remove.*not|isNot.*btn-danger|btn-danger.*isNot/i);
  });

  it('treeToSchema serializes not even when variant name is not "not"', () => {
    // The current code uses .find(c => c.name === 'not') which fails
    // if the variant was parsed from a schema where the child had a title.
    resetIdCounter();
    const schema = {
      not: { type: 'string', title: 'forbidden' },
    };
    const tree = schemaToTree(schema);

    // The tree should have compositionKeyword = 'not'
    expect(tree.compositionKeyword).toBe('not');
    expect(tree.variants).toHaveLength(1);

    // Round-trip: treeToSchema must produce the not keyword
    const output = treeToSchema(tree);
    expect(output.not).toBeDefined();
    expect(output.not.type).toBe('string');
  });

  it('treeToSchema omits not when variants is empty (constraint removed)', () => {
    resetIdCounter();
    const node: SchemaNode = {
      id: 'test-not-empty',
      name: '',
      type: '',
      required: false,
      schema: {},
      compositionKeyword: 'not',
      variants: [],
    };

    const output = treeToSchema(node);
    // With empty variants, `not` should not appear in output
    expect(output).not.toHaveProperty('not');
  });
});

// ─── Bug 3 (P2): detail-panel shows rename/metadata for special nodes ─────────

describe('Bug 3 (P2): detail-panel shows common fields for $ref/composition/conditional nodes', () => {
  it('detail-panel.ts has a _renderCommonFields method or equivalent', () => {
    const src = readFileSync(
      resolve(__dirname, 'tree/detail-panel.ts'),
      'utf-8',
    );

    // The detail panel must have a reusable method for common fields
    // (name, title, description, required) that is called in the special-case
    // render paths (ref, composition, conditional).
    expect(src).toMatch(/_renderCommonFields|_renderHeader/);
  });

  it('$ref render path includes property name input', () => {
    const src = readFileSync(
      resolve(__dirname, 'tree/detail-panel.ts'),
      'utf-8',
    );

    // Find the $ref early-return block
    const refBlock = extractRenderBlock(src, 'node.ref');
    expect(refBlock).not.toBeNull();

    // It should include either _renderCommonFields or a prop-name input
    expect(refBlock).toMatch(/prop-name|_renderCommonFields|_renderHeader/);
  });

  it('composition render path includes property name input', () => {
    const src = readFileSync(
      resolve(__dirname, 'tree/detail-panel.ts'),
      'utf-8',
    );

    // Find the composition early-return block
    const compBlock = extractRenderBlock(src, 'node.compositionKeyword');
    expect(compBlock).not.toBeNull();

    // It should include either _renderCommonFields or a prop-name input
    expect(compBlock).toMatch(/prop-name|_renderCommonFields|_renderHeader/);
  });

  it('conditional render path includes property name input', () => {
    const src = readFileSync(
      resolve(__dirname, 'tree/detail-panel.ts'),
      'utf-8',
    );

    // Find the conditional early-return block
    const condBlock = extractRenderBlock(src, 'schema.if');
    expect(condBlock).not.toBeNull();

    // It should include either _renderCommonFields or a prop-name input
    expect(condBlock).toMatch(/prop-name|_renderCommonFields|_renderHeader/);
  });

  it('$ref render path includes required checkbox', () => {
    const src = readFileSync(
      resolve(__dirname, 'tree/detail-panel.ts'),
      'utf-8',
    );

    const refBlock = extractRenderBlock(src, 'node.ref');
    expect(refBlock).not.toBeNull();

    // Must include required checkbox or call _renderCommonFields which has it
    expect(refBlock).toMatch(/required|_renderCommonFields|_renderHeader/);
  });

  it('composition render path includes title and description inputs', () => {
    const src = readFileSync(
      resolve(__dirname, 'tree/detail-panel.ts'),
      'utf-8',
    );

    const compBlock = extractRenderBlock(src, 'node.compositionKeyword');
    expect(compBlock).not.toBeNull();

    // Must include title/description inputs or call _renderCommonFields
    expect(compBlock).toMatch(/prop-title|prop-desc|_renderCommonFields|_renderHeader/);
  });
});

// ─── Helper ───────────────────────────────────────────────────────────────────

/**
 * Extract the render block starting at a conditional check (e.g. 'node.ref')
 * up to the next early return's closing `}` or the end of the render method.
 */
function extractRenderBlock(src: string, condition: string): string | null {
  // Find the if-block that checks for the condition
  const idx = src.indexOf(condition);
  if (idx === -1) return null;

  // Find the return html` ... ` block after it
  const afterCondition = src.slice(idx, idx + 1500);
  const returnMatch = afterCondition.match(/return html`[\s\S]*?`;/);
  if (!returnMatch) return afterCondition;
  return returnMatch[0];
}
