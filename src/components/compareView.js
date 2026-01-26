/**
 * Alpine.js component for managing version comparison UI state.
 * Handles URL state for resource and version selection parameters.
 */
export function compareView(initialState) {
  return {
    r1: initialState.r1,
    v1: initialState.v1,
    r2: initialState.r2,
    v2: initialState.v2,

    init() {
      // Component initialized with state from URL params passed via template
    },

    /**
     * Updates the URL with current comparison state and navigates to it.
     * This triggers a full page reload to fetch new comparison data.
     */
    updateUrl() {
      const url = new URL(window.location);
      url.searchParams.set('r1', this.r1);
      url.searchParams.set('v1', this.v1);
      url.searchParams.set('r2', this.r2);
      url.searchParams.set('v2', this.v2);
      window.location.href = url.toString();
    },

    /**
     * Fetches available versions for a given resource.
     * @param {number|string} resourceId - The resource ID to fetch versions for
     * @returns {Promise<Array>} Array of version objects
     */
    async fetchVersions(resourceId) {
      const response = await fetch(`/v1/resource/versions?resourceId=${resourceId}`);
      if (!response.ok) {
        console.error('Failed to fetch versions:', response.statusText);
        return [];
      }
      return response.json();
    },

    /**
     * Handles resource 1 selection change.
     * Fetches versions for the new resource and auto-selects the first version.
     * @param {number|string} resourceId - The newly selected resource ID
     */
    async onResource1Change(resourceId) {
      this.r1 = resourceId;
      const versions = await this.fetchVersions(resourceId);
      if (versions.length > 0) {
        this.v1 = versions[0].versionNumber;
      }
      this.updateUrl();
    },

    /**
     * Handles resource 2 selection change.
     * Fetches versions for the new resource and auto-selects the first version.
     * @param {number|string} resourceId - The newly selected resource ID
     */
    async onResource2Change(resourceId) {
      this.r2 = resourceId;
      const versions = await this.fetchVersions(resourceId);
      if (versions.length > 0) {
        this.v2 = versions[0].versionNumber;
      }
      this.updateUrl();
    },

    /**
     * Handles version 1 selection change.
     * @param {number|string} versionNumber - The newly selected version number
     */
    onVersion1Change(versionNumber) {
      this.v1 = versionNumber;
      this.updateUrl();
    },

    /**
     * Handles version 2 selection change.
     * @param {number|string} versionNumber - The newly selected version number
     */
    onVersion2Change(versionNumber) {
      this.v2 = versionNumber;
      this.updateUrl();
    }
  };
}
