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
  return data && typeof data.html === 'string' ? data.html : '';
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

class LazyShortcode extends HTMLElement {
  connectedCallback() {
    if (this._init) return;
    this._init = true;
    this._token = this.getAttribute('data-token') || '';

    // JS is present, so drop the <noscript> fallback and build our own region.
    this.innerHTML = '';
    this._content = document.createElement('div');
    this._content.className = 'lazy-content';
    this.appendChild(this._content);
    this.setAttribute('aria-busy', 'true');
    renderLoading(this._content);

    const obs = lazyObserver();
    if (obs) {
      obs.observe(this);
    } else {
      // No IntersectionObserver support — render immediately.
      this._reveal();
    }
  }

  disconnectedCallback() {
    if (sharedObserver) sharedObserver.unobserve(this);
  }

  _reveal() {
    if (this._loading || this._loaded || !this._token) return;
    this._loading = true;
    fetchDeferred(this._token)
      .then((html) => {
        this._loaded = true;
        this._loading = false;
        this._content.innerHTML = html;
        hydrate(this._content);
        this.removeAttribute('aria-busy');
      })
      .catch(() => {
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
  connectedCallback() {
    if (this._init) return;
    this._init = true;
    this._token = this.getAttribute('data-token') || '';
    const summaryText = this.getAttribute('data-summary') || 'Details';
    const startOpen = this.getAttribute('data-open') === 'true';

    this.innerHTML = '';
    this._details = document.createElement('details');
    this._details.className = 'details-shortcode';
    const summary = document.createElement('summary');
    summary.textContent = summaryText;
    this._content = document.createElement('div');
    this._content.className = 'details-content';
    this._details.append(summary, this._content);
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

  _load() {
    if (this._loading || this._loaded || !this._token) return;
    this._loading = true;
    this._content.setAttribute('aria-busy', 'true');
    renderLoading(this._content);
    fetchDeferred(this._token)
      .then((html) => {
        this._loaded = true;
        this._loading = false;
        this._content.innerHTML = html;
        hydrate(this._content);
        this._content.removeAttribute('aria-busy');
      })
      .catch(() => {
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
