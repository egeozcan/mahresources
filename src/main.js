// Import Alpine.js and plugins
import Alpine from 'alpinejs';
import morph from '@alpinejs/morph';
import collapse from '@alpinejs/collapse';
import focus from '@alpinejs/focus';

// Workaround: Alpine's x-trap (focus-trap) doesn't support shadow DOM.
// focus-trap's checkFocusIn handler uses composedPath()[0] to get the actual
// focused element inside shadow DOM, then checks container.contains() which
// fails across shadow boundaries. This causes focus to be stolen from shadow
// DOM inputs back to light DOM elements. We intercept focusin in capture phase
// (before focus-trap's handler) and stop propagation when focus is legitimately
// going to a shadow DOM element inside a trap container.
document.addEventListener('focusin', (e) => {
  const target = e.target;
  if (!(target instanceof HTMLElement) || !target.shadowRoot) return;
  if (e.composedPath()[0] === target) return;
  let el = target;
  while (el) {
    for (const attr of el.attributes) {
      if (attr.name === 'x-trap' || attr.name.startsWith('x-trap.')) {
        e.stopImmediatePropagation();
        return;
      }
    }
    el = el.parentElement;
  }
}, true);

// Import utility functions and expose them globally
import {
  abortableFetch,
  isUndef,
  isNumeric,
  pick,
  setCheckBox,
  updateClipboard,
  parseQueryParams,
  addMetaToGroup,
  addMetaToResource
} from './index.js';

// Import tableMaker
import { renderJsonTable } from './tableMaker.js';

// Import Alpine components
import { autocompleter } from './components/dropdown.js';
import { confirmAction } from './components/confirmAction.js';
import { freeFields, generateParamNameForMeta, getJSONValue, getJSONOrObjValue } from './components/freeFields.js';
import { registerBulkSelectionStore, bulkSelectionForms, selectableItem, setupBulkSelectionListeners } from './components/bulkSelection.js';
import { registerSavedSettingStore } from './components/storeConfig.js';
import { globalSearch } from './components/globalSearch.js';
import { registerLightboxStore } from './components/lightbox.js';
import { registerEntityPickerStore } from './components/picker/index.js';
import { registerPasteUploadStore, setupPasteListener } from './components/pasteUpload.js';
import { multiSort } from './components/multiSort.js';
import { downloadCockpit } from './components/downloadCockpit.js';
import { compareView } from './components/compareView.js';
import { imageCompare } from './components/imageCompare.js';
import { textDiff } from './components/textDiff.js';
import { blockEditor } from './components/blockEditor.js';
import { blockText, blockHeading, blockDivider, blockTodos, blockGallery, blockReferences, blockTable, blockCalendar, eventModal, blockPlugin } from './components/blocks/index.js';
import { sharedTodos } from './components/sharedTodos.js';
import { sharedCalendar } from './components/sharedCalendar.js';
import { codeEditor } from './components/codeEditor.js';
import { mrqlEditor } from './components/mrqlEditor.js';
import { groupTree } from './components/groupTree.js';
import { pluginSettings } from './components/pluginSettings.js';
import { pluginActionModal } from './components/pluginActionModal.js';
import { cardActionMenu } from './components/cardActionMenu.js';
import { mentionTextarea } from './components/mentionTextarea.js';
import { adminOverview } from './components/adminOverview.js';
import timeline from './components/timeline.js';
import { schemaEditorModal } from './components/schemaEditorModal.ts';

// Import utility modules
import { renderMentions } from './utils/renderMentions.js';

// Import web components
import './webcomponents/expandabletext.js';
import './webcomponents/inlineedit.js';
import './schema-editor/schema-editor.ts';

// Expose utility functions globally for templates that use them
window.abortableFetch = abortableFetch;
window.isUndef = isUndef;
window.isNumeric = isNumeric;
window.pick = pick;
window.setCheckBox = setCheckBox;
window.updateClipboard = updateClipboard;
window.parseQueryParams = parseQueryParams;
window.addMetaToGroup = addMetaToGroup;
window.addMetaToResource = addMetaToResource;
window.renderJsonTable = renderJsonTable;
window.generateParamNameForMeta = generateParamNameForMeta;
window.renderMentions = renderMentions;
window.getJSONValue = getJSONValue;
window.getJSONOrObjValue = getJSONOrObjValue;

