/**
 * The settings form of a Reduction review: name, matching mode, which copy to
 * keep, and the two keep-as-version flags.
 *
 * The preset is a client-side shorthand for the winner rule. The server stores
 * the ordered list as it always did, and the form shows whichever preset matches
 * it, or Custom with the ordered selects revealed.
 *
 * Saving is a fetch, so the page keeps its place and its state. A version
 * conflict means somebody else wrote the Reduction since this page loaded, and the
 * only correct response is to start from what is there now: it reloads.
 */

export const PRESETS = {
  best: ['pixels_desc', 'size_desc', 'created_asc'],
  smallest: ['size_asc', 'pixels_desc', 'created_asc'],
  oldest: ['created_asc', 'pixels_desc', 'size_desc'],
};

export const HINTS = {
  best: 'Highest resolution first, then the largest file, then the oldest copy.',
  smallest: 'The smallest file first, then the highest resolution, then the oldest copy.',
  oldest: 'The oldest copy first, then the highest resolution, then the largest file.',
  custom: '',
};

/**
 * The name a save sends: the typed one when the reviewer changed it, nothing
 * otherwise. The form shows the name cut to a display bound, so sending it back
 * unchanged would overwrite a longer stored name with its own truncation; the
 * server leaves the name alone when none is sent.
 */
export function nameToSend(typed, shown) {
  return typed === shown ? '' : typed;
}

export function matchPreset(tokens) {
  for (const [name, rule] of Object.entries(PRESETS)) {
    if (rule.length === tokens.length && rule.every((c, i) => c === tokens[i])) return name;
  }
  return 'custom';
}

function padded(tokens, length) {
  const out = [...tokens];
  while (out.length < length) out.push('');
  return out;
}

const CHANGED_ELSEWHERE = 'These settings were changed elsewhere. The form now shows the current values; check them and save again if you still want a change.';

function conflictKey(id) {
  return `reduction-settings-conflict-${id}`;
}

function sameSettings(a, b) {
  return JSON.stringify(a) === JSON.stringify(b);
}

