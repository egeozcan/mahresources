import { linter, lintGutter, forEachDiagnostic } from '@codemirror/lint';

// byteOffsetMapper returns a function that converts a UTF-8 byte offset (as
// produced by the Go linter) into a UTF-16 character index (as used by
// CodeMirror). ASCII content — the common case for templates — takes an
// identity fast path.
export function byteOffsetMapper(doc) {
  let ascii = true;
  for (let i = 0; i < doc.length; i++) {
    if (doc.charCodeAt(i) > 127) {
      ascii = false;
      break;
    }
  }
  if (ascii) {
    return (byteOffset) => Math.min(byteOffset, doc.length);
  }

  const map = new Map([[0, 0]]);
  let bytePos = 0;
  for (let i = 0; i < doc.length; ) {
    const cp = doc.codePointAt(i);
    const charLen = cp > 0xffff ? 2 : 1;
    bytePos += cp < 0x80 ? 1 : cp < 0x800 ? 2 : cp < 0x10000 ? 3 : 4;
    i += charLen;
    map.set(bytePos, i);
  }
  return (byteOffset) => {
    if (map.has(byteOffset)) return map.get(byteOffset);
    return Math.min(byteOffset, doc.length);
  };
}

function toCmSeverity(severity) {
  if (severity === 'warning') return 'warning';
  if (severity === 'info') return 'info';
  return 'error';
}

// shortcodeLintExtensions returns CodeMirror extensions that lint shortcode
// markup against the server-side linter (/v1/shortcodes/lint). The linter is
// debounced by CodeMirror (delay) and fails open — a network or server error
// yields no diagnostics rather than blocking editing.
export function shortcodeLintExtensions() {
  const source = async (view) => {
    const doc = view.state.doc.toString();
    if (!doc.trim()) return [];

    let issues = [];
    try {
      const resp = await fetch('/v1/shortcodes/lint', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ content: doc }),
      });
      if (!resp.ok) return [];
      const data = await resp.json();
      issues = data.issues || [];
    } catch (e) {
      return [];
    }

    const toChar = byteOffsetMapper(doc);
    const len = doc.length;
    return issues.map((iss) => {
      const from = Math.min(toChar(iss.start), len);
      const to = Math.min(Math.max(toChar(iss.end), from), len);
      return {
        from,
        to,
        severity: toCmSeverity(iss.severity),
        message: iss.message,
      };
    });
  };

  return [linter(source, { delay: 500 }), lintGutter()];
}

// installSubmitGuard registers view with its containing form and, once per form,
// installs a submit handler that soft-warns when any shortcode editor holds
// error-severity diagnostics. It never hard-blocks: the trust model allows
// arbitrary HTML, so false positives must not prevent saves.
export function installSubmitGuard(form, view) {
  if (!form) return;
  (form.__shortcodeViews || (form.__shortcodeViews = [])).push(view);
  if (form.__shortcodeLintGuard) return;
  form.__shortcodeLintGuard = true;

  form.addEventListener('submit', (e) => {
    let errors = 0;
    for (const v of form.__shortcodeViews) {
      try {
        forEachDiagnostic(v.state, (d) => {
          if (d.severity === 'error') errors++;
        });
      } catch (err) {
        /* view torn down — ignore */
      }
    }
    if (errors > 0) {
      const msg = `This template has ${errors} issue${errors === 1 ? '' : 's'} that may break rendering. Save anyway?`;
      if (!window.confirm(msg)) {
        e.preventDefault();
        e.stopPropagation();
      }
    }
  });
}
