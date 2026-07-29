// Custom elements backing the [lazy] and [details] category-template shortcodes.
//
// On a display page these shortcodes emit a placeholder element carrying a signed
// token instead of their rendered body; the body is fetched from
// /v1/shortcodes/deferred and injected only when the block is revealed:
//   - <lazy-shortcode>    — when it scrolls into view (IntersectionObserver)
//   - <details-shortcode> — the first time its native <details> is opened
//
// Both render into the light DOM so injected Alpine directives resolve against
// the ancestor x-data="{ entity: … }" scope the display pages provide, and the
// injected fragment is hydrated with Alpine.initTree (mirroring hoverCard.js).
// CSRF is added automatically by the fetch wrapper in src/csrf.js.

const ENDPOINT = '/v1/shortcodes/deferred';

// A single IntersectionObserver serves every <lazy-shortcode> on the page. It
// reveals slightly before the element is visible so content is ready by the time
// it scrolls in.
let sharedObserver = null;
function lazyObserver() {
  if (sharedObserver) return sharedObserver;
  if (typeof IntersectionObserver === 'undefined') return null;
  sharedObserver = new IntersectionObserver(
    (entries, obs) => {
      for (const entry of entries) {
        if (entry.isIntersecting) {
          obs.unobserve(entry.target);
          if (typeof entry.target._reveal === 'function') entry.target._reveal();
        }
      }
    },
    { rootMargin: '200px' },
  );
  return sharedObserver;
}

async function fetchDeferred(token) {
  const resp = await fetch(ENDPOINT, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token }),
  });
  if (!resp.ok) throw new Error(`deferred render failed: ${resp.status}`);
  const data = await resp.json();
  return {
    html: data && typeof data.html === 'string' ? data.html : '',
    entity: data ? data.entity : null,
  };
}

// The body was rendered against a freshly loaded entity, but the Alpine scope
// around it is the one the display page built at load. Custom templates bind to
// it directly — x-text="entity.Meta.status" is a documented way to write them —
// so without this a reload would refresh everything the server expanded and leave
// every Alpine binding beside it showing the snapshot it was asked to replace.
//
// Only a scope that already exposes an entity is touched: surfaces that never had
// one (a share page, the authoring preview) must not have one invented for them.
function applyEntity(el, entity) {
  const alpine = window.Alpine;
  if (!entity || !alpine || typeof alpine.$data !== 'function') return;
  try {
    const scope = alpine.$data(el);
    if (scope && 'entity' in scope) scope.entity = entity;
  } catch (e) {
    /* a scope we cannot reach must never break the swap */
  }
}

function hydrate(container) {
  if (window.Alpine && typeof window.Alpine.initTree === 'function') {
    try {
      window.Alpine.initTree(container);
    } catch (e) {
      /* a failed hydration must never break the page */
    }
  }
}

function renderLoading(el) {
  el.innerHTML = '<span class="deferred-loading">Loading…</span>';
}

function renderError(el, onRetry) {
  el.innerHTML = '';
  const wrap = document.createElement('div');
  wrap.className = 'deferred-error';
  wrap.setAttribute('role', 'alert');
  wrap.append('Could not load this content. ');
  const btn = document.createElement('button');
  btn.type = 'button';
  btn.className = 'deferred-retry';
  btn.textContent = 'Retry';
  btn.addEventListener('click', onRetry);
  wrap.appendChild(btn);
  el.appendChild(wrap);
}

// Only the newest write to a container may land. Two [reload] buttons in the same
// block activated in quick succession would otherwise race, and the slower
// response would win.
const reloadGenerations = new WeakMap();

