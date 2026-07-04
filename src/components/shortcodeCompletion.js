// Shortcode autocomplete + hover docs for the CodeMirror template editors.
// Fed by a once-per-page-cached fetch of /v1/shortcodes/docs, it completes
// shortcode names, attribute names, and closed-enum / meta-path values, and
// shows a hover doc card over a shortcode name.
import { hoverTooltip, EditorView } from '@codemirror/view';
import { startCompletion } from '@codemirror/autocomplete';
import { htmlCompletionSource } from '@codemirror/lang-html';
import { cssCompletionSource } from '@codemirror/lang-css';

let _docsPromise = null;

// loadDocs fetches the shortcode catalogue once per page and caches it.
export function loadDocs() {
  if (!_docsPromise) {
    _docsPromise = fetch('/v1/shortcodes/docs')
      .then((r) => (r.ok ? r.json() : []))
      .catch(() => []);
  }
  return _docsPromise;
}

// For tests: reset the cache.
export function _resetDocsCache() {
  _docsPromise = null;
}

function findDoc(docs, name) {
  return docs.find((d) => d.name === name) || null;
}

// schemaPaths walks a JSON Schema string and returns dot-notation paths from
// nested `properties`. Best-effort: returns [] on invalid JSON.
function schemaPaths(schemaStr) {
  if (!schemaStr) return [];
  let schema;
  try {
    schema = JSON.parse(schemaStr);
  } catch (e) {
    return [];
  }
  const paths = [];
  const walk = (node, prefix, depth) => {
    if (!node || typeof node !== 'object' || depth > 12) return;
    const props = node.properties;
    if (props && typeof props === 'object') {
      for (const key of Object.keys(props)) {
        const path = prefix ? `${prefix}.${key}` : key;
        paths.push(path);
        walk(props[key], path, depth + 1);
      }
    }
  };
  walk(schema, '', 0);
  return paths;
}

function attrInfo(attr) {
  const bits = [];
  if (attr.required) bits.push('required');
  if (attr.type) bits.push(attr.type);
  if (attr.default) bits.push(`default: ${attr.default}`);
  const head = bits.length ? `(${bits.join(', ')}) ` : '';
  return head + (attr.description || '');
}

// shortcodeOverrideSource composes the shortcode completion with the editor's
// language completion into a single override source: inside a shortcode bracket
// it returns shortcode completions only (so HTML tag suggestions don't clutter);
// everywhere else it delegates to the html/css completion so normal editing is
// unaffected. This is used as autocompletion({ override: [...] }) — the only
// reliable way to keep the two source sets from cross-contaminating, since they
// anchor completions at different offsets.
export function shortcodeOverrideSource(mode, schemaProvider) {
  const scSource = shortcodeCompletionSource(schemaProvider);
  const langSource = mode === 'css' ? cssCompletionSource : mode === 'html' ? htmlCompletionSource : null;
  return async (context) => {
    const sc = await scSource(context);
    if (sc && sc.options && sc.options.length) return sc;
    return langSource ? langSource(context) : null;
  };
}

