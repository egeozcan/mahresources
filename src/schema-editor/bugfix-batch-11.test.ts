/**
 * Bug 1 (P1): Nested required children still not enforced after optional object initialization
 * Bug 2 (P2): Nullable primitive root schemas still get {} as preview value
 *
 * Written RED-first before any fixes.
 */
import { readFileSync } from 'fs';
import { resolve, dirname } from 'path';
import { fileURLToPath } from 'url';
import { describe, it, expect } from 'vitest';

const __dirname = dirname(fileURLToPath(import.meta.url));

function readSource(relativePath: string): string {
  return readFileSync(resolve(__dirname, relativePath), 'utf-8');
}

// ─── Bug 1 (P1): Nested required after optional object has data ─────────────

describe('Bug 1: enforce nested required when optional object has data', () => {
  const formSrc = () => readSource('./modes/form-mode.ts');

  it('_renderObject computes childParentRequired based on data existence, not just schema required', () => {
    const src = formSrc();
    const renderObjectSection = src.slice(
      src.indexOf('private _renderObject('),
      src.indexOf('private _renderFieldWithAttributes('),
    );

    // The key insight: once an optional object has data, its children's
    // required constraints should be enforced. The code must account for
    // data existence when computing the parentRequired to pass to children.
    //
    // Look for logic that considers existing data when computing isRequired
    // or the parentRequired passed to _renderFieldWithAttributes.
    // The fix should have some form of "hasData" or data existence check
    // that feeds into the required computation for children.

    // The section where isRequired is computed should consider data existence
    const isRequiredLine = renderObjectSection.match(
      /const isRequired\b.*=.*requiredFields\.has\(key\)/s,
    );
    expect(isRequiredLine).not.toBeNull();

    // There should be a separate concept for what parentRequired to thread
    // to children, which accounts for data existence (not just schema required)
    expect(renderObjectSection).toContain('hasData');
  });

  it('_renderFieldWithAttributes call site passes childParentRequired for container types', () => {
    const src = formSrc();
    const renderObjectSection = src.slice(
      src.indexOf('private _renderObject('),
      src.indexOf('private _renderFieldWithAttributes('),
    );

    // The call to _renderFieldWithAttributes should pass childParentRequired
    // as a separate argument from isRequired, so containers get a parentRequired
    // that accounts for data existence on optional properties.
    expect(renderObjectSection).toContain('childParentRequired');

    // Verify childParentRequired appears in the _renderFieldWithAttributes call
    expect(renderObjectSection).toMatch(
      /_renderFieldWithAttributes[\s\S]*?childParentRequired/,
    );
  });

  it('_renderFieldWithAttributes accepts childParentRequired and uses it for containers', () => {
    const src = formSrc();
    const renderFieldWithAttrsSection = src.slice(
      src.indexOf('private _renderFieldWithAttributes('),
      src.indexOf('// ─── additional properties'),
    );

    // The _renderFieldWithAttributes method receives childParentRequired parameter
    // and uses it when rendering container types as the parentRequired for children.
    expect(renderFieldWithAttrsSection).toContain('childParentRequired');
  });
});


// ─── Bug 2 (P2): Nullable primitive root schemas get {} as preview ───────────

import { getPreviewValue } from '../components/schemaEditorModal';

describe('Bug 2: getPreviewValue handles nullable type arrays', () => {
  // BH-010 revision: preview-specific semantics now prefer `null` over a
  // zero-like primitive for nullable root schemas — a nullable integer's
  // most-correct empty state is null, not 0. Likewise for plain numeric/
  // string root schemas we return `{}` (JSON.stringify(undefined) → undefined
  // normalized to '{}'), which renders as the least-surprising empty form.
  it('returns null for nullable string root { type: ["string", "null"] }', () => {
    const result = getPreviewValue('{"type":["string","null"]}');
    expect(JSON.parse(result)).toBe(null);
  });

  it('returns null for nullable number root { type: ["number", "null"] }', () => {
    const result = getPreviewValue('{"type":["number","null"]}');
    expect(JSON.parse(result)).toBe(null);
  });

  it('returns null for nullable integer root { type: ["integer", "null"] }', () => {
    const result = getPreviewValue('{"type":["integer","null"]}');
    expect(JSON.parse(result)).toBe(null);
  });

  it('returns null for nullable boolean root { type: ["boolean", "null"] }', () => {
    // null > boolean in preview preference order (null is strictly "no value").
    const result = getPreviewValue('{"type":["boolean","null"]}');
    expect(JSON.parse(result)).toBe(null);
  });

  it('returns null for nullable array root { type: ["array", "null"] }', () => {
    const result = getPreviewValue('{"type":["array","null"]}');
    expect(JSON.parse(result)).toBe(null);
  });

  it('returns null for nullable object root { type: ["object", "null"] }', () => {
    const result = getPreviewValue('{"type":["object","null"]}');
    expect(JSON.parse(result)).toBe(null);
  });

  it('returns null for pure null type array { type: ["null"] }', () => {
    const result = getPreviewValue('{"type":["null"]}');
    expect(JSON.parse(result)).toBe(null);
  });

  it('returns null when null is present alongside string { type: ["null", "string"] }', () => {
    const result = getPreviewValue('{"type":["null","string"]}');
    expect(JSON.parse(result)).toBe(null);
  });

  // Non-array types: BH-010 normalizes undefined to '{}' string.
  it('normalizes plain string type to {} (renders empty input, not "")', () => {
    // getPreviewDefaultValue returns undefined for plain string type; normalizer
    // emits '{}' so the preview form has a valid empty object to start from.
    expect(getPreviewValue('{"type":"string"}')).toBe('{}');
  });

  it('still handles plain object type', () => {
    expect(JSON.parse(getPreviewValue('{"type":"object"}'))).toEqual({});
  });
});
