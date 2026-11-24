import Alpine from 'alpinejs';
import { morphOptionsWithShortcodeElements } from '../utils/shortcodeElementMorph.js';
import { hoistCustomCSS } from '../utils/customCssHoist.js';
import { focusedElement, restoreFocus } from '../utils/focus.js';
import { makeModal } from './reductionModal.js';
import { computeMethods } from './reductionCompute.js';
import { shortcutMethods, readShortcutPreference } from './reductionShortcuts.js';
import { plural, CLUSTER_NOUN, COPIES } from './reductionText.js';

// The Clusters container, addressed by its own attribute rather than through
// findListContainer: that helper takes the first `.items-container` on the page,
// and this page will grow more sections.
const CLUSTERS = '[data-reduction-clusters]';
const CARD = 'article[data-cluster-id]';

// The regions the server re-renders after each action, matched by position. The
// apply bar, About and the dialogs are not among them: their open state is the
// reviewer's, and the bar reads the store.
const LIVE = '[data-reduction-live]';

// The footer pagination, addressed by its landmark label. It lives in the base
// layout, outside the body block, so a `.body` fragment never carries it — the
// refresh below fetches the full page for exactly this reason.
const PAGINATION = 'nav[aria-label="Pagination"]';

/**
 * Force the approval buttons onto the server's verdict.
 *
 * The approval toggle is a button whose state is the `aria-pressed` attribute, and
 * the server renders it; nothing here sets it optimistically. Morph patches the
 * attribute from the fresh render, so the one divergence left is a button that
 * morph matched to the wrong card, and pairing by cluster id after every morph
 * closes that too. A card advertising a decision the server never recorded is the
 * one state a review of destructive work must not show, and bfcache and form
 * restoration cannot carry it back, because the state is in no form control.
 */
function repairApprovalButtons(from, fresh) {
  const truth = new Map();
  for (const button of fresh.querySelectorAll('[data-testid="cluster-approve"]')) {
    const card = button.closest('[data-cluster-id]');
    if (card) truth.set(card.getAttribute('data-cluster-id'), button.getAttribute('aria-pressed'));
  }
  if (truth.size === 0) return;
  for (const button of from.querySelectorAll('[data-testid="cluster-approve"]')) {
    const card = button.closest('[data-cluster-id]');
    const pressed = card && truth.get(card.getAttribute('data-cluster-id'));
    if (pressed != null && button.getAttribute('aria-pressed') !== pressed) {
      button.setAttribute('aria-pressed', pressed);
    }
  }
}

/**
 * A disclosure marked data-keep-open is the reviewer's to open and close: the
 * refreshed page carries the server's default, so carry the live state over to it
 * before morph patches the attribute away.
 */
function keepOpenState(el, toEl) {
  if (el?.hasAttribute?.('data-keep-open') && toEl?.hasAttribute?.('data-keep-open')) {
    toEl.toggleAttribute('open', el.open);
  }
}

/** Where keyboard focus is among the Clusters, so a refresh can put it back. */
function el(tag, attrs = {}, children = []) {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(attrs)) {
    if (value === null || value === undefined || value === false) continue;
    if (key === 'text') node.textContent = value;
    else node.setAttribute(key, value === true ? '' : String(value));
  }
  for (const child of children) node.append(child);
  return node;
}

function humanSize(bytes) {
  if (!bytes || bytes < 1) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(units.length - 1, Math.floor(Math.log(bytes) / Math.log(1024)));
  const value = bytes / 1024 ** i;
  return `${i === 0 ? value : value.toFixed(value < 10 ? 1 : 0)} ${units[i]}`;
}

/**
 * One copy of a Cluster, built from a GET /v1/reduction/members entry. It carries
 * the same test ids, roles and names as reductionMember.tpl; the server-rendered
 * card stays the reference, and a refresh replaces these with it.
 */
