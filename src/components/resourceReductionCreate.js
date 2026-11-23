import { createAndOpen } from './reductionStart.js';
import { PRESETS as WINNER_PRESETS } from './reductionSettings.js';

// The Winner Rule presets are the review page's Settings presets, so a Reduction
// created with one is shown with that same preset rather than as Custom.
export { WINNER_PRESETS };

const CUSTOM_SLOTS = 5;

export function reductionCreateForm() {
  return {
    busy: false,
    error: '',
    scope: 'chosen',
    computeNow: true,
    matchingMode: 'both',
    preset: 'best',
    rule: Array(CUSTOM_SLOTS).fill(''),

    init() {
      // Custom starts from the rule of the preset it replaced.
      this.$watch('preset', (now, was) => {
        if (now !== 'custom' || !WINNER_PRESETS[was]) return;
        this.rule = [...WINNER_PRESETS[was], ...Array(CUSTOM_SLOTS).fill('')].slice(0, CUSTOM_SLOTS);
      });
    },

    winnerRule() {
      if (this.preset === 'custom') return this.rule.filter(Boolean);
      return WINNER_PRESETS[this.preset];
    },

    async submit(form) {
      if (this.busy) return;
      const data = new FormData(form);
      const all = this.scope === 'all';
      // Empty selectors submit an empty hidden control; it is not an entity ID.
      const resourceIds = all ? [] : data.getAll('resourceIds').filter(Boolean).map(Number);
      const groupIds = all ? [] : data.getAll('groupIds').filter(Boolean).map(Number);
      if (!all && !resourceIds.length && !groupIds.length) {
        this.error = 'Choose at least one group or resource.';
        return;
      }

      this.busy = true;
      this.error = '';
      try {
        await createAndOpen({
          name: data.get('name').trim(),
          resourceIds,
          groupIds,
          all,
          excludeExternalResources: all ? false : data.has('excludeExternalResources'),
          matchingMode: this.matchingMode,
          winnerRule: this.winnerRule(),
        }, this.computeNow);
      } catch (err) {
        this.error = err.message;
        this.busy = false;
      }
    },
  };
}
