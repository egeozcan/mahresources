import { linter, lintGutter } from '@codemirror/lint';

// Findings 17/93: the Meta JSON Schema editor had no lint of any kind, so
// `{ not valid json ][` produced no marker, saved, and was reloaded verbatim —
// while every entity form in that category silently degraded to the freeform
// meta editor. The exact parse error was already computable in the browser (the
// Visual Editor's Raw JSON tab computes it, and "Format JSON" reports it), it
// just was not attached to the text.
//
// This is client-side feedback only. The rejection itself is enforced on the
// server (application_context.ValidateMetaSchema), because a client check is a
// convenience and not a rule.

// V8, SpiderMonkey and JavaScriptCore all put the failing offset in the
// SyntaxError message. There is no structured API for it, so read it out when it
// is there and fall back to marking the whole document when it is not.
const POSITION_PATTERNS = [
  /at position (\d+)/i, // V8 / JavaScriptCore
  /at line \d+ column \d+ of the JSON data/i, // SpiderMonkey (no absolute offset)
];

export function jsonParseDiagnostics(doc) {
  if (!doc.trim()) return [];
  try {
    JSON.parse(doc);
    return [];
  } catch (err) {
    const message = err?.message || 'Invalid JSON';
    let from = 0;
    let to = doc.length;
    const m = POSITION_PATTERNS[0].exec(message);
    if (m) {
      const pos = Math.min(Number(m[1]), doc.length);
      from = Math.max(0, pos - 1);
      to = Math.min(doc.length, pos + 1);
      if (from === to) from = Math.max(0, to - 1);
    }
    return [{ from, to, severity: 'error', message }];
  }
}

export function jsonLintExtensions() {
  return [linter((view) => jsonParseDiagnostics(view.state.doc.toString()), { delay: 400 }), lintGutter()];
}
