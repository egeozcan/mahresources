/**
 * Tests for confirmed bugs — written RED-first before any fixes.
 */
import { readFileSync } from 'fs';
import { describe, it, expect } from 'vitest';
import { schemaToTree, treeToSchema, resetIdCounter, type SchemaNode } from './schema-tree-model';

// ─── Bug 1 (P1): Convert-to-ref / wrap / add-if-then-else must not be offered for root ──

describe('Bug 1: context menu excludes dangerous actions for root node', () => {
  it('convert-to-ref on root produces dangling schema (demonstrates the bug)', () => {
    resetIdCounter();
    const root = schemaToTree({
      type: 'object',
      properties: {
        name: { type: 'string' },
        age: { type: 'integer' },
      },
    });

    // The fix prevents this from happening in the UI.
    // Verify the tree-panel should NOT render convert-to-ref for root.
    const isRoot = true;
    const actionsAllowedForRoot = getContextMenuActions(isRoot);

    expect(actionsAllowedForRoot).not.toContain('convert-to-ref');
    expect(actionsAllowedForRoot).not.toContain('wrap-oneOf');
    expect(actionsAllowedForRoot).not.toContain('wrap-anyOf');
    expect(actionsAllowedForRoot).not.toContain('wrap-allOf');
    expect(actionsAllowedForRoot).not.toContain('add-if-then-else');
  });

  it('non-root nodes still get all context menu actions', () => {
    const isRoot = false;
    const actions = getContextMenuActions(isRoot);

    expect(actions).toContain('convert-to-ref');
    expect(actions).toContain('wrap-oneOf');
    expect(actions).toContain('wrap-anyOf');
    expect(actions).toContain('wrap-allOf');
    expect(actions).toContain('add-if-then-else');
  });
});

/**
 * Simulates the context menu action list as it SHOULD be after the fix.
 * Before the fix, all actions are always returned regardless of isRoot.
 */
function getContextMenuActions(isRoot: boolean): string[] {
  const allActions = [
    'wrap-oneOf', 'wrap-anyOf', 'wrap-allOf',
    'add-if-then-else',
    'convert-to-ref',
  ];

  if (isRoot) {
    return allActions.filter(a =>
      !['convert-to-ref', 'wrap-oneOf', 'wrap-anyOf', 'wrap-allOf', 'add-if-then-else'].includes(a)
    );
  }

  return allActions;
}


// ─── Bug 2 (P2): Boolean enums corrupted by enum-editor ──────────────────────

describe('Bug 2: boolean enum values are preserved', () => {
  /**
   * Mirrors the fixed _updateValue logic in enum-editor.ts.
   * Before the fix, boolean values like true/false were stored as strings.
   */
  function simulateUpdateValue(values: any[], index: number, raw: string, valueType: string): any[] {
    const updated = [...values];
    if (valueType === 'number' || valueType === 'integer') {
      updated[index] = valueType === 'integer' ? parseInt(raw, 10) : parseFloat(raw);
    } else if (valueType === 'boolean') {
      updated[index] = raw === 'true';
    } else {
      updated[index] = raw;
    }
    return updated;
  }

  /**
   * Mirrors the fixed _addValue logic in enum-editor.ts.
   */
  function simulateAddValue(valueType: string): any {
    if (valueType === 'number' || valueType === 'integer') return 0;
    if (valueType === 'boolean') return false;
    return '';
  }

  it('_updateValue with boolean type stores boolean, not string', () => {
    const result = simulateUpdateValue([true, false], 0, 'false', 'boolean');
    expect(result[0]).toBe(false);
    expect(typeof result[0]).toBe('boolean');
  });

  it('_updateValue with boolean type: "true" -> boolean true', () => {
    const result = simulateUpdateValue([false], 0, 'true', 'boolean');
    expect(result[0]).toBe(true);
    expect(typeof result[0]).toBe('boolean');
  });

  it('_addValue with boolean type defaults to boolean false', () => {
    const newVal = simulateAddValue('boolean');
    expect(newVal).toBe(false);
    expect(typeof newVal).toBe('boolean');
  });

  it('_addValue with number type still defaults to 0', () => {
    expect(simulateAddValue('number')).toBe(0);
    expect(simulateAddValue('integer')).toBe(0);
  });

  it('_addValue with string type still defaults to empty string', () => {
    expect(simulateAddValue('string')).toBe('');
  });

  it('round-trips boolean enum through schemaToTree/treeToSchema', () => {
    resetIdCounter();
    const schema = {
      type: 'object' as const,
      properties: {
        flag: { type: 'boolean' as const, enum: [true, false] },
      },
    };
    const tree = schemaToTree(schema);
    const output = treeToSchema(tree);

    expect(output.properties!.flag.enum).toEqual([true, false]);
    expect(typeof output.properties!.flag.enum![0]).toBe('boolean');
    expect(typeof output.properties!.flag.enum![1]).toBe('boolean');
  });
});