function buildMemberNode(store, clusterId, member, { fixedKeep = false, readOnly = false, stale = false, merged = false, winnerChanged = false, winnerId = '', kept = null } = {}) {
  const view = { kept };
  const resource = member.Resource;
  if (!resource) {
    // Focusable (tabindex -1) so paging in a page that starts with one can still land on it.
    // A deleted copy's id marks its placeholder so a later page does not add it twice. A
    // Cluster that is not stale shows an unseen copy, whose id stays unprinted.
    const gone = el('div', { class: 'reduction-member reduction-member--gone', tabindex: '-1', 'data-testid': 'reduction-member-withheld', ...(stale ? { 'data-gone-resource-id': member.resourceId } : {}) }, [
      el('p', { class: 'text-xs text-stone-600 max-w-[10rem]', text: merged ? 'Merged and removed.' : stale ? 'Removed elsewhere.' : 'Resource unavailable.' }),
    ]);
    // A copy that is gone from a stale Cluster is left out like any other: this reopens
    // the rest for review (the server-rendered card offers the same).
    if (stale && !readOnly && !winnerChanged && !member.IsWinner && !member.ejected) {
      const eject = el('button', { type: 'button', 'data-testid': 'member-eject', class: 'reduction-btn reduction-btn--small' }, [
        "Don't remove",
        el('span', { class: 'sr-only', text: ' (the copy that was removed elsewhere)' }),
      ]);
      eject.addEventListener('click', () => store.act(clusterId, 'eject', member.resourceId));
      gone.append(eject);
    }
    return gone;
  }
  const rid = member.resourceId;
  const isWinner = !!member.IsWinner;
  const ejected = !!member.ejected;
  const role = isWinner ? ['keep', 'Keep'] : ejected ? ['ejected', 'Not included'] : ['remove', 'Remove'];
  const node = el('div', {
    class: `reduction-member ${isWinner ? 'reduction-member--keep' : ejected ? 'reduction-member--ejected' : 'reduction-member--remove'}`,
    tabindex: '-1',
    'data-testid': 'reduction-member',
    'data-resource-id': rid,
    'data-winner': isWinner,
    'data-ejected': ejected,
  });
  const roles = el('p', { class: 'reduction-roles' }, [
    el('span', { class: `reduction-badge reduction-badge--${role[0]}`, 'data-testid': 'member-role', text: role[1] }),
  ]);
  if (member.Changed) roles.append(' ', el('span', { class: 'reduction-badge reduction-badge--changed', 'data-testid': 'member-changed', text: 'This copy changed' }));
  if (member.inExtent === false) roles.append(' ', el('span', { class: 'reduction-badge reduction-badge--outside', text: 'Outside the selection' }));
  if (typeof member.distance === 'number') {
    const closeness = member.distance <= 2 ? 'Very close' : member.distance <= 6 ? 'Close' : 'Similar';
    roles.append(' ', el('span', { class: 'reduction-badge reduction-badge--distance', title: `perceptual distance ${member.distance}`, 'data-testid': 'member-distance' }, [
      closeness,
      el('span', { class: 'sr-only', text: ` (perceptual distance ${member.distance})` }),
    ]));
  }
  node.append(roles);

  const hash = resource.Hash || '';
  const link = el('a', {
    href: `/v1/resource/view?id=${resource.ID}&v=${encodeURIComponent(hash)}#${resource.ContentType}`,
    role: 'button',
    'aria-label': `Preview ${resource.Name}`,
    'data-lightbox-item': true,
    'data-resource-id': resource.ID,
    'data-content-type': resource.ContentType,
    'data-resource-name': resource.Name,
    'data-resource-hash': hash,
    'data-resource-width': resource.Width,
    'data-resource-height': resource.Height,
  });
  link.addEventListener('click', (event) => {
    event.preventDefault();
    Alpine.store('lightbox')?.openFromClick(event, resource.ID, resource.ContentType);
  });
  link.addEventListener('keydown', (event) => {
    if (event.key === ' ') {
      event.preventDefault();
      link.click();
    }
  });
  const img = el('img', { height: '160', src: `/v1/resource/preview?id=${resource.ID}&height=160&v=${encodeURIComponent(hash)}`, alt: '', loading: 'lazy', decoding: 'async' });
  const placeholder = el('span', { class: 'reduction-thumb-placeholder', 'data-testid': 'member-thumb-placeholder', hidden: true }, [
    el('span', { class: 'reduction-thumb-type', text: resource.ContentType }),
  ]);
  img.addEventListener('error', () => {
    img.hidden = true;
    placeholder.hidden = false;
  });
  link.append(img, placeholder);
  node.append(link);

  node.append(el('p', { class: 'reduction-member-name' }, [
    el('a', { class: 'reduction-member-link', href: `/resource?id=${resource.ID}`, title: resource.Name, text: resource.Name }),
  ]));

  const facts = el('ul', { class: 'reduction-facts', 'aria-label': `Details of ${resource.Name}` });
  // The same cues as the server-rendered card: a value that differs from the kept copy is
  // bold, led by a not-equal sign, and named for a screen reader.
  const compareTo = !isWinner && view.kept ? view.kept : null;
  const addFact = (text, differs = false) => {
    if (!differs) {
      facts.append(el('li', { text }));
      return;
    }
    facts.append(el('li', { class: 'reduction-fact--differs' }, [
      el('span', { 'aria-hidden': 'true', text: '\u2260 ' }),
      el('strong', { text }),
      el('span', { class: 'sr-only', text: ' (differs from the kept copy)' }),
    ]));
  };
  if (resource.Width) {
    addFact(`${resource.Width}\u00d7${resource.Height}`, !!compareTo && (String(resource.Width) !== compareTo.width || String(resource.Height) !== compareTo.height));
  }
  addFact(humanSize(resource.FileSize), !!compareTo && String(resource.FileSize) !== compareTo.size);
  addFact(resource.ContentType, !!compareTo && resource.ContentType !== compareTo.type);
  const added = String(resource.CreatedAt || '').slice(0, 10);
  if (added) addFact(`Added ${added}`, !!compareTo && !!compareTo.added && added !== compareTo.added);
  const count = (n, one) => `${n} ${one}${n === 1 ? '' : 's'}`;
  const tags = member.TagCount || 0;
  const notes = member.NoteCount || 0;
  const groups = member.GroupCount || 0;
  addFact(`${count(tags, 'tag')}, ${count(notes, 'note')}, ${count(groups, 'group')}`,
    !!compareTo && (String(tags) !== compareTo.tags || String(notes) !== compareTo.notes || String(groups) !== compareTo.groups));
  node.append(facts);

  // A Cluster that takes no decisions (read-only history) gets no controls, and a stale
  // one only "Don't remove", as the server-rendered card does: the server refuses the rest.
  if (readOnly) return node;
  const actions = el('div', { class: 'reduction-member-actions' });
  const button = (testid, label, run) => {
    const b = el('button', { type: 'button', 'data-testid': testid, class: 'reduction-btn reduction-btn--small' }, [
      label,
      el('span', { class: 'sr-only', text: ` (${resource.Name})` }),
    ]);
    b.addEventListener('click', run);
    return b;
  };
  if (stale) {
    if (!isWinner && !ejected && !winnerChanged) actions.append(button('member-eject', "Don't remove", () => store.act(clusterId, 'eject', rid)));
  } else if (!isWinner && !ejected) {
    const keep = button('member-promote', 'Keep this instead', () => store.act(clusterId, 'promote', rid));
    if (fixedKeep) {
      keep.disabled = true;
      keep.setAttribute('aria-describedby', `cluster-${clusterId}-fixed-note`);
    }
    actions.append(keep, ' ');
    // The compare page takes two Resources: this copy against the kept one that would absorb it.
    if (member.IsLoser && winnerId) {
      const compare = el('a', {
        class: 'reduction-btn reduction-btn--small',
        'data-testid': 'member-compare',
        href: `/resource/compare?r1=${winnerId}&r2=${rid}&from=reduction:${store.reductionId}`,
      }, ['Compare', el('span', { class: 'sr-only', text: ` ${resource.Name} with the kept copy` })]);
      compare.addEventListener('click', (event) => store.compareHref(event, clusterId));
      actions.append(compare, ' ');
    }
    actions.append(button('member-eject', "Don't remove", () => store.act(clusterId, 'eject', rid)));
  } else if (ejected && member.inExtent !== false) {
    actions.append(button('member-restore', 'Include again', () => store.act(clusterId, 'restore', rid)));
  }
  node.append(actions);
  return node;
}

/**
 * The kept copy's facts, as the server-rendered card carries them on its member, for the
 * comparison cues a paged copy shows. Null when the card has no kept copy to compare with.
 */
function keptFacts(card) {
  const d = card.querySelector('[data-testid="reduction-member"][data-winner="true"]')?.dataset;
  if (!d || d.keptWidth === undefined) return null;
  return {
    width: d.keptWidth, height: d.keptHeight, size: d.keptSize, type: d.keptType, added: d.keptAdded,
    tags: d.keptTags, notes: d.keptNotes, groups: d.keptGroups,
  };
}

function cardFocus() {
  const region = document.querySelector(CLUSTERS);
  const active = document.activeElement;
  if (!region || !active || !region.contains(active)) return null;
  const list = [...region.querySelectorAll(CARD)];
  const card = active.closest(CARD);
  return { index: list.indexOf(card), id: card?.dataset.clusterId, onCard: active === card };
}

/**
 * After a refresh: a card that left the view (approved under an Approved filter,
 * skipped from Open) takes focus with it. Focus goes to the card that took its
 * place, or to the previous one at the end of the page.
 */
function restoreCardFocus(before) {
  const region = document.querySelector(CLUSTERS);
  if (!before || !region || region.contains(document.activeElement)) return;
  const list = [...region.querySelectorAll(CARD)];
  const same = list.find((card) => card.dataset.clusterId === before.id);
  const target = same || list[Math.min(Math.max(before.index, 0), list.length - 1)];
  if (target) target.focus();
  else document.getElementById('reduction-clusters')?.focus();
}

/**
 * The review surface of a Resource Reduction, as a store.
 *
 * The page is two columns, the Clusters in the body and the settings in the
 * sidebar, and the two are separate Alpine roots, so the state they share (the
 * version, the counts, the apply result) cannot live in one component. A store is
 * what both roots can reach, and it is the app's established pattern for state
 * that crosses the body/sidebar split (downloads, confirmDialog, lightbox).
 *
 * Every decision is one POST carrying the version the page last saw, followed by
 * a re-render of the live regions from the server. Re-rendering rather than
 * patching the DOM in place is deliberate: a promote can eject a member, change
 * the deciding criterion and change the keeps-smaller warning all at once, and
 * keeping a second copy of that reasoning in JavaScript is how the two would
 * drift. The bulk actions are what remove the per-click cost.
 *
 * A refused write is shown, not swallowed. The one refusal that matters is the
 * version conflict (something else wrote the plan since this page loaded), and
 * the only correct response to it is to reload, because a decision made against a
 * plan that no longer exists is not a decision about anything.
 *
 * Compute, keyboard review and the dialogs' modality live in their own modules
 * and are spread in below.
 */
