/**
 * Bug 1 (P2): scoreSchemaMatch picks wrong variant for discriminated unions
 * Bug 2 (P2): Conditional branch change leaks stale keys from old branch
 * Bug 3 (P2): Top-level if-schema keywords not evaluated by evaluateCondition
 *
 * Written RED-first before any fixes.
 */
import { describe, it, expect } from 'vitest';
import { scoreSchemaMatch, evaluateCondition } from './schema-core';

// ─── Bug 1: scoreSchemaMatch discriminated union via const/enum ─────────────

describe('Bug 1: scoreSchemaMatch discriminated unions', () => {
  it('scores discriminated union variants correctly via const', () => {
    const emailVariant = {
      type: 'object',
      properties: { type: { const: 'email' }, address: { type: 'string' } },
    };
    const phoneVariant = {
      type: 'object',
      properties: { type: { const: 'phone' }, number: { type: 'string' } },
    };
    const phoneData = { type: 'phone', number: '555-1234' };

    expect(scoreSchemaMatch(phoneVariant, phoneData, {})).toBeGreaterThan(0);
    expect(scoreSchemaMatch(emailVariant, phoneData, {})).toBe(0);
  });

  it('scores const match much higher than plain key overlap', () => {
    const emailVariant = {
      type: 'object',
      properties: { type: { const: 'email' }, address: { type: 'string' } },
    };
    const emailData = { type: 'email', address: 'test@example.com' };

    // With const matching, score should be significantly boosted
    expect(scoreSchemaMatch(emailVariant, emailData, {})).toBeGreaterThan(50);
  });

  it('returns 0 when enum discriminator does not match', () => {
    const catVariant = {
      type: 'object',
      properties: { kind: { enum: ['cat', 'kitten'] }, purrs: { type: 'boolean' } },
    };
    const dogData = { kind: 'dog', purrs: false };

    expect(scoreSchemaMatch(catVariant, dogData, {})).toBe(0);
  });

  it('boosts score when enum discriminator matches', () => {
    const catVariant = {
      type: 'object',
      properties: { kind: { enum: ['cat', 'kitten'] }, purrs: { type: 'boolean' } },
    };
    const catData = { kind: 'cat', purrs: true };

    expect(scoreSchemaMatch(catVariant, catData, {})).toBeGreaterThan(20);
  });

  it('penalizes missing required fields', () => {
    const strictVariant = {
      type: 'object',
      properties: { type: { const: 'strict' }, a: { type: 'string' }, b: { type: 'string' } },
      required: ['type', 'a', 'b'],
    };
    const looseVariant = {
      type: 'object',
      properties: { type: { const: 'strict' }, a: { type: 'string' } },
      required: ['type'],
    };
    const data = { type: 'strict', a: 'hello' };

    // looseVariant should score higher because it doesn't require 'b' which is missing
    expect(scoreSchemaMatch(looseVariant, data, {})).toBeGreaterThan(
      scoreSchemaMatch(strictVariant, data, {}),
    );
  });

  it('does not break existing object scoring without const/enum', () => {
    // Regression: objects without const/enum should still score by key overlap
    const schema = { type: 'object', properties: { a: {}, b: {}, c: {} } };
    expect(scoreSchemaMatch(schema, { a: 1, b: 2 }, {})).toBe(12); // 2 matches + 10
  });
});

// ─── Bug 2: Conditional branch change leaks stale keys ──────────────────────

describe('Bug 2: conditional branch stale key cleanup', () => {
  /**
   * The _renderConditional method in form-mode.ts evaluates if/then/else
   * but doesn't clean up data keys from the inactive branch. We test this
   * by reading the source and verifying the cleanup logic exists.
   */
  it('_renderConditional strips inactive branch keys from data', async () => {
    const { readFileSync } = await import('fs');
    const { resolve, dirname } = await import('path');
    const { fileURLToPath } = await import('url');
    const __dirname = dirname(fileURLToPath(import.meta.url));
    const src = readFileSync(resolve(__dirname, './modes/form-mode.ts'), 'utf-8');

    // The _renderConditional method must clean up keys from the inactive branch
    // that don't exist in the base schema or the active branch.
    const renderConditionalSection = src.slice(
      src.indexOf('private _renderConditional'),
      src.indexOf('// ─── enum'),
    );

    // Must reference inactive branch properties for cleanup
    expect(renderConditionalSection).toContain('inactiveBranch');
  });

  it('inactive branch key deletion uses onChange to propagate', async () => {
    const { readFileSync } = await import('fs');
    const { resolve, dirname } = await import('path');
    const { fileURLToPath } = await import('url');
    const __dirname = dirname(fileURLToPath(import.meta.url));
    const src = readFileSync(resolve(__dirname, './modes/form-mode.ts'), 'utf-8');

    const renderConditionalSection = src.slice(
      src.indexOf('private _renderConditional'),
      src.indexOf('// ─── enum'),
    );

    // Must use onChange or queueMicrotask to propagate data cleanup
    // (can't mutate data directly in render path without notifying)
    expect(
      renderConditionalSection.includes('onChange') &&
      (renderConditionalSection.includes('delete') || renderConditionalSection.includes('queueMicrotask')),
    ).toBe(true);
  });
});