// shortcodeCompletionSource returns a CodeMirror completion source. schemaProvider
// is an optional () => string returning the MetaSchema JSON of the form being
// edited, used for [meta]/[conditional] path=... value completion.
export function shortcodeCompletionSource(schemaProvider) {
  return async (context) => {
    const before = context.state.sliceDoc(0, context.pos);
    const lastOpen = before.lastIndexOf('[');
    const lastClose = before.lastIndexOf(']');
    if (lastOpen < 0 || lastOpen < lastClose) {
      return null; // not inside an open bracket — let other sources handle it
    }
    const seg = before.slice(lastOpen); // "[name attr=\"v" up to cursor

    const docs = await loadDocs();
    if (!docs.length) return null;

    // 1) Completing the shortcode name: "[na" (no space yet, not a closing tag).
    const nameMatch = /^\[([a-zA-Z0-9:_-]*)$/.exec(seg);
    if (nameMatch) {
      const partial = nameMatch[1];
      const from = context.pos - partial.length;
      return {
        from,
        to: context.pos,
        options: docs.map((d) => nameOption(d)),
        validFor: /^[a-zA-Z0-9:_-]*$/,
      };
    }

    // Resolve the shortcode name for attribute/value completion.
    const scMatch = /^\[([a-zA-Z][a-zA-Z0-9:_-]*)\s/.exec(seg);
    if (!scMatch) return null;
    const doc = findDoc(docs, scMatch[1]);
    if (!doc) return null;

    // 2) Completing an attribute VALUE inside quotes: attr="par|
    const valMatch = /([\w-]+)=("|')([^"']*)$/.exec(seg);
    if (valMatch) {
      const [, attrName, , partial] = valMatch;
      const from = context.pos - partial.length;
      const values = valueOptionsFor(doc, attrName, schemaProvider);
      if (!values.length) return null;
      return {
        from,
        to: context.pos,
        options: values.map((v) => ({ label: v, type: 'enum' })),
        validFor: /^[\w.\-:]*$/,
      };
    }

    // 3) Completing an attribute NAME: after whitespace, partial word.
    const attrMatch = /\s([\w-]*)$/.exec(seg);
    if (attrMatch) {
      const partial = attrMatch[1];
      const from = context.pos - partial.length;
      const options = (doc.attrs || [])
        .filter((a) => !a.wildcard)
        .map((a) => attrOption(a));
      if (!options.length) return null;
      return {
        from,
        to: context.pos,
        options,
        validFor: /^[\w-]*$/,
      };
    }

    return null;
  };
}

function nameOption(d) {
  const isBlockRequired = d.isBlock === 'required';
  const opt = {
    label: d.name,
    type: 'keyword',
    detail: isBlockRequired ? 'block' : '',
    info: d.description || d.syntax,
  };
  if (isBlockRequired) {
    opt.apply = (view, _completion, from, to) => {
      const head = `${d.name}]\n  `;
      const insert = `${head}\n[/${d.name}]`;
      view.dispatch({
        changes: { from, to, insert },
        selection: { anchor: from + head.length },
      });
    };
  } else {
    opt.apply = `${d.name} `;
  }
  return opt;
}

function attrOption(attr) {
  return {
    label: attr.name,
    type: 'property',
    detail: attr.required ? 'required' : '',
    boost: attr.required ? 1 : 0,
    info: attrInfo(attr),
    apply: (view, _completion, from, to) => {
      const insert = `${attr.name}=""`;
      view.dispatch({
        changes: { from, to, insert },
        selection: { anchor: from + attr.name.length + 2 },
      });
    },
  };
}

function valueOptionsFor(doc, attrName, schemaProvider) {
  const attr = (doc.attrs || []).find((a) => a.name === attrName);

  // Closed enum from the registry (scope, format, boolean attrs, …).
  if (attr && Array.isArray(attr.enum) && attr.enum.length) {
    return attr.enum;
  }
  // Boolean fallback.
  if (attr && attr.type === 'boolean') {
    return ['true', 'false'];
  }
  // Meta-path completion for [meta]/[conditional]/[each] path from the live schema.
  if (attrName === 'path' && (doc.name === 'meta' || doc.name === 'conditional' || doc.name === 'each') && typeof schemaProvider === 'function') {
    return schemaPaths(schemaProvider());
  }
  return [];
}

// shortcodeAutoTrigger opens the completion popup while the caret sits inside an
// open shortcode bracket. A global languageData completion source does not
// auto-activate on typing the way a language-scoped one does, so we nudge it
// explicitly. Deferred to a microtask to avoid dispatching during an update.
export function shortcodeAutoTrigger() {
  return EditorView.updateListener.of((update) => {
    if (!update.docChanged || update.state.selection.ranges.length !== 1) return;
    if (!update.state.selection.main.empty) return;

    // Only react to inserts (typing/paste), not deletions.
    let inserted = false;
    update.changes.iterChanges((_fromA, _toA, fromB, toB) => {
      if (toB > fromB) inserted = true;
    });
    if (!inserted) return;

    const pos = update.state.selection.main.head;
    const before = update.state.sliceDoc(Math.max(0, pos - 300), pos);
    const lastOpen = before.lastIndexOf('[');
    const lastClose = before.lastIndexOf(']');
    if (lastOpen >= 0 && lastOpen > lastClose && !before.slice(lastOpen).startsWith('[/')) {
      const view = update.view;
      setTimeout(() => startCompletion(view), 0);
    }
  });
}

// shortcodeHoverTooltip returns a hover source showing a doc card over a
// shortcode name token.
export function shortcodeHoverTooltip() {
  return hoverTooltip(async (view, pos) => {
    const text = view.state.doc.toString();
    const re = /\[\/?([a-zA-Z][a-zA-Z0-9:_-]*)/g;
    let m;
    let hit = null;
    while ((m = re.exec(text)) !== null) {
      const nameStart = m.index + (m[0].startsWith('[/') ? 2 : 1);
      const nameEnd = nameStart + m[1].length;
      if (pos >= nameStart && pos <= nameEnd) {
        hit = { name: m[1], start: nameStart, end: nameEnd };
        break;
      }
    }
    if (!hit) return null;

    const docs = await loadDocs();
    const doc = findDoc(docs, hit.name);
    if (!doc) return null;

    return {
      pos: hit.start,
      end: hit.end,
      above: true,
      create() {
        return { dom: buildDocCard(doc) };
      },
    };
  });
}

function buildDocCard(doc) {
  const wrap = document.createElement('div');
  wrap.className = 'sc-hover-card';
  wrap.style.cssText = 'max-width:22rem;padding:8px 10px;font-size:12px;line-height:1.4;';

  const syntax = document.createElement('div');
  syntax.style.cssText = 'font-family:monospace;font-weight:600;margin-bottom:4px;';
  syntax.textContent = doc.syntax || `[${doc.name}]`;
  wrap.appendChild(syntax);

  if (doc.description) {
    const desc = document.createElement('div');
    desc.style.cssText = 'margin-bottom:4px;';
    desc.textContent = doc.description;
    wrap.appendChild(desc);
  }

  const attrs = (doc.attrs || []).filter((a) => !a.wildcard);
  if (attrs.length) {
    const ul = document.createElement('ul');
    ul.style.cssText = 'margin:0;padding-left:1rem;';
    attrs.forEach((a) => {
      const li = document.createElement('li');
      li.style.fontFamily = 'monospace';
      li.textContent = a.name + (a.required ? '*' : '');
      ul.appendChild(li);
    });
    wrap.appendChild(ul);
  }

  return wrap;
}
