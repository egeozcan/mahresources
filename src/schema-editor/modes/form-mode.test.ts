import { readFileSync } from 'fs';
import { describe, it, expect } from 'vitest';

/**
 * These tests verify that _renderField forwards describedBy and isRequired
 * through $ref and allOf recursive calls. Since form-mode.ts is a Lit
 * component requiring a DOM, we verify by inspecting the source code to
 * confirm the recursive call sites include the forwarded parameters.
 */

const source = readFileSync(
  new URL('./form-mode.ts', import.meta.url),
  'utf8',
);

describe('$ref/allOf attribute forwarding in _renderField', () => {
  it('forwards describedBy and isRequired through $ref resolution', () => {
    // Find the $ref handling block.  It starts with "// Handle $ref" and the
    // recursive call is the first this._renderField(...) that follows.
    const refSection = source.slice(
      source.indexOf('// Handle $ref'),
      source.indexOf('// Handle oneOf'),
    );
    expect(refSection).toBeTruthy();

    // The recursive _renderField call in the $ref branch
    const callMatch = refSection.match(
      /return\s+this\._renderField\(([^)]+)\)/s,
    );
    expect(callMatch).not.toBeNull();
    const args = callMatch![1];

    // Must include describedBy and isRequired in the argument list
    expect(args).toContain('describedBy');
    expect(args).toContain('isRequired');
  });

  it('forwards describedBy and isRequired through allOf merge', () => {
    // Find the allOf handling block.
    const allOfSection = source.slice(
      source.indexOf('// Handle allOf'),
      source.indexOf('// Handle anyOf'),
    );
    expect(allOfSection).toBeTruthy();

    // The recursive _renderField call in the allOf branch
    const callMatch = allOfSection.match(
      /return\s+this\._renderField\(([^)]+)\)/s,
    );
    expect(callMatch).not.toBeNull();
    const args = callMatch![1];

    // Must include describedBy and isRequired in the argument list
    expect(args).toContain('describedBy');
    expect(args).toContain('isRequired');
  });
});
