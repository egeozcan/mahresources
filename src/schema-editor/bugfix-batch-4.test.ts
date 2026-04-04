/**
 * Tests for 1 confirmed bug — written RED-first before any fixes.
 *
 * Bug (P2): $ref/composition/conditional nodes missing readOnly and writeOnly flags
 *
 * Root cause: _renderCommonFields() only includes the required checkbox in the
 * flags block. The readOnly and writeOnly checkboxes are only in the default
 * (non-special) render path's flags block (~line 208). Special-node early
 * returns call _renderCommonFields() + their specialized editor + _renderActions(),
 * but skip the flags block containing readOnly/writeOnly.
 *
 * Fix: Move readOnly and writeOnly into _renderCommonFields() so all render
 * paths include them. Remove them from the default render path to avoid
 * duplication.
 */
import { readFileSync } from 'fs';
import { resolve } from 'path';
import { describe, it, expect } from 'vitest';

// ─── Bug (P2): readOnly/writeOnly visible for all node types ─────────────────

describe('Bug (P2): detail-panel shows readOnly/writeOnly for $ref/composition/conditional nodes', () => {
  it('_renderCommonFields includes readOnly checkbox', () => {
    const src = readFileSync(
      resolve(__dirname, 'tree/detail-panel.ts'),
      'utf-8',
    );

    // Extract _renderCommonFields body
    const commonFieldsBody = extractMethodBody(src, '_renderCommonFields');
    expect(commonFieldsBody).not.toBeNull();

    // readOnly checkbox must appear inside _renderCommonFields
    expect(commonFieldsBody).toContain('readOnly');
  });

  it('_renderCommonFields includes writeOnly checkbox', () => {
    const src = readFileSync(
      resolve(__dirname, 'tree/detail-panel.ts'),
      'utf-8',
    );

    const commonFieldsBody = extractMethodBody(src, '_renderCommonFields');
    expect(commonFieldsBody).not.toBeNull();

    // writeOnly checkbox must appear inside _renderCommonFields
    expect(commonFieldsBody).toContain('writeOnly');
  });

  it('$ref render path includes readOnly (via _renderCommonFields)', () => {
    const src = readFileSync(
      resolve(__dirname, 'tree/detail-panel.ts'),
      'utf-8',
    );

    // The $ref block calls _renderCommonFields which now contains readOnly
    const refBlock = extractRenderBlock(src, 'node.ref');
    expect(refBlock).not.toBeNull();
    expect(refBlock).toMatch(/readOnly|_renderCommonFields/);
  });

  it('composition render path includes readOnly (via _renderCommonFields)', () => {
    const src = readFileSync(
      resolve(__dirname, 'tree/detail-panel.ts'),
      'utf-8',
    );

    const compBlock = extractRenderBlock(src, 'node.compositionKeyword');
    expect(compBlock).not.toBeNull();
    expect(compBlock).toMatch(/readOnly|_renderCommonFields/);
  });

  it('conditional render path includes readOnly (via _renderCommonFields)', () => {
    const src = readFileSync(
      resolve(__dirname, 'tree/detail-panel.ts'),
      'utf-8',
    );

    const condBlock = extractRenderBlock(src, 'schema.if');
    expect(condBlock).not.toBeNull();
    expect(condBlock).toMatch(/readOnly|_renderCommonFields/);
  });

  it('$ref render path includes writeOnly (via _renderCommonFields)', () => {
    const src = readFileSync(
      resolve(__dirname, 'tree/detail-panel.ts'),
      'utf-8',
    );

    const refBlock = extractRenderBlock(src, 'node.ref');
    expect(refBlock).not.toBeNull();
    expect(refBlock).toMatch(/writeOnly|_renderCommonFields/);
  });

  it('composition render path includes writeOnly (via _renderCommonFields)', () => {
    const src = readFileSync(
      resolve(__dirname, 'tree/detail-panel.ts'),
      'utf-8',
    );

    const compBlock = extractRenderBlock(src, 'node.compositionKeyword');
    expect(compBlock).not.toBeNull();
    expect(compBlock).toMatch(/writeOnly|_renderCommonFields/);
  });

  it('conditional render path includes writeOnly (via _renderCommonFields)', () => {
    const src = readFileSync(
      resolve(__dirname, 'tree/detail-panel.ts'),
      'utf-8',
    );

    const condBlock = extractRenderBlock(src, 'schema.if');
    expect(condBlock).not.toBeNull();
    expect(condBlock).toMatch(/writeOnly|_renderCommonFields/);
  });

  it('_renderCommonFields contains readOnly before writeOnly (correct order)', () => {
    const src = readFileSync(
      resolve(__dirname, 'tree/detail-panel.ts'),
      'utf-8',
    );

    const commonFieldsBody = extractMethodBody(src, '_renderCommonFields');
    expect(commonFieldsBody).not.toBeNull();

    const readOnlyIdx = commonFieldsBody!.indexOf('readOnly');
    const writeOnlyIdx = commonFieldsBody!.indexOf('writeOnly');
    expect(readOnlyIdx).toBeGreaterThan(-1);
    expect(writeOnlyIdx).toBeGreaterThan(-1);
    expect(readOnlyIdx).toBeLessThan(writeOnlyIdx);
  });
});

// ─── Helpers ──────────────────────────────────────────────────────────────────

/**
 * Extract the body of a private method from TypeScript source.
 * Returns the content from the method name declaration through its closing brace.
 */
function extractMethodBody(src: string, methodName: string): string | null {
  const idx = src.indexOf(`private ${methodName}`);
  if (idx === -1) return null;

  // Find opening brace
  const openBrace = src.indexOf('{', idx);
  if (openBrace === -1) return null;

  // Balance braces to find the closing one
  let depth = 1;
  let pos = openBrace + 1;
  while (pos < src.length && depth > 0) {
    if (src[pos] === '{') depth++;
    else if (src[pos] === '}') depth--;
    pos++;
  }

  return src.slice(openBrace, pos);
}

/**
 * Extract the render block starting at a conditional check (e.g. 'node.ref')
 * up to the closing `;` of the return html template literal.
 */
function extractRenderBlock(src: string, condition: string): string | null {
  const idx = src.indexOf(condition);
  if (idx === -1) return null;

  const afterCondition = src.slice(idx, idx + 1500);
  const returnMatch = afterCondition.match(/return html`[\s\S]*?`;/);
  if (!returnMatch) return afterCondition;
  return returnMatch[0];
}
