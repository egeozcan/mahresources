import { findListContainer, LIST_CONTAINER_SELECTOR } from '../utils/listContainer.js';
import { morphAndReinitChangedComponents } from '../utils/shortcodeElementMorph.js';

export const DOWNLOAD_LIST_REFRESH_DEBOUNCE_MS = 500;

/**
 * Refresh resource lists after background downloads create resources.
 *
 * Completion events are bursty and DOM event dispatch does not await async
 * listeners. The coordinator therefore debounces before starting, permits only
 * one page fetch at a time, and retains a dirty bit for one trailing refresh
 * when more resources complete while a request is active.
 */
export function setupDownloadListRefresh({
  eventTarget = window,
  root = document,
  fetchImpl = (...args) => fetch(...args),
  currentURL = () => window.location.href,
  morphList = morphAndReinitChangedComponents,
  afterRefresh = () => {},
  logger = console,
  debounceMs = DOWNLOAD_LIST_REFRESH_DEBOUNCE_MS,
} = {}) {
  let refreshTimer = null;
  let refreshInFlight = false;
  let refreshDirty = false;
  let destroyed = false;

  const armRefresh = () => {
    if (refreshTimer !== null) clearTimeout(refreshTimer);
    refreshTimer = setTimeout(() => {
      refreshTimer = null;
      void refresh();
    }, debounceMs);
  };

  const refresh = async () => {
    if (destroyed || refreshInFlight || !refreshDirty) return;

    const listContainer = findListContainer(root);
    if (!listContainer) {
      refreshDirty = false;
      return;
    }

    refreshDirty = false;
    refreshInFlight = true;

    try {
      const response = await fetchImpl(currentURL(), {
        headers: {
          'Accept': 'text/html',
          'X-Mahresources-Refresh-Reason': 'download-completed',
        },
      });
      if (!response.ok) {
        throw new Error(`resource list refresh failed: HTTP ${response.status}`);
      }

      const html = await response.text();
      if (destroyed) return;

      const parser = new DOMParser();
      const refreshedDocument = parser.parseFromString(html, 'text/html');
      const newListContainer = findListContainer(refreshedDocument);

      if (newListContainer) {
        // A page can hold several lists (a group page has its own resources beside its
        // related ones). Morphing only the first left the others showing pre-download
        // content, so pair them up when the refreshed document has the same shape and fall
        // back to the single container when it does not.
        const current = Array.from(root.querySelectorAll(LIST_CONTAINER_SELECTOR));
        const refreshed = Array.from(refreshedDocument.querySelectorAll(LIST_CONTAINER_SELECTOR));

        if (current.length > 1 && current.length === refreshed.length) {
          current.forEach((element, index) => morphList(element, refreshed[index]));
        } else {
          morphList(listContainer, newListContainer);
        }

        afterRefresh();
      }
    } catch (error) {
      logger.error('Failed to refresh resource list:', error);
    } finally {
      refreshInFlight = false;
      // If the debounce elapsed while this request was active, no timer remains to
      // consume the dirty bit. Arm one trailing refresh rather than losing the event.
      if (!destroyed && refreshDirty && refreshTimer === null) armRefresh();
    }
  };

  const handleCompletion = (event) => {
    if (!event.detail?.resourceId || !findListContainer(root)) return;
    refreshDirty = true;
    armRefresh();
  };

  eventTarget.addEventListener('download-completed', handleCompletion);
  return () => {
    destroyed = true;
    refreshDirty = false;
    if (refreshTimer !== null) clearTimeout(refreshTimer);
    refreshTimer = null;
    eventTarget.removeEventListener('download-completed', handleCompletion);
  };
}