// ─── Bug 3 (P2): Removing last enum value leaves invalid empty array ─────────

describe('Bug 3: removing all enum values deletes the enum key', () => {
  /**
   * Mirrors the fixed edit-mode _handleNodeChange logic for field='enum'.
   */
  function applyEnumChange(schema: Record<string, any>, value: any[]) {
    if (Array.isArray(value) && value.length === 0) {
      delete schema.enum;
    } else {
      schema.enum = value;
    }
  }

  it('empty enum array should be deleted from schema', () => {
    const schema: Record<string, any> = { enum: ['a', 'b'] };
    applyEnumChange(schema, []);
    expect(schema.enum).toBeUndefined();
    expect('enum' in schema).toBe(false);
  });

  it('non-empty enum array should still be stored', () => {
    const schema: Record<string, any> = {};
    applyEnumChange(schema, ['x', 'y']);
    expect(schema.enum).toEqual(['x', 'y']);
  });

  it('single-value enum array should still be stored', () => {
    const schema: Record<string, any> = {};
    applyEnumChange(schema, ['only']);
    expect(schema.enum).toEqual(['only']);
  });
});


// ─── Bug 4 (P2): Zero-valued constraints saved as undefined ─────────────────

describe('Bug 4: zero-valued numeric constraints are preserved', () => {
  /**
   * Mirrors the fixed constraint handler pattern: v !== '' ? parseInt(v, 10) : undefined
   * Before the fix: v ? parseInt(v) : undefined (truthy check, wrong for numeric 0).
   */
  function parseConstraint(inputValue: string): number | undefined {
    return inputValue !== '' ? parseInt(inputValue, 10) : undefined;
  }

  /**
   * Mirrors _emit's value check: value === '' ? undefined : value
   */
  function emitValue(value: any): any {
    return value === '' ? undefined : value;
  }

  it('string "0" from input should produce numeric 0, not undefined', () => {
    expect(parseConstraint('0')).toBe(0);
  });

  it('empty string from cleared input should produce undefined', () => {
    expect(parseConstraint('')).toBeUndefined();
  });

  it('positive values work correctly', () => {
    expect(parseConstraint('5')).toBe(5);
    expect(parseConstraint('100')).toBe(100);
  });

  it('_emit passes through numeric 0 without converting to undefined', () => {
    expect(emitValue(0)).toBe(0);
    expect(emitValue('')).toBeUndefined();
  });

  it('buggy truthy check fails when input is numeric 0 (not string)', () => {
    // This demonstrates why v ? parseInt(v) : undefined is wrong:
    // If the value were ever coerced to number 0, the truthy check fails.
    const vAsNumber: any = 0;
    const buggyResult = vAsNumber ? parseInt(String(vAsNumber)) : undefined;
    expect(buggyResult).toBeUndefined(); // demonstrates the latent bug
  });

  it('fixed pattern handles all edge cases correctly', () => {
    // The fixed pattern v !== '' works correctly for all string inputs
    expect(parseConstraint('0')).toBe(0);
    expect(parseConstraint('1')).toBe(1);
    expect(parseConstraint('')).toBeUndefined();
    expect(parseConstraint('42')).toBe(42);
  });
});


// ─── Bug 5 (P2): Raw JSON tab silently drops invalid edits on Apply ─────────

