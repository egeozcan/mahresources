export function groupCompareView(initialState) {
  return {
    g1: initialState.g1,
    g2: initialState.g2,

    init() {
      // State is fully driven by URL params and compare page reloads.
    },

    updateUrl() {
      const url = new URL(window.location);
      url.searchParams.set('g1', this.g1);
      url.searchParams.set('g2', this.g2);
      window.location.href = url.toString();
    },

    onGroup1Change(groupId) {
      this.g1 = groupId;
      this.updateUrl();
    },

    onGroup2Change(groupId) {
      this.g2 = groupId;
      this.updateUrl();
    },

    swapSides() {
      [this.g1, this.g2] = [this.g2, this.g1];
      this.updateUrl();
    }
  };
}
