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

const HINTS = {
  best: 'Highest resolution first, then the largest file, then the oldest copy.',
  smallest: 'The smallest file first, then the highest resolution, then the oldest copy.',
  oldest: 'The oldest copy first, then the highest resolution, then the largest file.',
  custom: '',
};

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

export function reductionSettingsForm(rule) {
  const tokens = (rule || []).map((criterion) => criterion.token);
  return {
    preset: matchPreset(tokens),
    slots: padded(tokens, 5),
    saving: false,
    saved: false,
    error: '',

    init() {
      const count = this.$el.querySelectorAll('[data-testid="winner-rule-slot"]').length;
      this.slots = padded(tokens, count || 5);
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
      const data = new FormData(form);
      const body = {
        id: review.reductionId,
        version: review.version,
        name: data.get('name') || '',
        matchingMode: data.get('matchingMode') || 'both',
        winnerRule: this.slots.filter(Boolean),
        keepAsVersionNear: this.$refs.keepNear.checked,
        keepAsVersionIdentical: this.$refs.keepIdentical.checked,
      };
      const renamed = body.name !== form.elements.namedItem('name')?.defaultValue;
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
          window.mahAnnounce?.('This Reduction changed since you loaded it. Reloading.', { assertive: true });
          window.location.reload();
          return;
        }
        if (!response.ok) throw new Error(await window.errorMessageFromResponse(response));
        const saved = await response.json();
        review.version = saved.version;
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
      } catch (err) {
        this.error = err.message;
        window.mahAnnounce?.(`Settings not saved: ${err.message}`, { assertive: true });
      } finally {
        this.saving = false;
      }
    },
  };
}