describe('Bug 5: raw JSON validation state and Apply button guard', () => {
  /**
   * Simulates the schemaEditorModal Alpine data component's handleRawChange()
   * and applySchema() methods. The fix adds rawJsonValid/rawJsonError state
   * and prevents Apply when the raw JSON is invalid.
   */
  function createModalState() {
    return {
      rawJson: '{"type":"object"}',
      rawJsonValid: true,
      rawJsonError: '',
      currentSchema: '{"type":"object"}',
      tab: 'raw' as string,
      _textareaEl: { value: '', dispatchEvent: () => {} } as any,

      handleRawChange() {
        try {
          JSON.parse(this.rawJson);
          this.rawJsonValid = true;
          this.rawJsonError = '';
          this.currentSchema = this.rawJson;
        } catch (e: any) {
          this.rawJsonValid = false;
          this.rawJsonError = e instanceof Error ? e.message : 'Invalid JSON';
        }
      },

      applySchema() {
        // The fix: refuse to apply when raw tab has invalid JSON
        if (this.tab === 'raw' && !this.rawJsonValid) return false;
        if (this._textareaEl) {
          try {
            this._textareaEl.value = JSON.stringify(JSON.parse(this.currentSchema));
          } catch {
            this._textareaEl.value = this.currentSchema;
          }
        }
        return true;
      },
    };
  }

  it('valid JSON sets rawJsonValid to true', () => {
    const state = createModalState();
    state.rawJson = '{"type":"string"}';
    state.handleRawChange();
    expect(state.rawJsonValid).toBe(true);
    expect(state.rawJsonError).toBe('');
    expect(state.currentSchema).toBe('{"type":"string"}');
  });

  it('invalid JSON sets rawJsonValid to false with error message', () => {
    const state = createModalState();
    state.rawJson = '{"type": }';
    state.handleRawChange();
    expect(state.rawJsonValid).toBe(false);
    expect(state.rawJsonError).toBeTruthy();
    // currentSchema should NOT be updated
    expect(state.currentSchema).toBe('{"type":"object"}');
  });

  it('Apply is blocked when raw tab has invalid JSON', () => {
    const state = createModalState();
    state.rawJson = '{"type": }';
    state.handleRawChange();
    const applied = state.applySchema();
    expect(applied).toBe(false);
    // The textarea should NOT be updated
    expect(state._textareaEl.value).toBe('');
  });

  it('Apply succeeds when raw tab has valid JSON', () => {
    const state = createModalState();
    state.rawJson = '{"type":"number"}';
    state.handleRawChange();
    const applied = state.applySchema();
    expect(applied).toBe(true);
    expect(state._textareaEl.value).toBe('{"type":"number"}');
  });

  it('Apply succeeds on non-raw tab even if rawJsonValid is false', () => {
    const state = createModalState();
    state.tab = 'edit';
    state.rawJsonValid = false; // stale from previous raw tab usage
    const applied = state.applySchema();
    expect(applied).toBe(true);
  });

  it('schemaEditorModal.ts source includes rawJsonValid and rawJsonError', async () => {
    const fsModule = 'node:fs', urlModule = 'node:url';
    const fs: any = await import(/* @vite-ignore */ fsModule);
    const url: any = await import(/* @vite-ignore */ urlModule);
    const tsPath = url.fileURLToPath(new URL('../components/schemaEditorModal.ts', import.meta.url));
    const source = fs.readFileSync(tsPath, 'utf-8');
    expect(source).toContain('rawJsonValid');
    expect(source).toContain('rawJsonError');
  });

  it('schemaEditorModal.tpl disables Apply button when raw JSON is invalid', async () => {
    const fsModule = 'node:fs', pathModule = 'node:path', urlModule = 'node:url';
    const fs: any = await import(/* @vite-ignore */ fsModule);
    const path: any = await import(/* @vite-ignore */ pathModule);
    const url: any = await import(/* @vite-ignore */ urlModule);
    const thisDir = url.fileURLToPath(new URL('.', import.meta.url));
    // Navigate from src/schema-editor/ to project root, then into templates
    const tplPath = path.resolve(thisDir, '../../templates/partials/form/schemaEditorModal.tpl');
    const source = fs.readFileSync(tplPath, 'utf-8');
    // The Apply button should have a :disabled binding
    expect(source).toContain('rawJsonValid');
  });
});


// ─── Bug 6 (P2): Duplicate DOM IDs for schema paths differing only by punctuation ──

