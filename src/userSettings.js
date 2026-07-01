// src/userSettings.js
//
// Server-backed store for per-user UI preferences, replacing the browser-localStorage
// prefs (lightbox quick tags, the showDescriptions toggle, MRQL history). It talks to
// GET/PUT/DELETE /v1/account/settings so preferences follow a user across browsers and
// devices. The CSRF token is attached automatically by the global fetch wrapper
// (src/csrf.js); under auth-off the server resolves the owner to the root admin.
//
// Consumers should:  await whenLoaded();  const v = get(key);  ...on change: set(key, v)
//
// Data-loss guard (the critical piece): set() never PUTs before the initial GET has
// SUCCEEDED, so a fresh page's default state can never clobber real saved data. A key
// mutated during the load window is marked dirty; the GET response then fills only the
// keys that were NOT locally dirtied, and the dirty keys are flushed afterwards.

const DEBOUNCE_MS = 400;
const LOAD_RETRIES = 3;

// Known localStorage → server-key migrations, applied once on first successful load.
const MIGRATIONS = [
  { key: 'quickTags', legacy: 'mahresources_quickTags', recoveryPrefix: 'mahresources_quickTags_recover_' },
  { key: 'uiSettings', legacy: 'settings' },
  { key: 'mrqlHistory', legacy: 'mrql_history' },
];

const _cache = {};             // key -> parsed JS value (the source of truth in-page)
const _dirty = new Set();      // keys with local changes not yet confirmed on the server
const _timers = {};            // key -> debounce timer id
let _loaded = false;           // true only after a SUCCESSFUL GET
let _loadPromise = null;       // shared promise, resolves once the GET settles (ok or give-up)

function clone(value) {
  if (value === null || typeof value !== 'object') return value;
  try {
    return structuredClone(value);
  } catch {
    return JSON.parse(JSON.stringify(value));
  }
}

function sleep(ms) {
  return new Promise((r) => setTimeout(r, ms));
}

async function putNow(key, value, keepalive = false) {
  try {
    const res = await fetch(`/v1/account/settings/${encodeURIComponent(key)}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ value }),
      keepalive,
    });
    return res.ok;
  } catch {
    return false;
  }
}

async function flushKey(key) {
  if (!_loaded) return;
  const value = _cache[key];
  if (value === undefined) return;
  if (await putNow(key, value)) _dirty.delete(key);
}

function scheduleFlush(key) {
  clearTimeout(_timers[key]);
  _timers[key] = setTimeout(() => flushKey(key), DEBOUNCE_MS);
}

// Best-effort one-time import of legacy localStorage prefs the server does not have yet.
async function migrateLegacy() {
  for (const m of MIGRATIONS) {
    if (_cache[m.key] !== undefined) continue; // server already owns this key
    let raw = localStorage.getItem(m.legacy);
    if (!raw && m.recoveryPrefix) raw = newestRecovery(m.recoveryPrefix);
    if (!raw) continue;

    let parsed;
    try {
      parsed = JSON.parse(raw);
    } catch {
      continue; // corrupt legacy blob — skip rather than fail the whole load
    }
    if (await putNow(m.key, parsed)) {
      _cache[m.key] = parsed;
      removeLegacy(m);
    }
  }
}

function newestRecovery(prefix) {
  let bestKey = null;
  let bestSuffix = '';
  for (let i = 0; i < localStorage.length; i++) {
    const k = localStorage.key(i);
    if (k && k.startsWith(prefix)) {
      const suffix = k.slice(prefix.length);
      if (suffix > bestSuffix) {
        bestSuffix = suffix;
        bestKey = k;
      }
    }
  }
  return bestKey ? localStorage.getItem(bestKey) : null;
}

function removeLegacy(m) {
  try {
    localStorage.removeItem(m.legacy);
    if (!m.recoveryPrefix) return;
    for (let i = localStorage.length - 1; i >= 0; i--) {
      const k = localStorage.key(i);
      if (k && k.startsWith(m.recoveryPrefix)) localStorage.removeItem(k);
    }
  } catch {
    /* private mode / quota — leaving the legacy copy is harmless (server now wins) */
  }
}

async function load() {
  let serverData = null;
  for (let attempt = 0; attempt < LOAD_RETRIES; attempt++) {
    try {
      const res = await fetch('/v1/account/settings', { headers: { Accept: 'application/json' } });
      if (res.ok) {
        serverData = await res.json();
        break;
      }
      // 401/403 (auth wall or not permitted) won't fix on retry — give up quietly.
      if (res.status === 401 || res.status === 403) break;
    } catch {
      /* network error — retry */
    }
    if (attempt < LOAD_RETRIES - 1) await sleep(200 * (attempt + 1));
  }

  if (serverData && typeof serverData === 'object') {
    // Fill only keys not dirtied locally during the load window (user edits win).
    for (const [k, v] of Object.entries(serverData)) {
      if (!_dirty.has(k)) _cache[k] = v;
    }
    _loaded = true;
    await migrateLegacy();
    // Push any edits made before load settled.
    for (const key of Array.from(_dirty)) flushKey(key);
  }
  // On failure we stay unloaded: reads fall back to defaults and set() caches
  // without PUTting, so a page reload can recover without ever clobbering the server.
}

function flushAllOnHide() {
  for (const key of Array.from(_dirty)) {
    clearTimeout(_timers[key]);
    if (_loaded && _cache[key] !== undefined) putNow(key, _cache[key], true);
  }
}

if (typeof window !== 'undefined') {
  window.addEventListener('pagehide', flushAllOnHide);
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'hidden') flushAllOnHide();
  });
}

/** Kick off (once) and await the initial load. Resolves when the GET settles. */
export function whenLoaded() {
  if (!_loadPromise) _loadPromise = load();
  return _loadPromise;
}

/** Whether the initial GET succeeded (settings are authoritative). */
export function isLoaded() {
  return _loaded;
}

/** Read a setting. Returns a clone so callers can mutate freely; undefined if unset. */
export function get(key) {
  return clone(_cache[key]);
}

/**
 * Write a setting. Fire-and-forget: updates the in-page cache immediately and schedules
 * a debounced PUT. Before the initial load succeeds it only caches (never PUTs), so a
 * default value cannot overwrite real server data.
 */
export function set(key, value) {
  _cache[key] = value;
  _dirty.add(key);
  if (_loaded) scheduleFlush(key);
}

// Start loading as early as possible so consumers' whenLoaded() resolves quickly.
if (typeof window !== 'undefined') whenLoaded();

export default { whenLoaded, isLoaded, get, set };