// invalidateReloads strands any reloadInto still in flight for contentEl. The
// reveal path cancels a reload the same way reload() cancels a reveal through
// _requestId: without both directions, a morph-triggered reveal and a reload can
// each land on top of the other, and whichever is slower wins.
function invalidateReloads(contentEl) {
  if (!contentEl) return;
  reloadGenerations.set(contentEl, (reloadGenerations.get(contentEl) || 0) + 1);
  // The stranded attempt will never run its own cleanup, so it is settled from
  // here: its caller learns straight away that it was abandoned, instead of
  // waiting on a response nobody will use — which may be slow, or never come.
  // Dropping the entry also matters because activeReloads holds strong
  // references, and a leaked one outlives the page it belonged to.
  const entry = activeReloads.get(contentEl);
  if (!entry) return;
  activeReloads.delete(contentEl);
  entry.abandon();
}

// Containers currently being reloaded, so an overlap can be detected. A WeakMap
// cannot be walked, and this only ever holds what is in flight.
const activeReloads = new Map();

// Two reloads whose containers nest are, to the reader, the same reload: writing
// a region also rewrites every block inside it. Keyed by container alone, each
// would believe itself unopposed, and the one that answered last would win no
// matter which was asked for last — so a slow region reload could land on top of
// a block the reader refreshed afterwards, with content that predates it.
//
// The newer activation wins, whether it targets the same container or one that
// nests with it.
function supersede(contentEl) {
  for (const [other, entry] of activeReloads) {
    if (other !== contentEl && !overlaps(other, contentEl)) continue;
    invalidateReloads(other);
    if (other === contentEl) continue;
    // A different container is taking over. The abandoned reload will not clear
    // its own busy state (a superseded attempt keeps its hands off shared
    // state), and the new one is setting it on a different element, so without
    // this its container would stay marked busy for good.
    entry.host.removeAttribute('aria-busy');
    other.classList.remove('deferred-reloading');
  }
}

function overlaps(a, b) {
  return (typeof a.contains === 'function' && a.contains(b)) || (typeof b.contains === 'function' && b.contains(a));
}

// reloadInto re-fetches token and swaps the result into contentEl, keeping the
// current content on screen for the whole round trip: a refresh must not
// collapse the block to a "Loading…" line and back, and a failed one must not
// destroy what the reader was already looking at. host carries aria-busy (it is
// contentEl itself for a region, whose content is its own body).
//
// isStale is an optional predicate asking "has something else taken this content
// over while we were waiting?" A region has no reveal path to bump the generation
// counter, so it uses this to notice that a page morph re-rendered it underneath.
//
// Resolves true when the swap was applied and false when it was abandoned, so the
// caller does not report success for a result nobody saw. Rejects when the fetch
// fails.
export function reloadInto(host, contentEl, token, isStale) {
  supersede(contentEl);
  const generation = (reloadGenerations.get(contentEl) || 0) + 1;
  reloadGenerations.set(contentEl, generation);

  // Settled the moment something else takes this container over, so the caller
  // is not left waiting on a request whose result can no longer be used.
  let abandon;
  const abandoned = new Promise((resolve) => {
    abandon = () => resolve(false);
  });
  activeReloads.set(contentEl, { host, abandon });

  host.setAttribute('aria-busy', 'true');
  contentEl.classList.add('deferred-reloading');

  // A superseded reload keeps its hands off the busy state: the newer one owns it.
  const current = () => {
    if (reloadGenerations.get(contentEl) !== generation) return false;
    activeReloads.delete(contentEl);
    host.removeAttribute('aria-busy');
    contentEl.classList.remove('deferred-reloading');
    return true;
  };

  const settled = fetchDeferred(token).then(
    ({ html, entity }) => {
      if (!current() || !contentEl.isConnected) return false;
      if (typeof isStale === 'function' && isStale()) return false;
      contentEl.innerHTML = html;
      applyEntity(contentEl, entity);
      hydrate(contentEl);
      return true;
    },
    (err) => {
      // A superseded or detached attempt does not own the outcome: a newer one is
      // in flight, has already landed, or the content is gone, so reporting this
      // failure would contradict what the reader can see. isStale is the same
      // question for a region — content re-rendered underneath us — and it has to
      // be asked on this side too, or a request that was abandoned for being
      // obsolete still reports "Reload failed" over content that just arrived.
      if (!current() || !contentEl.isConnected) return false;
      if (typeof isStale === 'function' && isStale()) return false;
      throw err;
    },
  );

  // Whichever comes first: a real outcome, or the news that this attempt no
  // longer matters. A late failure on an abandoned attempt resolves false rather
  // than throwing, so nothing is left unhandled.
  return Promise.race([abandoned, settled]);
}