// Register Alpine plugins (must be done before Alpine.start())
Alpine.plugin(morph);
Alpine.plugin(collapse);
Alpine.plugin(focus);

// Register Alpine stores
registerBulkSelectionStore(Alpine);
registerSavedSettingStore(Alpine);
registerLightboxStore(Alpine);
registerEntityPickerStore(Alpine);
registerPasteUploadStore(Alpine);

// Register Alpine data components
Alpine.data('autocompleter', autocompleter);
Alpine.data('confirmAction', confirmAction);
Alpine.data('freeFields', freeFields);
Alpine.data('bulkSelectionForms', bulkSelectionForms);
Alpine.data('selectableItem', selectableItem);
Alpine.data('globalSearch', globalSearch);
Alpine.data('multiSort', multiSort);
Alpine.data('downloadCockpit', downloadCockpit);
Alpine.data('compareView', compareView);
Alpine.data('imageCompare', imageCompare);
Alpine.data('textDiff', textDiff);
Alpine.data('blockEditor', blockEditor);
Alpine.data('blockText', blockText);
Alpine.data('blockHeading', blockHeading);
Alpine.data('blockDivider', blockDivider);
Alpine.data('blockTodos', blockTodos);
Alpine.data('blockGallery', blockGallery);
Alpine.data('blockReferences', blockReferences);
Alpine.data('blockTable', blockTable);
Alpine.data('blockCalendar', blockCalendar);
Alpine.data('eventModal', eventModal);
Alpine.data('blockPlugin', blockPlugin);
Alpine.data('sharedTodos', sharedTodos);
Alpine.data('sharedCalendar', sharedCalendar);
Alpine.data('codeEditor', codeEditor);
Alpine.data('mrqlEditor', mrqlEditor);
Alpine.data('groupTree', groupTree);
Alpine.data('pluginSettings', pluginSettings);
Alpine.data('pluginActionModal', pluginActionModal);
Alpine.data('cardActionMenu', cardActionMenu);
Alpine.data('mentionTextarea', mentionTextarea);
Alpine.data('adminOverview', adminOverview);
Alpine.data('timeline', timeline);
Alpine.data('schemaEditorModal', schemaEditorModal);

// Expose Alpine globally for debugging and morph usage
window.Alpine = Alpine;

// Start Alpine
Alpine.start();

// Initialize lightbox on DOM ready
document.addEventListener('DOMContentLoaded', () => {
  Alpine.store('lightbox').init();
  Alpine.store('lightbox').initFromDOM();
});

// Setup bulk selection listeners
setupBulkSelectionListeners();

// Setup global paste listener (handles file-input paste, modal, and context detection)
setupPasteListener();

// Refresh resource lists when background downloads complete
window.addEventListener('download-completed', async (e) => {
  const job = e.detail;
  const listContainer = document.querySelector('.list-container');

  if (!listContainer || !job.resourceId) return;

  try {
    // Fetch the current page
    const response = await fetch(window.location.href, {
      headers: { 'Accept': 'text/html' }
    });
    const html = await response.text();

    // Parse and extract the new list container
    const parser = new DOMParser();
    const doc = parser.parseFromString(html, 'text/html');
    const newListContainer = doc.querySelector('.list-container');

    if (newListContainer) {
      // Use Alpine morph to smoothly update the content
      Alpine.morph(listContainer, newListContainer, {
        updating(el, toEl, childrenOnly, skip) {
          // Preserve Alpine state where possible
          if (el._x_dataStack) {
            toEl._x_dataStack = el._x_dataStack;
          }
        }
      });

      // Re-initialize lightbox for new images
      Alpine.store('lightbox').initFromDOM();
    }
  } catch (err) {
    console.error('Failed to refresh resource list:', err);
  }
});
