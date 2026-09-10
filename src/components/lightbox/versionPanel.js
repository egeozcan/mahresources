import { focusOn } from '../../utils/focus.js';

/** Resource Versions change an item's media; its identity and metadata remain the Resource's. */
export const versionPanelState = {
  versionPanelOpen: false,
  versionsCache: new Map(),
};

export const versionPanelMethods = {
  versionEntries() {
    return this.versionsCache.get(this.getCurrentItem()?.id) || [];
  },

  isVersionDisplayable(version) {
    return !!version && /^(image|video)\//.test(version.contentType);
  },

  currentVersionId() {
    return this.displayDetails()?.currentVersionId ?? this.detailsCache.get(this.getCurrentItem()?.id)?.currentVersionId ??
      (this.versionEntries().some(version => version.id === 0) ? 0 : null);
  },

  displayedVersionId() {
    return this.getCurrentItem()?.displayedVersion?.id ?? this.currentVersionId();
  },

  isHistoricalVersion() {
    const version = this.getCurrentItem()?.displayedVersion;
    return !!version && version.id !== this.currentVersionId();
  },

  displayedVersionLabel() {
    const version = this.getCurrentItem()?.displayedVersion;
    if (!version) return '';
    const number = version.versionNumber ?? this.versionEntries().find(v => v.id === version.id)?.versionNumber;
    return number == null ? 'Historical version' : `Version ${number} of ${this.versionEntries().length}`;
  },

  formatVersionSize(bytes = 0) {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  },

  versionDate(version) {
    return version?.createdAt ? new Date(version.createdAt).toLocaleDateString('en-US', { month: 'short', day: '2-digit', year: 'numeric' }) : '';
  },

  versionThumbnailUrl(version, height = 96) {
    const target = version.id ? `/v1/resource/version/preview?versionId=${version.id}` : `/v1/resource/preview?id=${version.resourceId}`;
    return `${target}&height=${height}&v=${encodeURIComponent(version.hash)}`;
  },

  async openVersionPanel() {
    if (this.cropOpen) this.closeCrop();
    this.versionPanelOpen = true;
    this.scrollDisplayedVersion();
    await this.fetchResourceDetails(undefined, true);
    this.scrollDisplayedVersion();
  },

  toggleVersionPanel() {
    if (this.versionPanelOpen) {
      this._restoreVersionFocus('[data-version-panel]');
      this.versionPanelOpen = false;
      requestAnimationFrame(() => this.constrainPan());
    }
    else return this.openVersionPanel();
  },

  _restoreVersionFocus(selector) {
    if (!document.querySelector(selector)?.contains(document.activeElement)) return;
    requestAnimationFrame(() => {
      if (this.isOpen) focusOn(document.querySelector('button[aria-controls="lightbox-version-panel"]'));
    });
  },

  scrollDisplayedVersion() {
    requestAnimationFrame(() => {
      document.querySelector('[data-version-panel] [aria-current="true"]')?.scrollIntoView({ block: 'nearest', inline: 'center' });
      this.constrainPan();
    });
  },

  selectVersion(version) {
    const item = this.getCurrentItem();
    if (!item || version.resourceId !== item.id || !this.isVersionDisplayable(version) || this.rotating) return;
    if (version.id === this.currentVersionId()) { this.resetDisplayedVersion(); return; }
    if (this.cropOpen) this.closeCrop();
    this.pauseCurrentVideo();
    item.currentMedia ??= this._versionMediaFields(item);
    item.displayedVersion = version;
    Object.assign(item, {
      viewUrl: `/v1/resource/version/file?versionId=${version.id}`,
      contentType: version.contentType, width: version.width || 0, height: version.height || 0,
    });
    this._versionMediaChanged();
    this.announceVersion(version);
    this.scrollDisplayedVersion();
  },

  announceVersion(version) {
    if (version.versionNumber != null) {
      this.announce(`Showing version ${version.versionNumber} of ${this.versionEntries().length}, uploaded ${this.versionDate(version).replace(',', '')}`);
    }
  },

  _versionMediaFields(item) {
    return { viewUrl: item.viewUrl, contentType: item.contentType, width: item.width, height: item.height };
  },

  // All authoritative Resource refreshes update the reset target, even while history is shown.
  _preserveDisplayedVersion(item, next) {
    if (!item.displayedVersion || next === item) return next;
    return { ...next, currentMedia: this._versionMediaFields(next), ...this._versionMediaFields(item) };
  },

  _cacheVersions(resourceId, versions) {
    if (!Array.isArray(versions)) return;
    this.versionsCache.set(resourceId, versions);
    const item = this.getCurrentItem();
    if (item?.id !== resourceId || !item.displayedVersion) return;
    const fresh = versions.find(v => v.id === item.displayedVersion.id);
    if (!fresh || fresh.id === this.currentVersionId()) {
      this.resetDisplayedVersion(false);
      this.announce(!fresh ? 'Displayed version is no longer available. Showing current version' : 'Showing current version');
    } else {
      // The resource-page entry point paints before the detail response supplies the label.
      if (item.displayedVersion.versionNumber == null) this.announceVersion(fresh);
      item.displayedVersion = fresh;
    }
  },

  resetDisplayedVersion(announce = true) {
    this._restoreVersionFocus('[data-version-badge]');
    let changed = false;
    // Navigation has already advanced currentIndex; restore the just-left item too.
    for (const item of this.items) {
      if (!item.displayedVersion) continue;
      Object.assign(item, item.currentMedia);
      delete item.displayedVersion;
      delete item.currentMedia;
      changed = true;
    }
    if (changed) {
      this.pauseCurrentVideo();
      this._versionMediaChanged();
      if (announce) this.announce('Showing current version');
      this.scrollDisplayedVersion();
    }
  },

  _versionMediaChanged() {
    this.resetZoom();
    this.loading = this.isVersionDisplayable({ contentType: this.getCurrentItem()?.contentType });
    this.mediaErrorId = null;
    this.scheduleMediaCheck();
  },
};