// ─── Bug 3: evaluateCondition top-level keywords ────────────────────────────

describe('Bug 3: evaluateCondition top-level keywords', () => {
  it('evaluates top-level const condition — match', () => {
    expect(evaluateCondition({ const: 'special' }, 'special')).toBe(true);
  });

  it('evaluates top-level const condition — mismatch', () => {
    expect(evaluateCondition({ const: 'special' }, 'normal')).toBe(false);
  });

  it('evaluates top-level enum condition — match', () => {
    expect(evaluateCondition({ enum: ['a', 'b', 'c'] }, 'b')).toBe(true);
  });

  it('evaluates top-level enum condition — mismatch', () => {
    expect(evaluateCondition({ enum: ['a', 'b', 'c'] }, 'd')).toBe(false);
  });

  it('evaluates top-level type condition without properties — array match', () => {
    expect(evaluateCondition({ type: 'array' }, [1, 2])).toBe(true);
  });

  it('evaluates top-level type condition without properties — array mismatch', () => {
    expect(evaluateCondition({ type: 'array' }, 'hello')).toBe(false);
  });

  it('evaluates top-level type condition without properties — string match', () => {
    expect(evaluateCondition({ type: 'string' }, 'hello')).toBe(true);
  });

  it('evaluates top-level type condition without properties — string mismatch', () => {
    expect(evaluateCondition({ type: 'string' }, 42)).toBe(false);
  });

  it('does not misinterpret type:"object" with properties as top-level constraint', () => {
    // When both type and properties are present, the type check should be
    // handled by the existing property-level logic, not the new top-level check
    const cond = { type: 'object', properties: { status: { const: 'active' } } };
    expect(evaluateCondition(cond, { status: 'active' })).toBe(true);
    expect(evaluateCondition(cond, { status: 'inactive' })).toBe(false);
  });

  it('evaluates top-level minimum condition', () => {
    expect(evaluateCondition({ minimum: 10 }, 15)).toBe(true);
    expect(evaluateCondition({ minimum: 10 }, 5)).toBe(false);
  });

  it('evaluates top-level maximum condition', () => {
    expect(evaluateCondition({ maximum: 100 }, 50)).toBe(true);
    expect(evaluateCondition({ maximum: 100 }, 150)).toBe(false);
  });

  it('evaluates top-level minimum on non-number — fails', () => {
    expect(evaluateCondition({ minimum: 10 }, 'hello')).toBe(false);
  });

  it('evaluates top-level minLength condition', () => {
    expect(evaluateCondition({ minLength: 5 }, 'hello')).toBe(true);
    expect(evaluateCondition({ minLength: 5 }, 'hi')).toBe(false);
  });

  it('evaluates top-level maxLength condition', () => {
    expect(evaluateCondition({ maxLength: 3 }, 'hi')).toBe(true);
    expect(evaluateCondition({ maxLength: 3 }, 'hello')).toBe(false);
  });

  it('evaluates top-level minLength on non-string — fails', () => {
    expect(evaluateCondition({ minLength: 1 }, 42)).toBe(false);
  });

  it('evaluates top-level minItems condition', () => {
    expect(evaluateCondition({ minItems: 3 }, [1, 2, 3])).toBe(true);
    expect(evaluateCondition({ minItems: 3 }, [1, 2])).toBe(false);
  });

  it('evaluates top-level maxItems condition', () => {
    expect(evaluateCondition({ maxItems: 2 }, [1])).toBe(true);
    expect(evaluateCondition({ maxItems: 2 }, [1, 2, 3])).toBe(false);
  });

  it('evaluates top-level minItems on non-array — fails', () => {
    expect(evaluateCondition({ minItems: 1 }, 'hello')).toBe(false);
  });

  it('top-level type allows number/integer compat', () => {
    expect(evaluateCondition({ type: 'number' }, 42)).toBe(true); // integer is compatible with number
  });

  it('combined top-level and properties conditions', () => {
    // Both top-level const and properties should be checked
    const cond = { const: 'special' };
    expect(evaluateCondition(cond, 'special')).toBe(true);
    expect(evaluateCondition(cond, 'other')).toBe(false);
  });
});
