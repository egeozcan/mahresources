/**
 * Bug 1 (P1): Optional object properties submit invalid partial payloads
 * Bug 2 (P2): Schema preview incorrect for non-object root schemas
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

// ─── Bug 1: Optional object properties auto-materialize as {} ─────────────

describe('Bug 1: optional object properties should not auto-materialize', () => {
  const formSrc = () => readSource('./modes/form-mode.ts');

  it('_renderObject receives isRequired parameter to distinguish optional from required', () => {
    const src = formSrc();
    // Match the full multi-line signature
    const sigMatch = src.match(/private _renderObject\(([\s\S]*?)\):\s*TemplateResult/);
    expect(sigMatch).not.toBeNull();
    expect(sigMatch![1]).toContain('isRequired');
  });

  it('_renderObject shows initialize button for optional objects with undefined data', () => {
    const src = formSrc();
    const renderObjectSection = src.slice(
      src.indexOf('private _renderObject('),
      src.indexOf('// ─── additional properties'),
    );
    expect(renderObjectSection).toContain('Initialize');
  });

  it('_renderObject call site passes isRequired from parent context', () => {
    const src = formSrc();
    const renderFieldSection = src.slice(
      src.indexOf('// Object type'),
      src.indexOf('// Array type'),
    );
    expect(renderFieldSection).toContain('isRequired');
  });

  it('auto-materialization is guarded: optional undefined objects get Initialize button', () => {
    const src = formSrc();
    const renderObjectSection = src.slice(
      src.indexOf('private _renderObject('),
      src.indexOf('// ─── additional properties'),
    );

    // The initialize button path should check for undefined data and !isRequired
    // We verify the initialize button section references both 'undefined' and 'isRequired'
    const initBtnIdx = renderObjectSection.indexOf('Initialize');
    expect(initBtnIdx).toBeGreaterThan(-1);

    // The section before the Initialize button should check isRequired
    const beforeInit = renderObjectSection.slice(0, initBtnIdx);
    expect(beforeInit).toContain('isRequired');

    // And should check for undefined data
    expect(beforeInit).toContain('undefined');
  });
});

// ─── Bug 2: Schema preview incorrect for non-object root schemas ──────────

describe('Bug 2: schema preview uses correct default value for non-object schemas', () => {
  it('schemaEditorModal has getPreviewValue method', () => {
    const src = readSource('../components/schemaEditorModal.ts');
    expect(src).toContain('getPreviewValue');
  });

  it('module-level getPreviewValue handles all JSON schema types', () => {
    const src = readSource('../components/schemaEditorModal.ts');
    // The exported getPreviewValue function should handle all types
    const fnMatch = src.match(/export function getPreviewValue[\s\S]*?\n\}/);
    expect(fnMatch).not.toBeNull();
    const body = fnMatch![0];
    expect(body).toContain("'string'");
    expect(body).toContain("'number'");
    expect(body).toContain("'integer'");
    expect(body).toContain("'boolean'");
    expect(body).toContain("'array'");
  });

  it('template uses getPreviewValue() instead of hardcoded "{}"', () => {
    const tpl = readSource('../../templates/partials/form/schemaEditorModal.tpl');
    // The preview tab panel contains the <schema-editor mode="form" ...> element
    const previewPanelIdx = tpl.indexOf('id="panel-preview"');
    expect(previewPanelIdx).toBeGreaterThan(-1);
    const previewPanel = tpl.slice(previewPanelIdx, tpl.indexOf('</template>', previewPanelIdx));
    expect(previewPanel).toContain('getPreviewValue');
    expect(previewPanel).not.toContain('value="{}"');
  });
});

/**
 * Functional test: import the exported getPreviewValue helper and verify
 * correct defaults for each schema type.
 */
import { getPreviewValue } from '../components/schemaEditorModal';

describe('Bug 2: getPreviewValue returns correct defaults', () => {
  it('returns {} for object schemas', () => {
    expect(JSON.parse(getPreviewValue('{"type":"object","properties":{}}'))).toEqual({});
  });

  it('returns "" for string schemas', () => {
    expect(JSON.parse(getPreviewValue('{"type":"string"}'))).toBe('');
  });

  it('returns 0 for number schemas', () => {
    expect(JSON.parse(getPreviewValue('{"type":"number"}'))).toBe(0);
  });

  it('returns 0 for integer schemas', () => {
    expect(JSON.parse(getPreviewValue('{"type":"integer"}'))).toBe(0);
  });

  it('returns false for boolean schemas', () => {
    expect(JSON.parse(getPreviewValue('{"type":"boolean"}'))).toBe(false);
  });

  it('returns [] for array schemas', () => {
    expect(JSON.parse(getPreviewValue('{"type":"array"}'))).toEqual([]);
  });

  it('returns {} when schema has no type (defaults to object)', () => {
    expect(JSON.parse(getPreviewValue('{"properties":{"x":{"type":"string"}}}'))).toEqual({});
  });

  it('returns {} for invalid JSON (graceful fallback)', () => {
    expect(JSON.parse(getPreviewValue('not valid json'))).toEqual({});
  });
});
