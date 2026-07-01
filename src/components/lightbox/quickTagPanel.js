// src/components/lightbox/quickTagPanel.js

import { abortableFetch } from '../../index.js';
import * as userSettings from '../../userSettings.js';

const TAB_LABELS = [
  { name: 'QUICK 1', key: 'Z' },
  { name: 'QUICK 2', key: 'X' },
  { name: 'QUICK 3', key: 'C' },
  { name: 'QUICK 4', key: 'V' },
  { name: 'RECENT',  key: 'B' },
];

function padArray(arr, len) {
  const result = (arr || []).slice(0, len);
  while (result.length < len) result.push(null);
  return result;
}

/**
 * Quick tag panel state/methods for the lightbox store.
 * All methods use `this` which is bound to the Alpine store.
 */
export const quickTagPanelState = {
  quickTagPanelOpen: false,
  activeTab: 0, // 0-3=QUICK, 4=RECENT
  quickSlots: [
    Array(9).fill(null),
    Array(9).fill(null),
    Array(9).fill(null),
    Array(9).fill(null),
  ],
  _quickTagTogglingSlot: null,
  editingSlotIndex: null,
  expandedSlotIndex: null,
  _longPressTimer: null,
  _longPressThreshold: 400,
  _longPressSlotIdx: null, // tracks which slot started the long press (for progress bar)
  _expandedClickOutsideHandler: null,
  recentTags: Array(9).fill(null),
  tabLabels: TAB_LABELS,
  // Gate persistence until the server-backed settings have hydrated the store, so a
  // fresh page's default (empty) state never overwrites the user's saved slots.
  _quickTagsReady: false,

  // ---- Batch tagging pipeline (Tier 1) ----
  // Auto-advance to the next image after a successful quick-slot add (Item 5).
  flowModeEnabled: false,
  // One-shot prefix threaded into the next announcePosition() so the flow advance
  // is announced together with the new position in a single live-region message.
  _pendingFlowPrefix: '',
  // Snapshot of the previous image's tags so R can repeat them onto the next (Item 4).
  _carryForwardTags: [],
  _carryForwardName: '',
  // In-memory undo ring of batch tag writes; each entry can be inverted even after
  // the user has navigated away from the affected image (Item 6).
  _undoRing: [],
  _undoRingMax: 20,

  // ---- Context-aware suggested tags (Tier 3) ----
  // One-tap suggestions for the current image, unioned + ranked server-side from
  // perceptual-hash-similar resources and the owner group's popular tags.
  suggestedTags: [],
  suggestedTagsLoading: false,
  // Per-resource cache so paging back to a resource paints instantly.
  _suggestedCache: new Map(),
  // Monotonic token so a late response for a previous resource can't paint.
  _suggestedReq: 0,
};