class LazyShortcode extends HTMLElement {
  static get observedAttributes() {
    return ['data-token'];
  }

  connectedCallback() {
    if (this._init) return;
    this._init = true;
    this._token = this.getAttribute('data-token') || '';
    this._loaded = false;
    this._loading = false;
    this._revealed = false;
    this._requestId = 0;

    // JS is present, so drop the <noscript> fallback and build our own region.
    this.innerHTML = '';
    this._content = document.createElement('div');
    this._content.className = 'lazy-content';
    this.appendChild(this._content);
    this._resetContent();
    this._observeOrReveal();
  }

  attributeChangedCallback(name, oldValue, newValue) {
    if (name !== 'data-token' || oldValue === newValue || !this._init) return;
    this._token = newValue || '';
    this._resetContent();
    if (this._revealed) this._reveal();
    else this._observeOrReveal();
  }

  disconnectedCallback() {
    this._unobserve();
  }

  refreshFromMorph(toEl) {
    if (!this._init) return;

    const nextToken = toEl?.getAttribute('data-token') || this.getAttribute('data-token') || '';
    if (nextToken !== this._token) {
      this._token = nextToken;
      this._resetContent();
      if (this._revealed) this._reveal();
      else this._observeOrReveal();
      return;
    }

    if (this._revealed || this._loaded || this._loading) {
      this._resetContent();
      this._reveal();
    } else {
      this._observeOrReveal();
    }
  }

  // Public entry point for [reload]. The block is already revealed when a button
  // inside it can be clicked, so this refreshes in place rather than re-running
  // the reveal: bump the request id to strand any reveal still in flight, then
  // swap in the fresh body.
  reload() {
    if (!this._token || !this._content) return Promise.resolve();
    this._requestId++;
    this._revealed = true;
    this._loaded = true;
    this._loading = false;
    this._unobserve();
    return reloadInto(this, this._content, this._token);
  }

  _observeOrReveal() {
    if (this._loaded || this._loading || !this._token) return;
    const obs = lazyObserver();
    if (obs) {
      obs.observe(this);
    } else {
      // No IntersectionObserver support — render immediately.
      this._reveal();
    }
  }

  _unobserve() {
    if (sharedObserver) sharedObserver.unobserve(this);
  }

  _resetContent() {
    this._requestId++;
    this._loaded = false;
    this._loading = false;
    invalidateReloads(this._content);
    this.setAttribute('aria-busy', 'true');
    if (this._content) renderLoading(this._content);
  }

  _reveal() {
    if (this._loading || this._loaded || !this._token) return;
    this._revealed = true;
    this._unobserve();
    this._loading = true;
    this.setAttribute('aria-busy', 'true');
    const token = this._token;
    const reqId = ++this._requestId;
    fetchDeferred(token)
      .then(({ html, entity }) => {
        if (reqId !== this._requestId || token !== this._token || !this.isConnected) return;
        this._loaded = true;
        this._loading = false;
        this._content.innerHTML = html;
        applyEntity(this._content, entity);
        hydrate(this._content);
        this.removeAttribute('aria-busy');
      })
      .catch(() => {
        if (reqId !== this._requestId || token !== this._token || !this.isConnected) return;
        this._loading = false;
        this.removeAttribute('aria-busy');
        renderError(this._content, () => {
          renderLoading(this._content);
          this._reveal();
        });
      });
  }
}