export function reductionSettingsForm(rule) {
  const tokens = (rule || []).map((criterion) => criterion.token);
  return {
    preset: matchPreset(tokens),
    slots: padded(tokens, 5),
    saving: false,
    saved: false,
    error: '',
    // Said when the form was re-rendered from the server because the settings
    // changed somewhere else (another tab, another account).
    notice: '',
    // The server's values as this form last took them. The form is only ever
    // submitted against the server's current version, so it must also only ever
    // hold the server's current values, plus the reviewer's own edits.
    synced: null,

    init() {
      const count = this.$el.querySelectorAll('[data-testid="winner-rule-slot"]').length;
      this.slots = padded(tokens, count || 5);
      const review = globalThis.Alpine.store('reductionReview');
      this.synced = review.serverSettings;
      try {
        const key = conflictKey(review.reductionId || Number(document.querySelector('[data-reduction-id]')?.dataset.reductionId));
        if (window.sessionStorage.getItem(key) !== null) {
          window.sessionStorage.removeItem(key);
          this.notice = CHANGED_ELSEWHERE;
        }
      } catch {
        // No storage, no notice: the form still holds the current values.
      }
      // Settings changed elsewhere reach this store with every page read.
      this.$watch(() => review.serverSettings, () => this.reconcile());
    },

    /**
     * Re-render the form from the server when the server's settings are no longer
     * the ones the form was built on. A form left as it was could submit a stale
     * checkbox with a fresh version and silently undo someone else's change.
     * Resolves to true when it did.
     */
    reconcile(own = false) {
      const review = globalThis.Alpine.store('reductionReview');
      const server = review.serverSettings;
      if (!server || this.saving) return false;
      // This root can initialise before the page's store is seeded: its first sight of the
      // server's values is the baseline, not a change made elsewhere.
      if (!this.synced) {
        this.synced = server;
        return false;
      }
      if (sameSettings(server, this.synced)) return false;
      const form = this.$el.querySelector('form');
      if (!form) return false;
      const name = form.elements.namedItem('name');
      if (name) {
        name.value = server.name;
        name.defaultValue = server.name;
      }
      for (const radio of form.querySelectorAll('input[name="matchingMode"]')) {
        radio.checked = radio.value === server.matchingMode;
      }
      this.$refs.keepNear.checked = !!server.keepAsVersionNear;
      this.$refs.keepIdentical.checked = !!server.keepAsVersionIdentical;
      const ruleTokens = server.winnerRule || [];
      this.preset = matchPreset(ruleTokens);
      this.slots = padded(ruleTokens, this.slots.length);
      this.synced = server;
      // After this form's own save the server only normalised what was sent (a
      // duplicate criterion dropped): not a change by somebody else.
      if (!own) {
        this.saved = false;
        this.notice = CHANGED_ELSEWHERE;
        window.mahAnnounce?.(this.notice, { assertive: true });
      }
      return true;
    },

    get presetHint() {
      return HINTS[this.preset] || '';
    },

    applyPreset() {
      if (this.preset === 'custom') return;
      this.slots = padded(PRESETS[this.preset], this.slots.length);
    },

    async save(event) {
      const form = event.target;
      const review = globalThis.Alpine.store('reductionReview');
      if (this.saving) return;
      this.error = '';
      // A write whose re-read has not landed means the version below and the
      // settings above could disagree: read the page again first.
      if (review.figuresStale || review.refreshing) {
        try {
          await review.refresh();
        } catch (err) {
          this.error = `Could not confirm the current settings (${err.message}). Reload the page and try again.`;
          return;
        }
      }
      if (this.reconcile()) return;
      this.notice = '';
      const data = new FormData(form);
      const typedName = data.get('name') || '';
      const renamed = typedName !== form.elements.namedItem('name')?.defaultValue;
      const body = {
        id: review.reductionId,
        version: review.version,
        name: nameToSend(typedName, form.elements.namedItem('name')?.defaultValue),
        matchingMode: data.get('matchingMode') || 'both',
        winnerRule: this.slots.filter(Boolean),
        keepAsVersionNear: this.$refs.keepNear.checked,
        keepAsVersionIdentical: this.$refs.keepIdentical.checked,
      };
      this.saving = true;
      this.saved = false;
      this.error = '';
      try {
        const response = await fetch('/v1/reduction/edit', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
          body: JSON.stringify(body),
        });
        if (response.status === 409) {
          // The reload below ends this page: leave a word for the next one, so the
          // reviewer is told why the form no longer shows what they typed.
          try {
            window.sessionStorage.setItem(conflictKey(review.reductionId), '1');
          } catch {
            // Without storage the reload still shows the current values.
          }
          window.mahAnnounce?.('This Reduction changed since you loaded it. Reloading.', { assertive: true });
          window.location.reload();
          return;
        }
        if (!response.ok) throw new Error(await window.errorMessageFromResponse(response));
        const saved = await response.json();
        this.synced = {
          name: typedName,
          matchingMode: body.matchingMode,
          winnerRule: body.winnerRule,
          keepAsVersionNear: body.keepAsVersionNear,
          keepAsVersionIdentical: body.keepAsVersionIdentical,
        };
        // The Apply dialog's figures depend on the keep-as-version flags: they are
        // stale until the page is read again.
        review.adoptVersion(saved.version);
        if (renamed) {
          // The heading and the document title carry the name.
          window.location.reload();
          return;
        }
        this.saved = true;
        window.mahAnnounce?.('Settings saved.');
        try {
          await review.refresh();
        } catch (refreshErr) {
          this.error = `Saved, but the page could not be refreshed: ${refreshErr.message}. Reload to see the notices.`;
        }
        // The server may have normalised what was sent (duplicate criteria dropped).
        this.saving = false;
        this.reconcile(true);
      } catch (err) {
        this.error = err.message;
        window.mahAnnounce?.(`Settings not saved: ${err.message}`, { assertive: true });
      } finally {
        this.saving = false;
      }
    },
  };
}
