/**
 * Bug: Untouched boolean properties omitted from form submission
 *
 * When _data starts as {} (new group), boolean properties that the user never
 * interacts with are never added to _data. The hidden input submits {} instead
 * of {"active": false}, silently dropping the boolean field.
 *
 * The fix: in _renderObject, after ensuring data is an object, pre-populate
 * any boolean property that is undefined with false (or the schema default).
 */
import { readFileSync } from 'fs';
import { describe, it, expect } from 'vitest';

describe('Bug: undefined boolean properties are pre-populated in _renderObject', () => {
  it('source code initializes undefined boolean properties to false before rendering', () => {
    const src = readFileSync(
      new URL('./modes/form-mode.ts', import.meta.url).pathname,
      'utf-8',
    );

    // Find _renderObject method
    const renderObjectMatch = src.match(
      /private _renderObject[\s\S]*?(?=\n\s*(?:private |\/\/ ─))/,
    );
    expect(renderObjectMatch).not.toBeNull();
    const renderObjectSrc = renderObjectMatch![0];

    // The method must contain logic that checks for boolean type and undefined
    // property values, then initializes them before rendering
    expect(renderObjectSrc).toMatch(/type.*boolean|boolean.*type/i);
    expect(renderObjectSrc).toMatch(/undefined/);

    // The initialization should happen via an assignment that sets missing
    // boolean properties (covering the pattern: data[key] = false or data = {...data, [key]: false})
    expect(renderObjectSrc).toMatch(/data\s*=\s*\{.*\[key\]|data\[key\]\s*=/);
  });

  it('source initializes booleans to schema default when present, else false', () => {
    const src = readFileSync(
      new URL('./modes/form-mode.ts', import.meta.url).pathname,
      'utf-8',
    );

    const renderObjectMatch = src.match(
      /private _renderObject[\s\S]*?(?=\n\s*(?:private |\/\/ ─))/,
    );
    expect(renderObjectMatch).not.toBeNull();
    const renderObjectSrc = renderObjectMatch![0];

    // The code should reference `false` as the fallback when no default exists,
    // covering the pattern: ps.default !== undefined ? ps.default : false
    // or: ?? false or similar
    expect(renderObjectSrc).toMatch(/false/);

    // Must call onChange (or queueMicrotask + onChange) when initialization happens
    // so the parent state is updated to include the new defaults
    expect(renderObjectSrc).toMatch(/onChange|needsUpdate/);
  });
});