class DetailsShortcode extends HTMLElement {
  static get observedAttributes() {
    return ['data-token', 'data-summary'];
  }

  connectedCallback() {
    if (this._init) return;
    this._init = true;
    this._token = this.getAttribute('data-token') || '';
    const summaryText = this.getAttribute('data-summary') || 'Details';
    const startOpen = this.getAttribute('data-open') === 'true';
    this._loaded = false;
    this._loading = false;
    this._requestId = 0;

    this.innerHTML = '';
    this._details = document.createElement('details');
    this._details.className = 'details-shortcode';
    this._summary = document.createElement('summary');
    this._summary.textContent = summaryText;
    this._content = document.createElement('div');
    this._content.className = 'details-content';
    this._details.append(this._summary, this._content);
    this.appendChild(this._details);

    // Native <details> gives keyboard + screen-reader semantics for free; we only
    // defer the body, loading it the first time the disclosure opens.
    this._details.addEventListener('toggle', () => {
      if (this._details.open) this._load();
    });
    if (startOpen) {
      this._details.open = true;
      this._load();
    }
  }

  attributeChangedCallback(name, oldValue, newValue) {
    if (oldValue === newValue || !this._init) return;
    if (name === 'data-summary' && this._summary) {
      this._summary.textContent = newValue || 'Details';
      return;
    }
    if (name === 'data-token') {
      this._token = newValue || '';
      this._resetContent();
      if (this._details?.open) this._load(true);
    }
  }

  refreshFromMorph(toEl) {
    if (!this._init) return;

    const nextSummary = toEl?.getAttribute('data-summary') || this.getAttribute('data-summary') || 'Details';
    if (this._summary) this._summary.textContent = nextSummary;

    const nextToken = toEl?.getAttribute('data-token') || this.getAttribute('data-token') || '';
    if (nextToken !== this._token) {
      this._token = nextToken;
      this._resetContent();
      if (this._details?.open) this._load(true);
      return;
    }

    if (this._details?.open) {
      if (this._loading || this._loaded) this._resetContent();
      this._load(true);
    } else if (this._loaded || this._loading) {
      this._resetContent();
    }
  }

  // Public entry point for [reload]. A button inside the disclosure body is only
  // reachable once it is open and loaded, so this refreshes in place; see
  // LazyShortcode.reload. aria-busy goes on the content div, matching where
  // _load puts it for this element.
  reload() {
    if (!this._token || !this._content) return Promise.resolve();
    this._requestId++;
    this._loaded = true;
    this._loading = false;
    return reloadInto(this._content, this._content, this._token);
  }

  _resetContent() {
    this._requestId++;
    this._loaded = false;
    this._loading = false;
    invalidateReloads(this._content);
    if (this._content) {
      this._content.innerHTML = '';
      this._content.removeAttribute('aria-busy');
    }
  }

  _load(force = false) {
    if (this._loading || (!force && this._loaded) || !this._token) return;
    this._loading = true;
    this._loaded = false;
    this._content.setAttribute('aria-busy', 'true');
    renderLoading(this._content);
    const token = this._token;
    const reqId = ++this._requestId;
    fetchDeferred(token)
      .then(({ html, entity }) => {
        if (reqId !== this._requestId || token !== this._token || !this.isConnected) return;
        this._loaded = true;
        this._loading = false;
        this._content.innerHTML = html;
        applyEntity(this._content, entity);
        hydrate(this._content);
        this._content.removeAttribute('aria-busy');
      })
      .catch(() => {
        if (reqId !== this._requestId || token !== this._token || !this.isConnected) return;
        this._loading = false;
        this._content.removeAttribute('aria-busy');
        renderError(this._content, () => {
          renderLoading(this._content);
          this._load();
        });
      });
  }
}

if (!customElements.get('lazy-shortcode')) {
  customElements.define('lazy-shortcode', LazyShortcode);
}
if (!customElements.get('details-shortcode')) {
  customElements.define('details-shortcode', DetailsShortcode);
}