export function registerReductionReviewStore(Alpine) {
  Alpine.store('reductionReview', {
    reductionId: 0,
    version: 0,
    status: '',
    jobId: '',
    tab: 'open',
    // Counted by the server over the whole plan. The page can only see its own
    // Clusters, and apply acts on every page: a confirm that counts cards in this
    // DOM understates the blast radius by however many pages the reviewer has not
    // opened.
    checkedCount: 0,
    checkedLoserCount: 0,
    openCount: 0,
    figures: { identical: { clusters: 0, losers: 0, keepAsVersion: false }, near: { clusters: 0, losers: 0, keepAsVersion: false }, keepsSmaller: 0 },
    bulkFigures: { identicalApprovable: 0, identicalOpen: 0 },
    busy: false,
    // The totals above are the server's only while no write has landed since the
    // last page read. A write that lands but whose re-read fails leaves them stale:
    // Apply must not trust them (see apply()).
    figuresStale: false,
    refreshing: 0,
    // What the apply dialog was opened on: the version and the figures it shows,
    // read in one go. Apply sends exactly these back, never the live fields.
    confirmed: null,
    // The previous apply's start time, so a lost response can tell a new record from an old one.
    lastApplyStartedAt: '',
    // The digest of the exact approved proposals the figures describe; Apply is bound to it.
    applyToken: '',
    // The server's settings values as of the last page read, for the Settings form.
    serverSettings: null,
    // The counts the apply banner reports, from the response or from the plan.
    applyCounts: null,
    loadingMembers: false,
    // Bumped whenever the Clusters are re-rendered; a members response only lands if the
    // epoch it was requested under is still current.
    _membersEpoch: 0,
    _membersAbort: null,
    // Set while the outcome of an apply is not known (a lost response).
    applyUnknown: '',
    // The reviewer sent the running dialog to the background.
    backgrounded: false,
    // An apply running here, and one the plan says is running (another tab's, or a
    // lost response's). Either blocks every write.
    localApplying: false,
    serverApplying: false,
    applyingCount: 0,
    error: '',
    applyResult: null,
    bulkNotice: null,
    // The compute-start failure the create flow left in sessionStorage: a message,
    // or true when there is one but no reason to give.
    startError: '',
    progress: { text: 'Starting…', count: 0, total: 0 },
    dialog: '',
    moreOpen: false,
    shortcutsOn: true,
    // Oversized Near-Identical Clusters must be expanded before they can be acted
    // on, so a chained match cannot delete three hundred files behind one
    // approval. Keyed by cluster id; nothing is remembered across a reload,
    // deliberately: the gesture is meant to be made each time.
    expanded: {},
    // Clusters whose every copy is on the page, by "Show more members" having loaded the
    // rest. An oversized Cluster cut at the page's member budget renders only its kept
    // copy until then, and cannot be approved on that.
    allMembersShown: {},

    /** The apply bar is on screen (the Open tab, with something to apply or review). */
    get barShown() {
      return this.tab === 'open' && (this.openCount > 0 || this.checkedCount > 0);
    },

    get applying() {
      return this.localApplying || this.serverApplying;
    },

    ...computeMethods,
    ...shortcutMethods,

    /**
     * Seed the store from the server-rendered page. Called from the body root's
     * x-init, which is the first root in the document.
     *
     * Not named `init`: Alpine treats an `init` method on a store as its own
     * registration lifecycle hook and calls it with no arguments.
     */
    seed() {
      this.shortcutsOn = readShortcutPreference();
      this._readState(document);
      // The failure is the URL marker; storage only adds the reason when it is available.
      const params = new URLSearchParams(window.location.search);
      if (params.get('startFailed') === '1') {
        this.startError = true;
        try {
          const key = `reduction-compute-error-${this.reductionId}`;
          const message = window.sessionStorage.getItem(key);
          if (message) this.startError = message;
          window.sessionStorage.removeItem(key);
        } catch {
          // Blocked storage: the generic notice still shows, without a reason.
        }
        params.delete('startFailed');
        const search = params.toString();
        window.history.replaceState(window.history.state, '', `${window.location.pathname}${search ? `?${search}` : ''}${window.location.hash}`);
      }
      if (this.status === 'computing') this.watchCompute();
      this._clearBar();
      this._centerActive();
      // A refusal is shown where the reviewer is looking, not at the top of a page they have scrolled down.
      Alpine.watch?.(() => this.error, (message) => { if (message) this.surfaceError(); });
    },

    /** On a phone the tab and chip rows scroll sideways: start with the active ones in view. */
    _centerActive() {
      for (const selector of ['.reduction-tabs', '.reduction-chips']) {
        const row = document.querySelector(selector);
        const active = row?.querySelector('[aria-current]');
        if (!row || !active || row.scrollWidth <= row.clientWidth) continue;
        const offset = active.getBoundingClientRect().left - row.getBoundingClientRect().left + row.scrollLeft;
        row.scrollLeft = Math.max(0, offset - (row.clientWidth - active.offsetWidth) / 2);
      }
    },

    /**
     * Keep whatever takes focus clear of the sticky apply bar (WCAG 2.4.11). The
     * bar's height is not a constant (it wraps on a phone, and it is absent on the
     * tabs that have nothing to apply), so the page's scroll padding follows its
     * measured height. A browser scrolling a focused control into view stops that
     * much short of the viewport's bottom edge.
     */
    _clearBar() {
      const bar = document.querySelector('.reduction-bar');
      if (!bar || typeof ResizeObserver === 'undefined') return;
      const root = document.documentElement;
      const apply = () => {
        const height = bar.getBoundingClientRect().height;
        // Only a bar stuck to the viewport covers anything: below 700px tall it sits in the flow.
        const sticky = window.getComputedStyle(bar).position === 'sticky';
        root.style.scrollPaddingBottom = sticky && height > 0 ? `${Math.ceil(height) + 16}px` : '';
      };
      this._barObserver?.disconnect();
      this._barObserver = new ResizeObserver(apply);
      this._barObserver.observe(bar);
      window.addEventListener('resize', apply);
      apply();
      window.addEventListener('pagehide', () => { root.style.scrollPaddingBottom = ''; }, { once: true });
    },

    /** Take the numbers the server rendered, from the page or from a refreshed copy of it. */
    _readState(doc) {
      const node = doc.querySelector('[data-reduction-state]');
      if (!node) throw new Error('Could not find the refreshed page state');
      const state = JSON.parse(node.textContent);
      this.reductionId = state.id;
      this.version = state.version;
      this.status = state.status;
      this.tab = state.tab || 'open';
      this.jobId = state.jobId;
      this.checkedCount = state.checked;
      this.checkedLoserCount = state.checkedLosers;
      this.openCount = state.open;
      this.figures = state.figures;
      this.bulkFigures = state.bulk;
      // From the plan: another tab's apply blocks this one's, and a finished one
      // does not.
      this.serverApplying = state.applying;
      this.applyToken = state.applyToken || '';
      this.lastApplyStartedAt = state.lastApplyStartedAt || '';
      this.serverSettings = state.settings || null;
      this.figuresStale = false;
      // Any read that finds an apply running (another tab's) starts the watcher that
      // lifts the block when it finishes.
      if (this.serverApplying) this.watchApply();
    },

    /**
     * Take the version a write returned. The figures still describe the plan as it
     * was before the write, so they are stale until the page is read again; that
     * read (_readState) is the only thing that clears it.
     */
    adoptVersion(version) {
      this.version = version;
      this.figuresStale = true;
    },

    isExpanded(clusterId, oversized) {
      return !oversized || this.expanded[clusterId] === true;
    },

    /**
     * Whether a Cluster may be approved: expanded, and (for an oversized one whose page
     * cut its copies, `incomplete`) every copy rendered, so the approval is never of
     * removals the reviewer was not shown.
     */
    isReviewable(clusterId, oversized, incomplete) {
      return this.isExpanded(clusterId, oversized) && (!oversized || !incomplete || this.allMembersShown[clusterId] === true);
    },

    /** A refresh re-renders the Clusters from the server and drops the copies paging appended. */
    forgetShownMembers() {
      this.allMembersShown = {};
      // The card was re-rendered from the server, so a "Show more members" request still
      // in flight describes the card as it was: its copies would join a Cluster whose Keep
      // may be a different copy now. Abort it, and let its handler see the epoch moved.
      this._membersEpoch++;
      this._membersAbort?.abort();
      this._membersAbort = null;
    },

    expand(clusterId) {
      this.expanded[clusterId] = true;
      window.mahAnnounce?.('Cluster expanded.');
    },

    /**
     * Toggle a Cluster's approval from its button.
     *
     * Approving an oversized Near-Identical Cluster carries an explicit
     * acknowledgement, which the server requires. The flag says "this reviewer
     * expanded it first": it is not proof they looked, but it is what stops a
     * caller that is not this page approving three hundred files in one request.
     */
    toggleApproval(clusterId, oversized, event) {
      if (this.busy) return;
      const pressed = event?.currentTarget?.getAttribute('aria-pressed') === 'true';
      const extra = { acknowledgeOversized: oversized && this.expanded[clusterId] === true };
      if (extra.acknowledgeOversized) {
        // How many copies this page has rendered, which the server holds against the
        // Cluster's own count: the acknowledgement is of what was on screen.
        const card = [...document.querySelectorAll(CARD)].find((c) => c.dataset.clusterId === clusterId);
        extra.membersSeen = card
          ? card.querySelectorAll('.reduction-cluster-members [data-testid="reduction-member"], .reduction-cluster-members [data-testid="reduction-member-withheld"]').length
          : 0;
        // The kept copy the reviewer was looking at: the server refuses the approval if
        // the Cluster keeps a different one now.
        const keptId = Number(card?.querySelector('[data-testid="reduction-member"][data-winner="true"]')?.dataset.resourceId);
        if (keptId) extra.winnerId = keptId;
      }
      return this.act(clusterId, pressed ? 'uncheck' : 'check', 0, extra);
    },

    async act(clusterId, action, resourceId = 0, extra = {}) {
      if (this.busy) return;
      this.busy = true;
      this.error = '';
      this.bulkNotice = null;
      const focus = cardFocus();
      try {
        const response = await fetch('/v1/reduction/cluster', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
          body: JSON.stringify({
            id: this.reductionId,
            version: this.version,
            clusterId,
            action,
            resourceId,
            ...extra,
          }),
        });
        if (!response.ok) {
          const message = await window.errorMessageFromResponse(response);
          throw new Error(message);
        }
        const json = await response.json();
        this.adoptVersion(json.version);
        await this.refresh(focus);
        window.mahAnnounce?.(ANNOUNCEMENTS[action] || 'Cluster updated.');
      } catch (err) {
        this.error = err.message;
        this._announceError(`Could not update Cluster: ${err.message}`);
        // Re-render from server truth (best effort) so the page never shows a
        // decision that will not be applied.
        try {
          await this.refresh(focus);
        } catch {
          // The action's own error stays on screen if the repair fetch fails too,
          // and the totals can no longer be vouched for.
          this.figuresStale = true;
        }
      } finally {
        this.busy = false;
      }
    },

    /**
     * Approve or clear Clusters in bulk. `scope` is 'page' (the open Clusters on
     * screen, any tier, because the reviewer has them in front of them) or
     * 'identical' (every open Identical Cluster in the plan, which carries the
     * number on the button so the server can refuse a count that has moved).
     * The server never bulk-approves a Cluster that needs attention.
     */
    async bulk(action, scope) {
      if (this.busy || this.applying) return;
      const body = { id: this.reductionId, version: this.version, action };
      if (scope === 'page') {
        body.clusterIds = [...document.querySelectorAll(`${CLUSTERS} ${CARD}[data-cluster-state="open"]`)]
          .map((card) => card.dataset.clusterId);
        if (body.clusterIds.length === 0) {
          this.bulkNotice = { text: 'There are no open Clusters on this page.', needsReview: false };
          window.mahAnnounce?.(this.bulkNotice.text);
          return;
        }
      } else {
        body.allIdentical = true;
        body.expectedCount = action === 'check' ? this.bulkFigures.identicalApprovable : this.bulkFigures.identicalOpen;
      }

      this.busy = true;
      this.error = '';
      this.bulkNotice = null;
      // Keyboard focus on a button of the bulk menu is about to lose its home when the
      // menu closes (a phone): it goes back to the button that opened it.
      const fromMenu = !!document.activeElement?.closest?.('#reduction-bulk-menu');
      this.moreOpen = false;
      this._returnToMore = fromMenu;
      try {
        const response = await fetch('/v1/reduction/clusters', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
          body: JSON.stringify(body),
        });
        if (!response.ok) throw new Error(await window.errorMessageFromResponse(response));
        const json = await response.json();
        this.adoptVersion(json.version);
        const needs = json.needsReview || 0;
        const text = action === 'check'
          ? `Approved ${json.changed}.${needs ? ` ${needs} ${needs === 1 ? 'needs' : 'need'} individual review.` : ''}`
          : `Cleared ${json.changed}.`;
        const focus = cardFocus();
        await this.refresh(focus);
        this.bulkNotice = { text, needsReview: needs > 0 };
        window.mahAnnounce?.(text);
      } catch (err) {
        this.error = err.message;
        this._announceError(`Could not update Clusters: ${err.message}`);
        // A count that has moved, or a plan that has: show the truth.
        try { await this.refresh(); } catch { this.figuresStale = true; /* the action's own error stays */ }
      } finally {
        this.busy = false;
        if (this._returnToMore) {
          this._returnToMore = false;
          const more = document.querySelector('[data-testid="reduction-more"]');
          if (more && more.getClientRects().length > 0) more.focus();
        }
      }
    },

    toggleMore() {
      this.moreOpen = !this.moreOpen;
    },

    closeMore() {
      if (!this.moreOpen) return;
      this.moreOpen = false;
      document.querySelector('[data-testid="reduction-more"]')?.focus();
    },

    /**
     * Before the Compare link is followed, carry the review's filters and the Cluster
     * it came from, so the way back lands where the reviewer was. Known parameters
     * only; the compare page validates them again.
     */
    compareHref(event, clusterId) {
      const link = event.currentTarget;
      const kept = new URLSearchParams();
      const current = new URLSearchParams(window.location.search);
      for (const key of ['status', 'tier', 'approval', 'attention', 'sort', 'pageSize', 'page']) {
        for (const value of current.getAll(key)) kept.append(key, value);
      }
      const url = new URL(link.getAttribute('href'), window.location.href);
      const q = kept.toString();
      if (q) url.searchParams.set('fq', q);
      if (clusterId) url.searchParams.set('fa', clusterId);
      link.setAttribute('href', `${url.pathname}${url.search}`);
    },

    /**
     * Announce a refusal, unless the apply bar's own alert is on screen: that one is
     * a live region and says the same thing, so a second announcement is a repeat.
     */
    _announceError(text) {
      const bar = document.querySelector('[data-testid="reduction-bar-error"]');
      if (this.barShown && bar) return;
      window.mahAnnounce?.(text, { assertive: true });
    },

    /** The error as a sentence. */
    get errorText() {
      return this.error ? this.error.charAt(0).toUpperCase() + this.error.slice(1) : '';
    },

    /**
     * Bring the error into view and focus it: the one in the apply bar while the
     * bar is on screen, the one at the top otherwise.
     */
    surfaceError() {
      setTimeout(() => {
        const candidates = [...document.querySelectorAll('[data-reduction-error]')].filter((el) => el.getClientRects().length > 0);
        const target = candidates[0];
        if (!target) return;
        target.scrollIntoView({ block: 'nearest' });
        target.focus({ preventScroll: true });
      }, 0);
    },

    /** The skip link's target: the Apply button, or the bar's menu while Apply is disabled. */
    focusApply() {
      const apply = document.getElementById('reduction-apply-button');
      // Apply when it can be pressed; otherwise the bar itself, which is a labelled
      // region that takes programmatic focus (its menu button is hidden on a desktop).
      const target = apply && !apply.disabled ? apply : document.querySelector('.reduction-bar');
      if (!target || target.getClientRects().length === 0) return;
      target.scrollIntoView({ block: 'nearest' });
      target.focus();
    },

    focusSettings() {
      // Below 900px the sidebar is a collapsed disclosure; open it first.
      const disclosure = document.getElementById('sidebar-disclosure');
      if (disclosure && !disclosure.open) disclosure.open = true;
      const field = document.querySelector('#reduction-settings input[name="name"]');
      if (!field) return;
      field.scrollIntoView({ block: 'center' });
      field.focus();
    },

    // --- Dialogs -----------------------------------------------------------

    /**
     * `opener` is the element to give focus back to. Without it the event's own
     * target is used, and failing that the focused element: a key handler bound to
     * the Clusters region has the region as its currentTarget, which is not where
     * the reviewer was.
     */
    openDialog(kind, event, opener = null) {
      if (this.dialog) return;
      this._opener = opener?.nodeType === 1
        ? opener
        : (event?.currentTarget?.nodeType === 1 ? event.currentTarget : focusedElement());
      this.dialog = kind;
    },

    closeDialog() {
      if (!this.dialog) return;
      const opener = this._opener;
      this.dialog = '';
      this._opener = null;
      this._release?.();
      this._release = null;
      // Restore focus only once Alpine has torn the dialog down: moved sooner it
      // is pulled back inside, and then the subtree is removed and focus falls to
      // <body>.
      setTimeout(() => {
        restoreFocus(opener, document.getElementById('reduction-clusters'));
      }, 0);
    },

    escapeDialog() {
      // A running apply cannot be cancelled, but the dialog can be left: the
      // server finishes the batch either way.
      if (this.localApplying) {
        this.continueInBackground();
        return;
      }
      if (this.applying) return;
      this.closeDialog();
    },

    _mountDialog(root) {
      this._release?.();
      this._release = makeModal(root);
      // After the inner x-if has rendered its buttons.
      setTimeout(() => (root.querySelector('[autofocus]') || root).focus(), 0);
    },

    /** The per-tier lines of the apply dialog, from the server's figures. */
    tierSummaries() {
      const out = [];
      const figures = this.confirmed?.figures;
      if (!figures) return out;
      for (const [key, label, same] of [['identical', 'Identical', true], ['near', 'Near-Identical', false]]) {
        const tier = figures[key];
        if (!tier || tier.clusters === 0) continue;
        const clusters = plural(tier.clusters, ...CLUSTER_NOUN);
        const copies = `${plural(tier.losers, ...COPIES)} removed`;
        const files = tier.keepAsVersion
          ? 'Their files are added as versions of the kept copy.'
          : 'Their files move to the deleted-files backup unless a version still uses them.';
        out.push({
          key,
          headline: `${label}: ${clusters}, ${copies}.`,
          detail: `${same ? 'They have the same content as the kept copy. ' : ''}${files}`,
        });
      }
      return out;
    },

    keepsSmallerHref() {
      return `/reduction?id=${this.reductionId}&status=open&approval=approved&attention=true`;
    },

    // --- Apply -------------------------------------------------------------

    async apply(event) {
      if (this.busy || this.applying || this.refreshing) return;
      const opener = event?.currentTarget?.nodeType === 1 ? event.currentTarget : focusedElement();
      if (this.checkedCount === 0 && !this.figuresStale) {
        this.error = 'Approve at least one Cluster to apply.';
        return;
      }
      // The totals the dialog shows must be the ones the version it sends belongs
      // to. After a write whose re-read failed they are not, so read again first
      // and refuse to open the dialog when that fails too.
      if (this.figuresStale) {
        this.error = '';
        try {
          await this.refresh();
        } catch (err) {
          this.error = `The approved totals could not be confirmed (${err.message}), so nothing was applied. Reload the page and try again.`;
          this._announceError(this.error);
          return;
        }
        if (this.checkedCount === 0) {
          this.error = 'Approve at least one Cluster to apply.';
          return;
        }
      }
      this.confirmed = {
        version: this.version,
        token: this.applyToken,
        checked: this.checkedCount,
        losers: this.checkedLoserCount,
        figures: JSON.parse(JSON.stringify(this.figures)),
      };
      this.openDialog('apply', null, opener);
    },

    /** The text of the apply report: the response's own counts, or the plan's after a lost response. */
    applySummary() {
      const counts = this.applyCounts;
      if (!counts) return '';
      // A scoped viewer's own apply can include Clusters they can no longer see:
      // the plan then withholds the numbers.
      if (counts.hidden) return 'The apply finished.';
      const changed = counts.changed ? ` ${plural(counts.changed, ...CLUSTER_NOUN)} changed by another decision and ${counts.changed === 1 ? 'was' : 'were'} left alone.` : '';
      const errors = counts.errors ? ` ${plural(counts.errors, ...CLUSTER_NOUN)} could not be checked; try applying again.` : '';
      return `Applied ${plural(counts.applied, ...CLUSTER_NOUN)}, removed ${plural(counts.removed, ...COPIES)}.${changed}${errors}`;
    },

    /** True when the apply response's lists stopped at their cap, so the report names only some Clusters. */
    get applyListsCapped() {
      return !!this.applyCounts?.more;
    },

    async confirmApply() {
      if (this.busy || this.applying || !this.confirmed) return;
      // What the reviewer was shown, not what the store says now.
      const confirmed = this.confirmed;
      this.busy = true;
      this.localApplying = true;
      this.applyingCount = confirmed.checked;
      this.error = '';
      this.applyUnknown = '';
      this.bulkNotice = null;
      // The previous attempt's report is not this attempt's: clear it, so a refusal
      // here cannot re-announce, re-navigate or re-focus an earlier success.
      this.applyCounts = null;
      this.applyResult = null;
      this.backgrounded = false;
      const requestId = newRequestId();
      // The dialog's buttons are about to be replaced; keep focus on the dialog.
      setTimeout(() => (document.querySelector('[data-testid="reduction-apply-background"]') || document.querySelector('[data-testid="reduction-dialog"]'))?.focus(), 0);
      const send = async () => {
        const result = { outcome: null, lost: false, error: null };
        try {
          let response;
          try {
            response = await fetch('/v1/reduction/apply', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
              body: JSON.stringify(applyRequest(this.reductionId, confirmed, requestId)),
            });
          } catch {
            // No response at all: the request may or may not have been processed.
            result.lost = true;
          }
          if (response) {
            if (response.ok) {
              try {
                result.outcome = await response.json();
              } catch {
                result.lost = true;
              }
            } else if (response.status === 503 && (await isOwnUnavailableRefusal(response))) {
              // The app itself answered with one of its two 503 refusals (the Reduction
              // is being upgraded, or its indexes are still building): it refuses
              // before anything is applied. A JSON 503 from a proxy in front of an
              // upstream that accepted the request is not this, and falls through.
              throw new Error(UPGRADING_MESSAGE);
            } else if (LOST_STATUSES.has(response.status)) {
              // A gateway gave up on a request the server may still be running.
              result.lost = true;
            } else {
              throw new Error(await window.errorMessageFromResponse(response));
            }
          }
        } catch (err) {
          // The server answered and refused: nothing was applied.
          result.error = err;
        }
        return result;
      };
      // A reviewer who sends the dialog to the background no longer waits on this
      // response: the plan's own record is followed instead, so a response held up
      // by a slow proxy cannot hold the page.
      const background = new Promise((resolve) => { this._onBackground = () => resolve('background'); });
      const sending = send();
      let first = await Promise.race([sending, background]);
      this._onBackground = null;
      const quiet = first === 'background';
      let recovered = null;
      if (quiet) {
        // Keep listening to the request as well as the plan: a refusal that arrives
        // first is the answer (and is shown as one), a recovered record is the answer
        // otherwise. A response that merely failed (lost) leaves it to the plan.
        // While the request is in flight the plan's answers are provisional: only a
        // record carrying this attempt's id ends the wait early, and "unknown" is
        // never concluded before the request has itself failed without an answer.
        const cancel = { stop: false };
        let postPending = true;
        const recoveryRun = this._recoverApply(requestId, cancel, () => postPending);
        sending.then(() => { postPending = false; });
        const byRecord = recoveryRun.then((result) => (result.record ? { recovered: result } : new Promise(() => {})));
        const byRequest = sending.then(async (result) => (result.lost ? { recovered: await recoveryRun } : result));
        const second = await Promise.race([byRequest, byRecord]);
        cancel.stop = true;
        if (second.recovered) {
          first = { lost: true };
          recovered = second.recovered;
        } else {
          first = second;
        }
      }
      let { outcome = null, lost = false, error = null } = first;
      if (error) {
        this.error = error.message;
        this._announceError(`Nothing was applied: ${error.message}`);
      }

      if (lost) {
        if (!quiet) {
          this.applyUnknown = 'The result of this apply is unknown; checking…';
          window.mahAnnounce?.(this.applyUnknown, { assertive: true });
        }
        if (!recovered) recovered = await this._recoverApply(requestId);
        this.applyUnknown = '';
      }

      if (outcome) {
        this.applyResult = outcome;
        // The response's own totals: its lists stop at a cap, and "more" says so. An
        // older response has no totals and its lists are complete.
        this.applyCounts = {
          applied: outcome.appliedCount || outcome.applied?.length || 0,
          removed: outcome.destroyed || 0,
          stale: outcome.staleCount || outcome.stale?.length || 0,
          changed: outcome.changed || 0,
          errors: outcome.errors || 0,
          more: !!outcome.more,
        };
        this._nameOutcomes(this.applyResult.stale);
      } else if (recovered?.record) {
        this.applyResult = null;
        const last = recovered.record;
        this.applyCounts = { applied: last.applied || 0, removed: last.removed || 0, stale: last.stale || 0, hidden: !!last.countsHidden };
      } else if (lost) {
        this.error = recovered?.message || 'The result of this apply could not be determined. Reload the page to see what happened.';
        this._announceError(this.error);
      }

      if (this.applyCounts) {
        const stale = this.applyCounts.stale;
        window.mahAnnounce?.(
          `${this.applySummary()}${stale ? ` ${stale} could not be applied. See the report.` : ''}`,
          { assertive: true },
        );
        // Then the Open tab: what is left to do.
        window.history.replaceState(window.history.state, '', `/reduction?id=${this.reductionId}`);
      }
      // Announced before the re-render, and its failure caught separately. The
      // merge has already happened by this point; a shared catch would report
      // "Nothing was applied" over Resources that no longer exist, which is the
      // one thing a destructive surface must never say.
      try {
        await this.refresh();
      } catch (refreshErr) {
        this.error = this.applyCounts
          ? `Applied, but the page could not be refreshed: ${refreshErr.message}. Reload to see the result.`
          : this.error;
      }
      this.busy = false;
      // The dialog closes onto the report, which is where the reviewer's next
      // decision is.
      const report = document.getElementById('reduction-apply-result');
      this.closeDialog();
      this.confirmed = null;
      // Only now: until the dialog is gone it keeps showing the running view, not a dead confirm.
      this.localApplying = false;
      // After closeDialog's own focus hand-back, so the refusal is where focus ends up.
      if (this.error) setTimeout(() => this.surfaceError(), 20);
      // Focus moves to the report only for a reviewer who stayed with the dialog;
      // one who sent it to the background keeps their place.
      if (this.applyCounts && report && !this.backgrounded) {
        this._opener = null;
        setTimeout(() => report.focus(), 0);
      }
      this.backgrounded = false;
    },

    /**
     * After a response that never arrived, find out what the server did: read the
     * plan's own apply record until it carries this attempt's request id and says
     * finished (or interrupted).
     *
     * Only a record with this attempt's id is this attempt's result. A different or
     * missing id means another apply (another tab, an older server) and is never
     * reported as ours: the outcome is then unknown, and the Applied tab is where
     * to look.
     *
     * Resolves to {record} (this attempt's finished or interrupted apply) or {message}.
     */
    async _recoverApply(requestId, cancel = { stop: false }, pending = () => false) {
      let foreign = 0;
      const started = Date.now();
      for (let i = 0; !cancel.stop && Date.now() - started < RECOVER_DEADLINE_MS; i++) {
        // Every 2s for the first minute, every 10s after.
        const wait = i === 0 ? RECOVER_FIRST_MS : (Date.now() - started < 60_000 ? RECOVER_MS : RECOVER_SLOW_MS);
        await new Promise((resolve) => setTimeout(resolve, wait));
        if (cancel.stop) break;
        let last;
        try {
          const response = await fetch(`/reduction.json?id=${this.reductionId}`, { headers: { Accept: 'application/json' } });
          if (!response.ok) continue;
          last = (await response.json()).lastApply || null;
        } catch {
          continue;
        }
        if (last && last.requestId === requestId) {
          if (last.interrupted) {
            return { message: 'This apply was interrupted on the server. Applied Clusters are on the Applied tab; the rest are still approved.', record: last.finishedAt ? last : null };
          }
          if (!last.running && last.finishedAt) return { record: last };
          foreign = 0;
          continue;
        }
        // Not ours (or not yet recorded). Give the server a few reads to record ours.
        // While the request itself is still in flight no answer from the plan is final:
        // the claim may simply not be written yet. Only a request that has failed gets
        // a fresh round of reads, and each read must be one made after it failed.
        if (pending()) {
          foreign = 0;
        } else if (!(last && last.running)) {
          foreign++;
          if (foreign >= RECOVER_UNCHANGED) {
            return { message: UNKNOWN_OUTCOME };
          }
        }
      }
      return { message: 'The apply is still running on the server. Reload the page in a minute to see the result.' };
    },

    /**
     * Settle Clusters an interrupted apply left claimed ("Merging"). Merges
     * nothing and records no apply; the server refuses while one is running.
     */
    async settleOnly() {
      if (this.busy || this.applying) return;
      this.busy = true;
      this.error = '';
      try {
        const response = await fetch('/v1/reduction/apply', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
          body: JSON.stringify({ id: this.reductionId, version: this.version, settleOnly: true }),
        });
        if (!response.ok) throw new Error(await window.errorMessageFromResponse(response));
        await this.refresh();
        window.mahAnnounce?.('The interrupted apply was settled.');
      } catch (err) {
        this.error = err.message;
        this._announceError(`Could not settle the interrupted apply: ${err.message}`);
        try { await this.refresh(); } catch { this.figuresStale = true; }
      } finally {
        this.busy = false;
      }
    },

    /**
     * Name each refused Cluster by the copy it was keeping. The response carries
     * a bounded winnerName on every entry, so nothing is fetched; "a Cluster"
     * stays when the name is empty (the legacy path, or a copy the viewer may not read).
     */
    _nameOutcomes(list) {
      for (const item of list || []) {
        if (item.winnerName) item.label = item.winnerName;
      }
    },

    /**
     * Load the next page of a Cluster's members (GET /v1/reduction/members) and
     * append them to its card. Focus lands on the first copy added, and a polite
     * announcement says how many arrived. The button carries the cursor
     * (data-members-after) and is removed when the server has no more.
     */
    async loadMoreMembers(clusterId, button) {
      if (this.loadingMembers || !button) return;
      const card = button.closest(CARD);
      const list = card?.querySelector('.reduction-cluster-members');
      if (!list) return;
      this.loadingMembers = true;
      const epoch = this._membersEpoch;
      const abort = new AbortController();
      this._membersAbort = abort;
      const superseded = () => epoch !== this._membersEpoch || !button.isConnected || !card.isConnected;
      // The card as this request saw it: the version, and the copy it kept.
      const requestedWinner = card.querySelector('[data-testid="reduction-member"][data-winner="true"]')?.dataset.resourceId || '';
      // Focus belongs to the reviewer: it moves to the new copies only if it is still on
      // the trigger (or was dropped to the page by disabling it), not if they tabbed elsewhere.
      const focusUnmoved = () => {
        const active = document.activeElement;
        return !active || active === document.body || active === button;
      };
      button.setAttribute('aria-disabled', 'true');
      button.disabled = true;
      this.error = '';
      try {
        const url = new URL('/v1/reduction/members', window.location.origin);
        url.searchParams.set('id', this.reductionId);
        url.searchParams.set('cluster', clusterId);
        const after = button.dataset.membersAfter || '';
        if (after) url.searchParams.set('after', after);
        // The server refuses a page for a Reduction or a kept copy that is no longer these.
        url.searchParams.set('version', String(card.dataset.clusterVersion || this.version));
        if (requestedWinner) url.searchParams.set('winner', requestedWinner);
        const response = await fetch(url, { headers: { Accept: 'application/json' }, signal: abort.signal });
        if (superseded()) return;
        if (response.status === 503) throw new Error(UPGRADING_MESSAGE);
        if (!response.ok) throw new Error(await window.errorMessageFromResponse(response));
        const page = await response.json();
        if (superseded()) return;
        const present = new Set([
          ...[...list.querySelectorAll('[data-testid="reduction-member"]')].map((m) => m.dataset.resourceId),
          ...[...list.querySelectorAll('[data-gone-resource-id]')].map((m) => m.dataset.goneResourceId),
        ]);
        const view = {
          fixedKeep: card.dataset.clusterFixedKeep === 'true',
          // An applied Cluster takes no decisions: the server-rendered card gives its copies no controls.
          readOnly: !!card.querySelector('[data-cluster-read-only="true"]') || card.dataset.clusterState === 'applied',
          merged: card.dataset.clusterMerged === 'true',
          stale: card.dataset.clusterState === 'stale',
          winnerChanged: card.dataset.clusterWinnerChanged === 'true',
          winnerId: card.querySelector('[data-testid="reduction-member"][data-winner="true"]')?.dataset.resourceId || '',
          kept: keptFacts(card),
        };
        const added = [];
        for (const member of page.members || []) {
          if (present.has(String(member.resourceId))) continue;
          present.add(String(member.resourceId));
          const node = buildMemberNode(this, clusterId, member, view);
          list.append(node);
          added.push(node);
        }
        const next = page.after || '';
        // Placeholders (a copy merged away, removed elsewhere, or unseen) are members
        // the server counted in "shown" too: leaving them out showed "1 of 340" after
        // a page of 199 of them.
        const shown = list.querySelectorAll('[data-testid="reduction-member"], [data-testid="reduction-member-withheld"]').length;
        const total = Number(card.dataset.memberCount) || shown;
        const note = card.querySelector('[data-testid="cluster-shown-count"]');
        if (note) note.textContent = shown >= total ? `Showing all ${total} copies.` : `Showing ${shown} of ${total} copies.`;
        window.mahAnnounce?.(added.length ? `Showed ${plural(added.length, 'more copy', 'more copies')}.` : 'No more copies to show.');
        if (next) {
          button.dataset.membersAfter = next;
          button.removeAttribute('aria-disabled');
          button.disabled = false;
          if (focusUnmoved()) (added[0] || button).focus();
        } else {
          const unmoved = focusUnmoved();
          button.closest('[data-testid="cluster-more-members-wrap"]')?.remove();
          this.allMembersShown = { ...this.allMembersShown, [clusterId]: true };
          // The trigger is gone: the first copy added, else whatever focusable the added
          // copies carry, else the Cluster itself, so focus never falls to the page.
          if (unmoved) (added.find((n) => n.hasAttribute('tabindex')) || card).focus();
        }
      } catch (err) {
        if (superseded() || err?.name === 'AbortError') return;
        this.error = err.message;
        this._announceError(`Could not load more copies: ${err.message}`);
        button.removeAttribute('aria-disabled');
        button.disabled = false;
        if (focusUnmoved()) button.focus();
      } finally {
        if (this._membersAbort === abort) {
          this._membersAbort = null;
          this.loadingMembers = false;
        } else if (!this._membersAbort) {
          this.loadingMembers = false;
        }
      }
    },

    /** Close the running dialog without cancelling anything: the server keeps going. */
    continueInBackground() {
      // Only an apply this page started can be sent to the background.
      if (!this.localApplying) return;
      this.backgrounded = true;
      this._onBackground?.();
      window.mahAnnounce?.('The apply continues in the background. This page updates when it finishes.');
      this.closeDialog();
    },

    /**
     * An apply the plan says is running that this page did not start (another tab's,
     * or this one's before a reload): read the record until it finishes, then reload
     * to show the result.
     */
    watchApply() {
      if (this._applyWatch) return;
      const started = Date.now();
      let failures = 0;
      const stop = (message) => {
        clearInterval(this._applyWatch);
        this._applyWatch = null;
        if (message) {
          this.error = message;
          this._announceError(message);
        }
      };
      this._applyWatch = setInterval(async () => {
        // The same ceiling as recovering a lost response.
        if (Date.now() - started > RECOVER_DEADLINE_MS) {
          stop(WATCH_FAILED);
          return;
        }
        try {
          const response = await fetch(`/reduction.json?id=${this.reductionId}`, { headers: { Accept: 'application/json' } });
          if (response.status === 403 || response.status === 404) {
            stop(WATCH_FAILED);
            return;
          }
          if (!response.ok) throw new Error(String(response.status));
          const last = (await response.json()).lastApply;
          failures = 0;
          if (last && last.running && !last.interrupted) return;
        } catch {
          failures++;
          if (failures >= 3) stop(WATCH_FAILED);
          return;
        }
        stop();
        window.location.reload();
      }, APPLY_WATCH_MS);
    },

    /**
     * Re-render the live regions from the server.
     *
     * The full page, not the `.body` fragment: the footer pagination lives in
     * the base layout, and an action under a filter can change how many Clusters
     * match, so the pagination has to come back with everything else. The server
     * render cost is identical either way: the context is built fully for a
     * `.body` request too.
     *
     * Morphing rather than replacing keeps focus where the reviewer left it,
     * which matters a great deal here: the whole review is meant to be operable
     * from the keyboard.
     */
    async refresh(focus = null) {
      this.refreshing++;
      try {
        await this._refresh(focus);
      } finally {
        this.refreshing--;
      }
    },

    async _refresh(focus) {
      const response = await fetch(window.location.href, { headers: { Accept: 'text/html' } });
      if (response.status === 503) throw new Error(UPGRADING_MESSAGE);
      if (!response.ok) throw new Error(`Could not refresh the Clusters: ${response.status}`);

      // An out-of-range page (one an action just emptied) is 302'd to the last
      // valid page, and the redirected response IS that page. Morph it in and
      // adopt its URL rather than unloading: a reload would clear the apply
      // report this page exists to show.
      const adopted = new URL(response.url);
      const refreshed = new DOMParser().parseFromString(await response.text(), 'text/html');

      const current = [...document.querySelectorAll(LIVE)];
      const next = [...refreshed.querySelectorAll(LIVE)];
      if (current.length === 0 || current.length !== next.length) throw new Error('Could not find the refreshed Clusters');
      const before = focus || cardFocus();
      current.forEach((region, i) => {
        hoistCustomCSS(next[i]);
        Alpine.morph(region, next[i], morphOptionsWithShortcodeElements({ updating: keepOpenState }));
        repairApprovalButtons(region, next[i]);
      });

      // The version travels in the page too, so a refresh from any other cause
      // leaves this store agreeing with what was rendered.
      this._readState(refreshed);
      // The morph dropped any copies "Show more members" had appended.
      this.forgetShownMembers();

      // The footer pagination, which the base layout renders and the review
      // action just made stale. Three transitions are possible: morph the new
      // nav in over the old one, remove a nav the refreshed page no longer has
      // (the match set dropped to a single page), or insert a nav where there was
      // none (Apply moved Clusters into the filtered status, creating a
      // multi-page result). The include sits first in the footer, so that is
      // where an inserted nav belongs.
      const footer = document.querySelector('footer.footer');
      const nav = document.querySelector(PAGINATION);
      const nextNav = refreshed.querySelector(PAGINATION);
      if (nav && nextNav) {
        Alpine.morph(nav, nextNav, morphOptionsWithShortcodeElements());
      } else if (nav && !nextNav) {
        nav.remove();
      } else if (!nav && nextNav && footer) {
        footer.insertBefore(nextNav, footer.firstChild);
      }

      // Adopt the server's verdict on which page is current: an out-of-range
      // page was redirected to the last valid one, and the URL must match what
      // is rendered. replaceState so nothing unloads: the apply report and the
      // reviewer's scroll survive. The entry's state is kept: the media viewer's
      // marker may be on it, and without that marker closing the viewer would
      // leave its entry behind for Back to step through.
      if (adopted.pathname + adopted.search !== window.location.pathname + window.location.search) {
        history.replaceState(history.state, '', adopted.pathname + adopted.search);
      }

      restoreCardFocus(before);
    },
  });
}

