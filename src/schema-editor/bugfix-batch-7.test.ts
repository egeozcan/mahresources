import { readFileSync } from 'fs';
import { describe, it, expect } from 'vitest';
import { flattenSchema } from './schema-core';

// ─── Bug 1: Required boolean fields cannot be submitted as false ────────────

describe('Bug 1: boolean checkbox must not get HTML required attribute', () => {
  /**
   * In form-mode.ts, _renderPrimitive applies `?required=${!!isRequired}` and
   * `aria-required` to checkbox inputs. In HTML, a required checkbox is only
   * valid when checked, so a schema-required boolean forces the user to check
   * it (true). Submitting false is impossible.
   *
   * The fix: either _renderFieldWithAttributes must not pass isRequired for
   * boolean-typed schemas, or _renderPrimitive must skip required for booleans.
   */
  it('source code does not apply required attribute to boolean checkbox', () => {
    const src = readFileSync(
      new URL('./modes/form-mode.ts', import.meta.url).pathname,
      'utf-8',
    );

    // Find the boolean rendering section in _renderPrimitive
    const primitiveMatch = src.match(
      /private _renderPrimitive[\s\S]*?if \(type === 'boolean'\)([\s\S]*?)if \(type === 'integer'/,
    );
    expect(primitiveMatch).not.toBeNull();
    const booleanBlock = primitiveMatch![1];

    // The boolean block should NOT set ?required or aria-required
    expect(booleanBlock).not.toContain('?required');
    expect(booleanBlock).not.toContain('aria-required');
  });
});

// ─── Bug 2: Re-adding conditionals overwrites existing if/then/else ─────────

describe('Bug 2: add-if-then-else does not overwrite existing conditionals', () => {
  /**
   * In edit-mode.ts, the 'add-if-then-else' context action unconditionally
   * sets schema.if/then/else = { properties: {} }, overwriting any existing
   * conditionals.
   *
   * The fix: guard the action and hide the menu item when schema.if exists.
   */
  it('handler guards against existing schema.if in edit-mode.ts', () => {
    const src = readFileSync(
      new URL('./modes/edit-mode.ts', import.meta.url).pathname,
      'utf-8',
    );

    // Find the add-if-then-else case block
    const caseMatch = src.match(
      /case 'add-if-then-else':([\s\S]*?)(?:case '|break;\s*\})/,
    );
    expect(caseMatch).not.toBeNull();
    const caseBlock = caseMatch![1];

    // Must check if node already has schema.if before setting
    // Acceptable patterns: node.schema.if, schema.if, .if
    expect(caseBlock).toMatch(/\.schema\.if|\.if/);
    // Must have a guard (if statement) before the assignment
    expect(caseBlock).toMatch(/if\s*\(/);
  });

  it('context menu hides add-if-then-else when node has schema.if in tree-panel.ts', () => {
    const src = readFileSync(
      new URL('./tree/tree-panel.ts', import.meta.url).pathname,
      'utf-8',
    );

    // The context menu should check for existing conditionals before showing
    // the "Add if/then/else" button. The source should reference schema.if
    // near the add-if-then-else menu item.
    const contextMenuMatch = src.match(
      /_renderContextMenu[\s\S]*?add-if-then-else/,
    );
    expect(contextMenuMatch).not.toBeNull();

    // There must be a conditional check near the add-if-then-else button
    // that references .schema?.if or .schema.if
    const contextMenuSection = src.match(
      /_renderContextMenu[\s\S]*/,
    );
    expect(contextMenuSection).not.toBeNull();
    const menuSource = contextMenuSection![0];

    // The section should contain a check for schema.if to conditionally
    // hide the add-if-then-else option
    expect(menuSource).toMatch(/schema\?*\.if/);
  });
});

// ─── Bug 3: Boolean enums rendered as unrestricted booleans in search mode ──

describe('Bug 3: boolean enum fields render as enum controls in search mode', () => {
  /**
   * In search-mode.ts, _renderField checks field.type === 'boolean' before
   * field.enum. So { type: "boolean", enum: [true] } gets the generic
   * Yes/No/Any radio instead of enum-specific checkboxes.
   *
   * The fix: check field.enum BEFORE field.type === 'boolean'.
   */
  it('flattenSchema preserves enum on boolean-typed fields', () => {
    const schema = {
      type: 'object',
      properties: {
        isActive: { type: 'boolean', enum: [true] },
      },
    };
    const fields = flattenSchema(schema);
    const isActive = fields.find(f => f.path === 'isActive');
    expect(isActive).toBeDefined();
    expect(isActive!.type).toBe('boolean');
    expect(isActive!.enum).toEqual([true]);
  });

  it('search-mode renders enum fields before boolean check', () => {
    const src = readFileSync(
      new URL('./modes/search-mode.ts', import.meta.url).pathname,
      'utf-8',
    );

    // Find the _renderField method where the type dispatch happens
    const renderFieldMatch = src.match(
      /private _renderField[\s\S]*?return html/,
    );
    expect(renderFieldMatch).not.toBeNull();
    const renderFieldSrc = renderFieldMatch![0];

    // The enum check (field.enum) must appear BEFORE the boolean check
    // (field.type === 'boolean') in the rendered template.
    const enumCheckPos = renderFieldSrc.indexOf('field.enum');
    const boolCheckPos = renderFieldSrc.indexOf("field.type === 'boolean'");

    // If both exist, enum must come first
    if (enumCheckPos >= 0 && boolCheckPos >= 0) {
      expect(enumCheckPos).toBeLessThan(boolCheckPos);
    }

    // The template itself: enum should be checked first in the ternary chain
    const templateMatch = src.match(
      /\$\{field\.type === 'boolean'[\s\S]*?_renderBoolean|field\.enum[\s\S]*?_renderEnum/,
    );
    // If the match starts with field.enum, it's correct (enum comes first)
    // If it starts with field.type === 'boolean', it's wrong
    if (templateMatch) {
      expect(templateMatch[0]).not.toMatch(/^\$\{field\.type === 'boolean'/);
    }
  });
});

// ─── Bug 4: String search values not escaped before quoting ─────────────────

describe('Bug 4: string search values escape quotes and backslashes', () => {
  /**
   * In search-mode.ts _getHiddenInputs, string values are wrapped in double
   * quotes without escaping. If the value contains " or \, the output is
   * malformed. The fix: escape \ and " before wrapping.
   */
  it('source code escapes backslashes and quotes in string field values', () => {
    const src = readFileSync(
      new URL('./modes/search-mode.ts', import.meta.url).pathname,
      'utf-8',
    );

    // Find the _getHiddenInputs method - specifically the string type branch
    const methodMatch = src.match(
      /private _getHiddenInputs[\s\S]*?(?=\n\s*(?:private |\/\/ ─))/,
    );
    expect(methodMatch).not.toBeNull();
    const methodSrc = methodMatch![0];

    // The string branch must escape backslashes and quotes
    // It should contain a .replace() call for escaping
    expect(methodSrc).toMatch(/\.replace\(/);

    // Specifically, it should escape backslashes (\\) and double quotes (")
    // The exact regex may vary, but the key patterns are:
    // - replace(/\\/g, '\\\\') or similar for backslashes
    // - replace(/"/g, '\\"') or similar for quotes
    expect(methodSrc).toMatch(/\\\\|backslash/i); // some reference to backslash escaping
  });

  it('enum string values are also escaped before quoting', () => {
    const src = readFileSync(
      new URL('./modes/search-mode.ts', import.meta.url).pathname,
      'utf-8',
    );

    // Find the enum section in _getHiddenInputs
    const enumMatch = src.match(
      /if \(field\.enum\)[\s\S]*?return field\.enumValues\.map/,
    );
    expect(enumMatch).not.toBeNull();
    const enumSection = enumMatch![0];

    // String enums must also be escaped when quoted
    // The section should contain a replace() or escape function call
    // (If it quotes with `"${v}"`, it must escape v first)
    const quotedPattern = enumSection.match(/`"\$\{/);
    if (quotedPattern) {
      // If values are quoted with template literals, there must be escaping
      expect(enumSection).toMatch(/\.replace\(/);
    }
  });

  it('freeFields.js generateParamNameForMeta also escapes string values', () => {
    const src = readFileSync(
      new URL('../components/freeFields.js', import.meta.url).pathname,
      'utf-8',
    );

    // Find the generateParamNameForMeta function
    const fnMatch = src.match(
      /export function generateParamNameForMeta[\s\S]*?^\}/m,
    );
    expect(fnMatch).not.toBeNull();
    const fnSrc = fnMatch![0];

    // The function wraps string values in quotes: `"${realValue}"`
    // It must escape backslashes and quotes in the value first
    expect(fnSrc).toMatch(/\.replace\(/);
  });
});