export const quickTagPanelMethods = {
  // ==================== Persistence ====================

  // Load quick/recent tags from the server-backed user-settings store. Async: waits for
  // the initial settings fetch, then hydrates the store. The v1→v3 schema migrations are
  // preserved for blobs imported from the old localStorage key. Persistence is enabled
  // only after this settles (_quickTagsReady) so a fresh page's default state can never
  // overwrite the user's saved slots before they load.
  async _loadQuickTagsFromStorage() {
    try {
      await userSettings.whenLoaded();
      const data = userSettings.get('quickTags');
      if (data && typeof data === 'object') this._applyQuickTagsData(data);
    } catch (e) {
      console.warn('Failed to load quick tags:', e);
    } finally {
      this._quickTagsReady = true;
    }
  },

  _applyQuickTagsData(data) {
    // Migration v1 → v2: flat `slots` array to nested quickSlots
    if (Array.isArray(data.slots) && !Array.isArray(data.quickSlots)) {
      data.quickSlots = [
        padArray(data.slots, 9),
        Array(9).fill(null),
        Array(9).fill(null),
      ];
      data.version = 2;
    }

    // Migration v2 → v3: single-tag slots to multi-tag arrays, 3→4 tabs
    if (!data.version || data.version < 3) {
      if (Array.isArray(data.quickSlots)) {
        // Wrap each non-null single-tag {id, name} in [{ id, name }]
        data.quickSlots = data.quickSlots.map(tab =>
          (tab || []).map(slot => slot && !Array.isArray(slot) ? [slot] : slot)
        );
        // Extend from 3 to 4 inner arrays
        while (data.quickSlots.length < 4) {
          data.quickSlots.push(Array(9).fill(null));
        }
      }
      data.version = 3;
    }

    if (Array.isArray(data.quickSlots)) {
      this.quickSlots = [
        padArray(data.quickSlots[0], 9),
        padArray(data.quickSlots[1], 9),
        padArray(data.quickSlots[2], 9),
        padArray(data.quickSlots[3], 9),
      ];
    }
    if (typeof data.flowMode === 'boolean') this.flowModeEnabled = data.flowMode;
    if (Array.isArray(data.recentTags)) this.recentTags = padArray(data.recentTags, 9);
    // drawerOpen / activeTab are deliberately NOT restored: the load is async, so applying
    // them after the user has opened the panel or switched tabs would clobber that action
    // (e.g. close a panel the user just opened). They stay transient, per-session UI state.
  },

  // Persist durable quick-tag state (fire-and-forget). Transient UI (panel open, active
  // tab) is excluded — it must not be written on every open/tab switch, and restoring it
  // across the async load would race with user interaction. No-ops until the initial load
  // has hydrated the store, so a fresh page's default state cannot overwrite saved slots.
  _saveQuickTagsToStorage() {
    if (!this._quickTagsReady) return;
    userSettings.set('quickTags', {
      version: 3,
      quickSlots: this.quickSlots,
      recentTags: this.recentTags,
      flowMode: this.flowModeEnabled,
    });
  },

  // ==================== Tab Management ====================

  switchTab(tabIndex) {
    if (tabIndex < 0 || tabIndex > 4) return;
    if (this.expandedSlotIndex !== null) {
      this.expandedSlotIndex = null;
      this._cancelLongPress();
    }
    this.activeTab = tabIndex;
    this.editingSlotIndex = null;
    this._saveQuickTagsToStorage();
    this.announce(`Switched to ${TAB_LABELS[tabIndex].name} tab`);
  },

  getActiveTabSlots() {
    if (this.activeTab < 4) return this.quickSlots[this.activeTab];
    return this.recentTags;
  },

  isQuickTab() {
    return this.activeTab < 4;
  },

  // ==================== Open / Close ====================

  openQuickTagPanel() {
    // Responsive exclusivity: close edit panel on narrow viewports
    if (window.innerWidth < 1024 && this.editPanelOpen) {
      this.closeEditPanel();
    }
    this.quickTagPanelOpen = true;
    this._saveQuickTagsToStorage();
    this.announce('Edit tags panel opened');
    // The panel narrows the media viewport — re-clamp pan so a zoomed image stays on
    // screen (BH: M7). rAF lets the new width class apply first.
    requestAnimationFrame(() => this.constrainPan());

    // Ensure resource details are loaded (reuses editPanel cache), revalidating against
    // the server so a stale cached entry is refreshed on (re)open (BH: L5).
    this.fetchResourceDetails(undefined, true);

    // Load context-aware suggestions for the current image (Tier 3).
    this.fetchSuggestedTags(undefined, true);
  },

  closeQuickTagPanel() {
    this.editingSlotIndex = null;
    this.expandedSlotIndex = null;
    this._cancelLongPress();
    this.quickTagPanelOpen = false;
    // Drop suggestions so a reopen never flashes the previous image's chips.
    this.suggestedTags = [];
    this._saveQuickTagsToStorage();
    // The media viewport widens again — re-clamp pan to the new bounds (BH: M7).
    requestAnimationFrame(() => this.constrainPan());

    // Only refresh when both panels are closed — the last panel to close triggers the refresh
    if (!this.editPanelOpen && this.needsRefreshOnClose) {
      this.needsRefreshOnClose = false;
      this.refreshPageContent();
    }

    // Clear resource details if edit panel is also closed
    if (!this.editPanelOpen) {
      if (this.detailsAborter) {
        this.detailsAborter();
        this.detailsAborter = null;
      }
      this.resourceDetails = null;
    }

    this.announce('Edit tags panel closed');
  },

  // ==================== Slot Management ====================

  addTagToSlot(index, tag) {
    if (!this.isQuickTab()) return;
    const tabIdx = this.activeTab;
    // tag = { ID: number, Name: string }
    if (!tag) return;
    const entry = { id: tag.ID, name: tag.Name };
    const current = this.quickSlots[tabIdx][index];
    if (current) {
      // Skip if tag already in slot
      if (current.some(t => t.id === tag.ID)) return;
      current.push(entry);
    } else {
      this.quickSlots[tabIdx][index] = [entry];
    }
    // Force Alpine reactivity
    this.quickSlots = [...this.quickSlots];
    // Remove from recents if this tag was there
    const recentIdx = this.recentTags.findIndex(r => r && r.id === tag.ID);
    if (recentIdx !== -1) {
      this.recentTags[recentIdx] = null;
      this.recentTags = [...this.recentTags];
    }
    this._saveQuickTagsToStorage();

    // Dismiss any open popovers in the quick-tag panel
    document.querySelectorAll('[data-quick-tag-panel] [popover]').forEach(p => {
      try { p.hidePopover(); } catch {}
    });
  },

  removeTagFromSlot(index, tagId) {
    if (!this.isQuickTab()) return;
    const tabIdx = this.activeTab;
    const current = this.quickSlots[tabIdx][index];
    if (!current) return;
    const filtered = current.filter(t => t.id !== tagId);
    this.quickSlots[tabIdx][index] = filtered.length > 0 ? filtered : null;
    this.quickSlots = [...this.quickSlots];
    this._saveQuickTagsToStorage();
  },

  clearQuickTagSlot(index) {
    if (!this.isQuickTab()) return;
    const tabIdx = this.activeTab;
    this.quickSlots[tabIdx][index] = null;
    this.quickSlots = [...this.quickSlots];
    this._saveQuickTagsToStorage();
  },

  // ==================== Recent Tags ====================

  recordRecentTag(tag) {
    // tag = { ID: number, Name: string }
    // Skip if this tag is in any quick-add slot
    if (this.quickSlots.some(slots => slots.some(s => s && s.some(t => t.id === tag.ID)))) return;

    const now = Date.now();

    // If already in recents, update ts in place
    const existingIdx = this.recentTags.findIndex(r => r && r.id === tag.ID);
    if (existingIdx !== -1) {
      this.recentTags[existingIdx] = { id: tag.ID, name: tag.Name, ts: now };
      this.recentTags = [...this.recentTags];
      this._saveQuickTagsToStorage();
      return;
    }

    // Find the position to replace: first null, or oldest ts
    let targetIdx = this.recentTags.indexOf(null);
    if (targetIdx === -1) {
      // All filled — find oldest (smallest ts)
      targetIdx = 0;
      for (let i = 1; i < this.recentTags.length; i++) {
        if (this.recentTags[i].ts < this.recentTags[targetIdx].ts) {
          targetIdx = i;
        }
      }
    }

    this.recentTags[targetIdx] = { id: tag.ID, name: tag.Name, ts: now };
    this.recentTags = [...this.recentTags];
    this._saveQuickTagsToStorage();
  },

  // ==================== Tag Toggle ====================

  isTagOnResource(tagId) {
    return (this.resourceDetails?.Tags || []).some(t => t.ID === tagId);
  },

  slotMatchState(index) {
    const slots = this.getActiveTabSlots();
    const slot = slots[index];
    if (!slot) return 'none';
    if (!this.resourceDetails) return 'none';

    // Normalize: RECENT entries are single {id, name, ts}, QUICK entries are arrays
    const tags = Array.isArray(slot) ? slot : [slot];
    if (tags.length === 0) return 'none';

    const presentCount = tags.filter(t => this.isTagOnResource(t.id ?? t.ID)).length;
    if (presentCount === tags.length) return 'all';
    if (presentCount > 0) return 'some';
    return 'none';
  },

  // Wait until resourceDetails authoritatively describes the CURRENT image before an
  // add/remove decision is read from it via slotMatchState()/isTagOnResource(). During a
  // cache-miss navigation load window resourceDetails still holds the PREVIOUS image, so a
  // decision made then can pick the wrong action — e.g. a no-op remove instead of the add the
  // user intended (finding #1, decision path). We passively poll the reactive condition rather
  // than firing our own fetchResourceDetails(): the navigation already has one in flight, and a
  // competing fetch would be aborted by onQuickTagResourceChange's follow-up call (shared
  // detailsAborter) — which is exactly what made an eager await unreliable. Display keeps using
  // the stale details (no color flash); only the decision waits. Returns true once details
  // match, false if the user navigated on or details never arrive within the budget.
  async _ensureCurrentDetailsForDecision(maxWaitMs = 3000) {
    const targetId = this.getCurrentItem()?.id;
    if (!targetId) return false;
    if (this.resourceDetails?.ID === targetId) return true;
    const step = 30;
    for (let waited = 0; waited < maxWaitMs; waited += step) {
      await new Promise(r => setTimeout(r, step));
      if (this.getCurrentItem()?.id !== targetId) return false; // navigated on — abandon
      if (this.resourceDetails?.ID === targetId) return true;
    }
    return this.resourceDetails?.ID === targetId;
  },

  async toggleTabTag(index) {
    const slots = this.getActiveTabSlots();
    const slot = slots[index];
    if (!slot) return;

    if (this._quickTagTogglingSlot === index) return;
    this._quickTagTogglingSlot = index;

    try {
      // Decide add-vs-remove only against the current image's authoritative tags — never a
      // stale load-window snapshot of the previous image (finding #1, decision path).
      if (this.resourceDetails?.ID !== this.getCurrentItem()?.id) {
        const ready = await this._ensureCurrentDetailsForDecision();
        if (!ready) {
          this.announce('Image still loading — tag not changed');
          return;
        }
      }

      // Normalize: RECENT entries are {id, name, ts}, QUICK entries are [{id, name}, ...]
      const tags = (Array.isArray(slot) ? slot : [slot]).map(t => ({
        ID: t.id ?? t.ID,
        Name: t.name ?? t.Name,
      }));

      const state = this.slotMatchState(index);

      if (state === 'all') {
        await this._batchToggleTags(tags, 'remove');
      } else {
        const missing = tags.filter(tag => !this.isTagOnResource(tag.ID));
        if (missing.length > 0) {
          const ok = await this._batchToggleTags(missing, 'add');
          // Flow mode: only advance on a confirmed add, never on remove or failure.
          if (ok && this.flowModeEnabled) this._advanceFlow(missing);
        }
      }
    } finally {
      this._quickTagTogglingSlot = null;
    }
  },

  // POST a tag add/remove with a bounded retry on transient failures. Returns the final
  // Response (whose .ok the caller checks). A 4xx is returned immediately (a client error
  // won't fix itself); only 5xx and network throws are retried, since the operation is
  // idempotent. Backoff is short and capped so the optimistic UI is not left hanging.
  async _postTagsWithRetry(endpoint, resourceId, tags, attempts = 3) {
    let lastErr = null;
    for (let attempt = 0; attempt < attempts; attempt++) {
      if (attempt > 0) {
        await new Promise(r => setTimeout(r, 150 * attempt));
      }
      const formData = new FormData();
      formData.append('ID', resourceId);
      for (const tag of tags) {
        formData.append('EditedId', tag.ID);
      }
      try {
        const response = await fetch(endpoint, {
          method: 'POST',
          body: formData,
          headers: { 'Accept': 'application/json' },
        });
        // Retry only transient server-side failures; surface client errors immediately.
        if (response.status >= 500 && attempt < attempts - 1) continue;
        return response;
      } catch (err) {
        lastErr = err;
        if (attempt < attempts - 1) continue;
        throw err;
      }
    }
    if (lastErr) throw lastErr;
  },

  // Toggle a batch of tags on a resource. Defaults to the current image, but accepts an
  // explicit targetResourceId so undo (Item 6) can invert a change on an image the user
  // has since navigated away from. Returns true on success, false on failure, so callers
  // (flow advance, undo) can gate on the result. fromUndo suppresses the undo-ring push so
  // an undo does not record its own inverse and become a toggle loop.
  async _batchToggleTags(tags, action, { targetResourceId = null, fromUndo = false } = {}) {
    const resourceId = targetResourceId ?? this.getCurrentItem()?.id;
    if (!resourceId) return false;

    const endpoint = action === 'add' ? '/v1/resources/addTags' : '/v1/resources/removeTags';

    // Only mutate the live resourceDetails optimistically when it actually describes the
    // target resource. A non-current target (cross-image undo), a write that lands after the
    // user navigated, OR the cache-miss load window where resourceDetails still holds the
    // PREVIOUS image all yield null here — we then operate server + cache only, so the
    // on-screen resource's cache entry is never poisoned with another image's data (BH: H5).
    const details = this._currentDetails(resourceId);

    // Optimistic UI update on the captured object (current target only)
    if (details) {
      if (!details.Tags) details.Tags = [];
      for (const tag of tags) {
        if (action === 'add') {
          if (!details.Tags.some(t => t.ID === tag.ID)) {
            details.Tags.push(tag);
          }
        } else {
          const idx = details.Tags.findIndex(t => t.ID === tag.ID);
          if (idx !== -1) details.Tags.splice(idx, 1);
        }
      }
    }

    try {
      // addTags/removeTags are idempotent set operations, so a transient failure is safe to
      // retry. Under a high-volume "tag 5000" workload the server can briefly return 5xx
      // (e.g. SQLite write contention / busy) or drop a connection; retrying with a short
      // backoff keeps a tag/undo from silently no-op'ing instead of bailing on the first blip.
      const response = await this._postTagsWithRetry(endpoint, resourceId, tags);

      if (!response.ok) {
        throw new Error(`Failed to ${action} tags: ${response.status}`);
      }

      if (details) {
        this.detailsCache.set(resourceId, { ...details });
      } else {
        // Non-current target: any cached copy is now stale — drop it so a later view of
        // that resource refetches the authoritative tag set.
        this.detailsCache.delete(resourceId);
      }
      this.needsRefreshOnClose = true;

      // Record an undo-ring entry for every non-undo batch write (Item 6).
      if (!fromUndo) {
        this._pushUndo({
          resourceId,
          tags,
          action,
          name: this.items.find(i => i.id === resourceId)?.name || details?.Name || 'image',
        });
      }

      const names = tags.map(t => t.Name).join(', ');
      this.announce(`${action === 'add' ? 'Added' : 'Removed'} tags: ${names}`);

      // Record each as recent tag
      if (action === 'add') {
        for (const tag of tags) {
          this.recordRecentTag(tag);
        }
      }
      return true;
    } catch (err) {
      console.error(`Failed to ${action} tags:`, err);
      // Roll back the optimistic update on the captured object (the one we mutated),
      // and drop the now-uncertain cache entry for this specific resource.
      if (details) {
        for (const tag of tags) {
          if (action === 'add') {
            const idx = details.Tags ? details.Tags.findIndex(t => t.ID === tag.ID) : -1;
            if (idx !== -1) details.Tags.splice(idx, 1);
          } else {
            if (!details.Tags) details.Tags = [];
            if (!details.Tags.some(t => t.ID === tag.ID)) details.Tags.push(tag);
          }
        }
      }
      this.detailsCache.delete(resourceId);
      this.announce(`Failed to ${action} tags`);
      return false;
    }
  },

  // ==================== Batch Pipeline: Carry-forward / Flow / Undo ====================

  // Capture the current resourceDetails' tags so the NEXT image can repeat them. Called at
  // the top of onResourceChange (before the refetch) while resourceDetails still holds the
  // just-left image. Only overwrites when the left image actually had tags, so repeat keeps
  // working across an interleaved untagged image.
  _snapshotCarryForward() {
    if (this.resourceDetails?.Tags?.length) {
      this._carryForwardTags = this.resourceDetails.Tags.map(t => ({ ID: t.ID, Name: t.Name }));
      this._carryForwardName = this.resourceDetails.Name || this.getCurrentItem()?.name || '';
    }
  },

  async repeatPreviousTags() {
    if (!this._carryForwardTags.length) {
      this.announce('No previous tags to repeat');
      return;
    }
    // Diff against the CURRENT image's authoritative tags. Wait for the navigation's own
    // details fetch to land rather than firing a competing one (which the shared detailsAborter
    // would let onQuickTagResourceChange abort, leaving isTagOnResource reading the previous
    // image — finding #1, decision path). Skip rather than repeat against stale tags.
    if (!(await this._ensureCurrentDetailsForDecision())) {
      this.announce('Image still loading — tags not repeated');
      return;
    }
    const missing = this._carryForwardTags.filter(t => !this.isTagOnResource(t.ID));
    if (!missing.length) {
      this.announce('All previous tags already applied');
      return;
    }
    const ok = await this._batchToggleTags(missing, 'add');
    // Only override _batchToggleTags' own announce with this count+source message on success;
    // under the 50ms latest-wins live region it is the one a screen reader hears. On failure
    // its "Failed to add tags" must remain the final message rather than being masked by a
    // false "Repeated…" success.
    if (ok) {
      this.announce(`Repeated ${missing.length} tag(s) from ${this._carryForwardName}`);
    }
  },

  toggleFlowMode() {
    this.flowModeEnabled = !this.flowModeEnabled;
    this._saveQuickTagsToStorage();
    this.announce(`Flow mode ${this.flowModeEnabled ? 'on' : 'off'}`);
  },

  // Advance to the next image after a flow-mode add. Threads a one-shot prefix into
  // announcePosition so the applied tag(s) and the new position are announced together
  // (the shared single-slot live region would otherwise clobber a separate message).
  _advanceFlow(addedTags) {
    const names = addedTags.map(t => t.Name).join(', ');
    if (this.currentIndex < this.items.length - 1 || this.hasNextPage) {
      this._pendingFlowPrefix = `Added ${names}. `;
      this.next();
    } else {
      this.announce(`Added ${names}. End of list`);
    }
  },

  _pushUndo(entry) {
    this._undoRing.push({
      resourceId: entry.resourceId,
      tags: entry.tags.map(t => ({ ID: t.ID, Name: t.Name })),
      action: entry.action,
      name: entry.name,
    });
    if (this._undoRing.length > this._undoRingMax) {
      this._undoRing.splice(0, this._undoRing.length - this._undoRingMax);
    }
  },

  async undoLastTagAction() {
    const entry = this._undoRing.pop();
    if (!entry) {
      this.announce('Nothing to undo');
      return;
    }
    const inverse = entry.action === 'add' ? 'remove' : 'add';
    const ok = await this._batchToggleTags(entry.tags, inverse, {
      targetResourceId: entry.resourceId,
      fromUndo: true,
    });
    if (ok) {
      const verb = inverse === 'remove' ? 'Removed' : 'Added';
      const prep = entry.action === 'add' ? 'from' : 'to';
      this.announce(`${verb} ${entry.tags.map(t => t.Name).join(', ')} ${prep} ${entry.name}`);
    } else {
      // Restore the entry so a transient failure can be retried.
      this._undoRing.push(entry);
      this.announce('Undo failed');
    }
  },

  // ==================== Resource Change Hook ====================

  onQuickTagResourceChange() {
    if (!this.quickTagPanelOpen) return;
    if (this.expandedSlotIndex !== null) {
      this.expandedSlotIndex = null;
      this._cancelLongPress();
    }
    this.fetchResourceDetails();
    // Clear immediately so the row never shows the previous image's chips while
    // the new ones load, then refetch for the resource we navigated to (Tier 3).
    this.suggestedTags = [];
    this.fetchSuggestedTags();
  },

  async focusTagEditor() {
    if (!this.quickTagPanelOpen) {
      this.openQuickTagPanel();
    }
    // Wait for resource details to load (input is inside x-if="resourceDetails")
    await this.fetchResourceDetails();
    // Try immediately (e.g. details came from cache), otherwise poll for Alpine to render
    const findAndFocus = () => {
      const panel = document.querySelector('[data-quick-tag-panel]');
      return panel?.querySelector('[data-tag-editor-input]');
    };
    const input = findAndFocus();
    if (input) {
      input.focus();
      return;
    }
    const poll = (attempts) => {
      const el = findAndFocus();
      if (el) {
        el.focus();
      } else if (attempts > 0) {
        requestAnimationFrame(() => poll(attempts - 1));
      }
    };
    requestAnimationFrame(() => poll(10));
  },

  // ==================== Suggested Tags (Tier 3) ====================

  // Fetch context-aware suggestions for a resource. Mirrors fetchResourceDetails:
  // per-resource cache for an instant paint, a monotonic token so a late response
  // for a previous resource can't paint, and a current-id guard before committing.
  // Suggestions are advisory, so any failure degrades to an empty row rather than
  // surfacing an error.
  async fetchSuggestedTags(id, forceRefresh = false) {
    const resourceId = id ?? this.getCurrentItem()?.id;
    if (!resourceId) return;

    const cached = this._suggestedCache.get(resourceId);
    if (cached) {
      if (this.getCurrentItem()?.id === resourceId) this.suggestedTags = cached;
      if (!forceRefresh) return;
    }

    const reqId = ++this._suggestedReq;
    this.suggestedTagsLoading = true;

    try {
      const { ready } = abortableFetch(`/v1/resource/suggestedTags?id=${resourceId}`);
      const response = await ready;
      if (!response.ok) throw new Error(`Failed to fetch suggested tags: ${response.status}`);

      const data = await response.json();
      const list = Array.isArray(data?.suggestions) ? data.suggestions : [];

      // A newer request has superseded this one — drop the stale result.
      if (reqId !== this._suggestedReq) return;

      this._suggestedCache.set(resourceId, list);
      if (this.getCurrentItem()?.id === resourceId) {
        this.suggestedTags = list;
      }
    } catch (err) {
      if (err.name !== 'AbortError' && reqId === this._suggestedReq && this.getCurrentItem()?.id === resourceId) {
        this.suggestedTags = [];
      }
    } finally {
      if (reqId === this._suggestedReq) this.suggestedTagsLoading = false;
    }
  },

  // Apply one suggestion to the current image. Reuses the optimistic batch-add
  // pipeline (cache write, undo ring, live-region announce), then optimistically
  // drops the chip and invalidates the resource's suggestion cache so a later
  // view refetches without the now-applied tag.
  async applySuggestedTag(tag) {
    if (!tag) return;
    const ok = await this._batchToggleTags([{ ID: tag.ID, Name: tag.Name }], 'add');
    if (!ok) return;
    this.suggestedTags = this.suggestedTags.filter(s => s.ID !== tag.ID);
    const currentId = this.getCurrentItem()?.id;
    if (currentId != null) this._suggestedCache.delete(currentId);
  },

  // Shift+1..Shift+8 apply suggestion N. Keyed on event.code (Digit/Numpad) since
  // a shifted digit reports a punctuation event.key. Bare digits 1-9 are reserved
  // for the numpad slots, so the suggested row uses the Shift layer.
  handleSuggestedTagKeydown(event) {
    if (!this.quickTagPanelOpen || event.repeat) return;
    if (!event.shiftKey || event.altKey || event.ctrlKey || event.metaKey) return;
    const m = /^(?:Digit|Numpad)([1-8])$/.exec(event.code || '');
    if (!m) return;
    const tag = this.suggestedTags[parseInt(m[1], 10) - 1];
    if (!tag) return;
    event.preventDefault();
    this.applySuggestedTag(tag);
  },

  // ==================== Numpad Layout ====================

  // Numpad visual order: top row = 7,8,9 → mid = 4,5,6 → bottom = 1,2,3
  _numpadOrder: [6, 7, 8, 3, 4, 5, 0, 1, 2],

  numpadIndex(visualIndex) {
    return this._numpadOrder[visualIndex];
  },

  quickTagKeyLabel(index) {
    // index 0-8 → '1'-'9'
    return String(index + 1);
  },

  _mediaMaxWidthClass() {
    const bothOpen = this.editPanelOpen && this.quickTagPanelOpen;
    const editOnly = this.editPanelOpen && !this.quickTagPanelOpen;
    const tagsOnly = !this.editPanelOpen && this.quickTagPanelOpen;
    if (bothOpen) return 'lg:max-w-[calc(100vw-690px)] max-w-[90vw]';
    if (editOnly || tagsOnly) return 'lg:max-w-[calc(100vw-450px)] max-w-[90vw]';
    return 'max-w-[90vw]';
  },

  // ==================== Slot Expansion ====================

  isExpanded() {
    return this.expandedSlotIndex !== null;
  },

  expandedTags() {
    if (this.expandedSlotIndex === null) return [];
    const slot = this.getActiveTabSlots()[this.expandedSlotIndex];
    if (!slot) return [];
    const tags = Array.isArray(slot) ? slot : [slot];
    return tags.slice(0, 9);
  },

  collapseExpanded() {
    if (this.expandedSlotIndex === null) return;
    this.expandedSlotIndex = null;
    this._cancelLongPress();
    if (this._expandedClickOutsideHandler) {
      document.removeEventListener('click', this._expandedClickOutsideHandler, true);
      this._expandedClickOutsideHandler = null;
    }
    this.announce('Back to quick slots');
  },

  _expandSlot(index) {
    this.expandedSlotIndex = index;
    this._longPressTimer = null;
    this._longPressSlotIdx = null;
    const tags = this.expandedTags();
    const label = this.quickTagKeyLabel(index);
    this.announce(`Expanded slot ${label}: ${tags.length} tags. Press Escape to go back.`);
  },

  _cancelLongPress() {
    if (this._longPressTimer) {
      clearTimeout(this._longPressTimer);
      this._longPressTimer = null;
    }
    this._longPressSlotIdx = null;
  },

  _slotTagCount(index) {
    const slots = this.getActiveTabSlots();
    const slot = slots[index];
    if (!slot) return 0;
    return Array.isArray(slot) ? slot.length : 1;
  },

  // ==================== Expanded Tag Toggle ====================

  async toggleExpandedTag(index) {
    const tags = this.expandedTags();
    if (index >= tags.length) return;
    const tag = tags[index];
    const tagObj = { ID: tag.id ?? tag.ID, Name: tag.name ?? tag.Name };
    // Decide add-vs-remove against the current image's authoritative tags, not a stale
    // load-window snapshot of the previous image (finding #1, decision path).
    if (this.resourceDetails?.ID !== this.getCurrentItem()?.id) {
      const ready = await this._ensureCurrentDetailsForDecision();
      if (!ready) {
        this.announce('Image still loading — tag not changed');
        return;
      }
    }
    const isOn = this.isTagOnResource(tagObj.ID);
    await this._batchToggleTags([tagObj], isOn ? 'remove' : 'add');
  },

  // ==================== Keyboard Dispatch ====================

  handleSlotKeydown(idx, event) {
    // Guard against key repeat — never act on held keys
    if (event.repeat) return;

    if (this.isExpanded()) {
      // In expanded mode: toggle individual tag at this index
      this.toggleExpandedTag(idx);
      return;
    }

    const tagCount = this._slotTagCount(idx);
    if (tagCount <= 1) {
      // Single-tag or empty: fire immediately (existing behavior)
      this.toggleTabTag(idx);
      return;
    }

    // Multi-tag: start long-press timer
    this._longPressSlotIdx = idx;
    this._longPressTimer = setTimeout(() => {
      this._expandSlot(idx);
    }, this._longPressThreshold);
  },

  handleSlotKeyup(idx) {
    if (this.isExpanded()) return; // expansion already happened

    const tagCount = this._slotTagCount(idx);
    if (tagCount <= 1) return; // already fired on keydown

    if (this._longPressTimer) {
      // Short press: cancel timer, fire batch toggle
      this._cancelLongPress();
      this.toggleTabTag(idx);
    }
  },

  // ==================== Mouse Dispatch ====================

  handleSlotMousedown(idx) {
    if (this.isExpanded()) return; // in expanded mode, click on slot cards toggles individually

    const tagCount = this._slotTagCount(idx);
    if (tagCount <= 1) return; // single-tag: normal click handler fires

    this._longPressSlotIdx = idx;
    this._longPressTimer = setTimeout(() => {
      this._expandSlot(idx);
    }, this._longPressThreshold);
  },

  handleSlotMouseup(idx) {
    if (this.isExpanded()) return;

    const tagCount = this._slotTagCount(idx);
    if (tagCount <= 1) return;

    if (this._longPressTimer) {
      this._cancelLongPress();
      this.toggleTabTag(idx);
    }
  },

  handleSlotMouseleave(idx) {
    if (this._longPressTimer) {
      this._cancelLongPress();
    }
  },

  _setupExpandedClickOutside() {
    // Called via x-effect when isExpanded() changes
    if (this._expandedClickOutsideHandler) {
      document.removeEventListener('click', this._expandedClickOutsideHandler, true);
      this._expandedClickOutsideHandler = null;
    }
    if (this.isExpanded()) {
      this._expandedClickOutsideHandler = (e) => {
        const panel = document.querySelector('[data-quick-tag-panel]');
        if (panel && !panel.contains(e.target)) {
          this.collapseExpanded();
        }
      };
      // Use capture + nextTick to avoid triggering on the same click that caused expansion
      setTimeout(() => {
        if (this._expandedClickOutsideHandler) {
          document.addEventListener('click', this._expandedClickOutsideHandler, true);
        }
      }, 0);
    }
  },
};