// Statuses a gateway answers with when it gave up on a request the server may
// still be running; they say nothing about what the server did.
// The two refusals the server answers 503 for before it starts (error_status.go:
// ErrReductionUpgrading, ErrReductionIndexesBuilding). HandleError's JSON body is
// {"error": message} with no code, so the message is what identifies them.
const OWN_UNAVAILABLE_ERRORS = [
  'this Resource Reduction is being upgraded; try again shortly',
  'indexes are still being built; try again shortly',
];

async function isOwnUnavailableRefusal(response) {
  if (!/json/i.test(response.headers?.get?.('Content-Type') || '')) return false;
  try {
    const body = JSON.parse(await response.text());
    return typeof body?.error === 'string' && OWN_UNAVAILABLE_ERRORS.includes(body.error);
  } catch {
    return false;
  }
}

const UPGRADING_MESSAGE = 'This Reduction is being upgraded, try again shortly.';
const LOST_STATUSES = new Set([408, 502, 503, 504, 524]);
const RECOVER_FIRST_MS = 1000;
const RECOVER_MS = 2000;
// The apply's own ceiling on the server.
const RECOVER_DEADLINE_MS = 30 * 60 * 1000;
const RECOVER_SLOW_MS = 10000;
const RECOVER_UNCHANGED = 4;
const APPLY_WATCH_MS = 3000;
const WATCH_FAILED = "Could not check the apply's progress. Reload the page.";
const UNKNOWN_OUTCOME = 'The result of this apply is unknown: the server has no record of this attempt, and another apply may have run. Check the Applied tab before applying again.';

/** A random id for one apply attempt, echoed in the plan's apply record. */
function newRequestId() {
  if (globalThis.crypto?.randomUUID) return globalThis.crypto.randomUUID();
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}${Math.random().toString(36).slice(2)}`;
}

/**
 * The apply request. The version and the figures are the ones the dialog showed,
 * so the server can refuse when the plan has moved on since the reviewer saw them.
 */
function applyRequest(id, confirmed, requestId) {
  return { id, version: confirmed.version, applyToken: confirmed.token, requestId };
}

const ANNOUNCEMENTS = {
  promote: 'Kept copy changed. Copies that do not match it were left out.',
  eject: 'Copy left out of the Cluster. It will be kept.',
  restore: 'Copy included again.',
  skip: 'Cluster skipped. Excluded from applying and recomputing.',
  reopen: 'Cluster reopened.',
  check: 'Cluster approved for the next apply.',
  uncheck: 'Cluster approval cleared.',
};
