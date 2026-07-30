import { describe, it, expect } from 'vitest';
import { jsonParseDiagnostics } from './jsonLint.js';

// WS12, findings 17/93 (client half). The Meta JSON Schema editor had no lint at
// all, so `{ not valid json ][` produced no marker anywhere. Measured on the
// unfixed page: `{"value":"{ not valid json ][","lintMarkers":0,"diagText":[]}`.
//
// The parse error itself was already computable in the browser — clicking "Format
// JSON" reports `Expected property name or '}' in JSON at position 2 (line 1
// column 3)` in a visible role="alert" — it just was never attached to the text.

describe('findings 17/93 — the JSON editor lints its content', () => {
  it('reports nothing for valid JSON', () => {
    expect(jsonParseDiagnostics('{"type":"object"}')).toEqual([]);
  });

  it('reports nothing for an empty or whitespace-only document', () => {
    expect(jsonParseDiagnostics('')).toEqual([]);
    expect(jsonParseDiagnostics('   \n ')).toEqual([]);
  });

  it('reports the parser’s own message for the schema from the report', () => {
    const diags = jsonParseDiagnostics('{ not valid json ][');
    expect(diags).toHaveLength(1);
    expect(diags[0].severity).toBe('error');
    // V8's wording. Asserted loosely because the exact phrasing is the engine's.
    expect(diags[0].message.toLowerCase()).toContain('json');
  });

  it('anchors the diagnostic at the reported offset rather than the whole document', () => {
    const doc = '{"a": 1, oops}';
    const diags = jsonParseDiagnostics(doc);
    expect(diags).toHaveLength(1);
    // V8 puts the failing offset in the message; the range must be narrow enough to
    // point at it rather than underlining everything.
    expect(diags[0].to - diags[0].from).toBeLessThanOrEqual(2);
    expect(diags[0].from).toBeGreaterThan(0);
    expect(diags[0].to).toBeLessThanOrEqual(doc.length);
  });

  it('keeps the range inside the document even for a truncated one', () => {
    const doc = '{';
    const diags = jsonParseDiagnostics(doc);
    expect(diags).toHaveLength(1);
    expect(diags[0].from).toBeGreaterThanOrEqual(0);
    expect(diags[0].to).toBeLessThanOrEqual(doc.length);
    expect(diags[0].from).toBeLessThanOrEqual(diags[0].to);
  });

  it('reports the report’s other example too — a bare non-JSON string', () => {
    const diags = jsonParseDiagnostics('not json at all ###');
    expect(diags).toHaveLength(1);
    expect(diags[0].severity).toBe('error');
  });
});
