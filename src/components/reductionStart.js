/**
 * "Create or add, then find duplicates, then open the review page".
 *
 * One path for every entry point (the create page, the bulk bar, the list's
 * empty state and the list cards), so the compute-failure handling is written
 * once. A failed compute never loses the Reduction: it exists by then, so the
 * reviewer lands on its page either way. The reason travels in sessionStorage
 * under `reduction-compute-error-<id>` (never in the URL), where the review page
 * reads and clears it.
 */

import { defaultReductionName } from './reductionText.js';

export function computeErrorKey(id) {
  return `reduction-compute-error-${id}`;
}

function rememberComputeError(id, message) {
  try {
    sessionStorage.setItem(computeErrorKey(id), message);
  } catch {
    // Storage can be blocked; the review page then shows the sentence without a reason.
  }
}

/** POST /v1/reduction; resolves to {id, version, url}. Throws with the server's message. */
export async function createReduction(body) {
  const response = await fetch('/v1/reduction', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify(body),
  });
  if (!response.ok) throw new Error(await window.errorMessageFromResponse(response));
  return response.json();
}

/**
 * Starts the compute, then navigates to `url`. Never throws: a compute that
 * cannot start is remembered for the review page and the navigation still happens.
 */
export async function computeThenOpen({ id, version, url }) {
  try {
    const response = await fetch('/v1/reduction/compute', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({ id, version }),
    });
    if (!response.ok) throw new Error(await window.errorMessageFromResponse(response));
  } catch (err) {
    rememberComputeError(id, (err && err.message) || String(err));
    // A one-shot marker the page strips again: the failure itself must not depend on
    // storage that may be blocked. The reason travels in storage when it can.
    const target = new URL(url, window.location.href);
    target.searchParams.set('startFailed', '1');
    window.location.href = `${target.pathname}${target.search}${target.hash}`;
    return;
  }
  window.location.href = url;
}

/** Create (or extend) a Reduction from `body`, then either compute or just open it. */
export async function createAndOpen(body, computeNow) {
  const result = await createReduction(body);
  if (computeNow) {
    await computeThenOpen(result);
  } else {
    window.location.href = result.url;
  }
}

/**
 * The list page's "Find duplicates in everything": a whole-library Reduction
 * through the same flow.
 */
export function reductionLibraryStart() {
  return {
    busy: false,
    error: '',
    async start() {
      if (this.busy) return;
      this.busy = true;
      this.error = '';
      try {
        await createAndOpen({ all: true, name: defaultReductionName({ all: true }) }, true);
      } catch (err) {
        this.error = err.message;
        this.busy = false;
      }
    },
  };
}

/**
 * A list card's Find duplicates / Try again button: compute with the version the
 * list rendered, then open the review page.
 */
export function reductionCardAction({ id, version }) {
  return {
    busy: false,
    async start() {
      if (this.busy) return;
      this.busy = true;
      await computeThenOpen({ id, version, url: `/reduction?id=${id}` });
    },
  };
}

const BACK_VALUES = {
  status: /^(open|reviewed|skipped|stale|applied|all)$/,
  tier: /^(identical|near)$/,
  approval: /^(approved|unapproved)$/,
  attention: /^true$/,
  sort: /^(copies|size|computed)$/,
  pageSize: /^(20|50|100)$/,
  page: /^[1-9]\d{0,5}$/,
};

/**
 * The review URL to return to: only known parameters with known-shaped values, and
 * the Cluster's own heading as the anchor.
 */
export function backHref(id, params) {
  const out = new URLSearchParams({ id: String(id) });
  let carried;
  try {
    carried = new URLSearchParams(params.get('fq') || '');
  } catch {
    carried = new URLSearchParams();
  }
  for (const [key, pattern] of Object.entries(BACK_VALUES)) {
    for (const value of carried.getAll(key)) {
      if (pattern.test(value)) out.append(key, value);
    }
  }
  const anchor = /^(identical|near)-[0-9a-f]{8,64}$/.test(params.get('fa') || '') ? `#cluster-${params.get('fa')}-heading` : '';
  return `/reduction?${out.toString()}${anchor}`;
}

/**
 * The compare page's way back to the Reduction it was opened from
 * (`?from=reduction:<id>`). The id is read as an integer and the link shows only
 * once the server has agreed the viewer can open that Reduction: the review's own
 * JSON twin answers with an error for one they may not see.
 */
export function reductionBackLink() {
  return {
    href: '',
    label: '',
    async init() {
      const params = new URLSearchParams(window.location.search);
      const match = /^reduction:(\d+)$/.exec(params.get('from') || '');
      if (!match) return;
      const id = Number(match[1]);
      if (!Number.isSafeInteger(id) || id <= 0) return;
      try {
        const response = await fetch(`/reduction.json?id=${id}&pageSize=20`, { headers: { Accept: 'application/json' } });
        if (!response.ok) return;
        const page = await response.json();
        if (!page?.raw?.name) return;
        this.label = `Back to ${page.raw.name}`;
        this.href = backHref(id, params);
      } catch {
        // No link is better than a wrong one.
      }
    },
  };
}