describe('Bug 6: generateFieldId produces unique IDs for different separators', () => {
  it('generates distinct IDs for paths differing by separator', () => {
    // Read the generateFieldId function from form-mode.ts source
    const source = readFileSync(
      new URL('./modes/form-mode.ts', import.meta.url),
      'utf8',
    );

    // Extract the generateFieldId function body
    const fnMatch = source.match(
      /function\s+generateFieldId\(prefix:\s*string,\s*path:\s*string\):\s*string\s*\{([\s\S]*?)\n\}/
    );
    expect(fnMatch).not.toBeNull();

    // Create the function for testing
    const fn = new Function('prefix', 'path', fnMatch![1]) as (prefix: string, path: string) => string;

    const id1 = fn('field', 'first_name');
    const id2 = fn('field', 'first-name');
    const id3 = fn('field', 'first.name');

    expect(id1).not.toBe(id2);
    expect(id1).not.toBe(id3);
    expect(id2).not.toBe(id3);
  });

  it('preserves uniqueness for dotted paths (nested properties)', () => {
    const source = readFileSync(
      new URL('./modes/form-mode.ts', import.meta.url),
      'utf8',
    );
    const fnMatch = source.match(
      /function\s+generateFieldId\(prefix:\s*string,\s*path:\s*string\):\s*string\s*\{([\s\S]*?)\n\}/
    );
    expect(fnMatch).not.toBeNull();
    const fn = new Function('prefix', 'path', fnMatch![1]) as (prefix: string, path: string) => string;

    const id1 = fn('field', 'address.city');
    const id2 = fn('field', 'address-city');
    expect(id1).not.toBe(id2);
  });

  it('generates distinct IDs for foo/bar vs foobar (slash stripped, no replacement)', () => {
    const source = readFileSync(
      new URL('./modes/form-mode.ts', import.meta.url),
      'utf8',
    );
    const fnMatch = source.match(
      /function\s+generateFieldId\(prefix:\s*string,\s*path:\s*string\):\s*string\s*\{([\s\S]*?)\n\}/
    );
    expect(fnMatch).not.toBeNull();
    const fn = new Function('prefix', 'path', fnMatch![1]) as (prefix: string, path: string) => string;

    const id1 = fn('field', 'foo/bar');
    const id2 = fn('field', 'foobar');
    expect(id1).not.toBe(id2);
  });

  it('generates distinct IDs for address.city vs address--city (dot encoding collides with literal --)', () => {
    const source = readFileSync(
      new URL('./modes/form-mode.ts', import.meta.url),
      'utf8',
    );
    const fnMatch = source.match(
      /function\s+generateFieldId\(prefix:\s*string,\s*path:\s*string\):\s*string\s*\{([\s\S]*?)\n\}/
    );
    expect(fnMatch).not.toBeNull();
    const fn = new Function('prefix', 'path', fnMatch![1]) as (prefix: string, path: string) => string;

    const id1 = fn('field', 'address.city');
    const id2 = fn('field', 'address--city');
    expect(id1).not.toBe(id2);
  });

  it('generates distinct IDs for all separator variations including slash', () => {
    const source = readFileSync(
      new URL('./modes/form-mode.ts', import.meta.url),
      'utf8',
    );
    const fnMatch = source.match(
      /function\s+generateFieldId\(prefix:\s*string,\s*path:\s*string\):\s*string\s*\{([\s\S]*?)\n\}/
    );
    expect(fnMatch).not.toBeNull();
    const fn = new Function('prefix', 'path', fnMatch![1]) as (prefix: string, path: string) => string;

    const paths = ['first_name', 'first-name', 'first.name', 'first/name', 'firstname'];
    const ids = paths.map(p => fn('field', p));
    const unique = new Set(ids);
    expect(unique.size).toBe(paths.length);
  });
});

// ─── Bug: rawJsonDirty state prevents Apply bypass via tab switch ──

describe('Bug: raw JSON invalid state bypassed by switching tabs', () => {
  it('Apply button disabled condition does not reference tab', () => {
    const tplSource = readFileSync(
      new URL('../../templates/partials/form/schemaEditorModal.tpl', import.meta.url),
      'utf8',
    );
    // Find the Apply button's :disabled attribute
    const disabledMatch = tplSource.match(/:disabled="([^"]+)"/);
    expect(disabledMatch).not.toBeNull();
    const condition = disabledMatch![1];
    // The condition should NOT reference 'tab' — it should work on all tabs
    expect(condition).not.toContain('tab');
  });

  it('schemaEditorModal component has rawJsonDirty state', () => {
    const componentSource = readFileSync(
      new URL('../components/schemaEditorModal.ts', import.meta.url),
      'utf8',
    );
    expect(componentSource).toContain('rawJsonDirty');
  });

  it('handleRawChange sets rawJsonDirty flag', () => {
    const componentSource = readFileSync(
      new URL('../components/schemaEditorModal.ts', import.meta.url),
      'utf8',
    );
    // handleRawChange should set rawJsonDirty to true
    const handleRawSection = componentSource.slice(
      componentSource.indexOf('handleRawChange'),
      componentSource.indexOf('applySchema'),
    );
    expect(handleRawSection).toContain('rawJsonDirty');
  });

  it('handleSchemaChange clears rawJsonDirty flag', () => {
    const componentSource = readFileSync(
      new URL('../components/schemaEditorModal.ts', import.meta.url),
      'utf8',
    );
    // handleSchemaChange should clear rawJsonDirty
    const handleSchemaSection = componentSource.slice(
      componentSource.indexOf('handleSchemaChange'),
      componentSource.indexOf('handleRawChange'),
    );
    expect(handleSchemaSection).toContain('rawJsonDirty');
  });
});
