// Import Alpine.js and plugins
import Alpine from 'alpinejs';
import morph from '@alpinejs/morph';
import collapse from '@alpinejs/collapse';

// Import baguettebox
import baguetteBox from 'baguettebox.js';
import 'baguettebox.js/dist/baguetteBox.min.css';

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
import { schemaForm } from './components/schemaForm.js';

// Import web components
import './webcomponents/expandabletext.js';
import './webcomponents/inlineedit.js';

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
window.getJSONValue = getJSONValue;
window.getJSONOrObjValue = getJSONOrObjValue;

// Register Alpine plugins (must be done before Alpine.start())
Alpine.plugin(morph);
Alpine.plugin(collapse);

// Register Alpine stores
registerBulkSelectionStore(Alpine);
registerSavedSettingStore(Alpine);

// Register Alpine data components
Alpine.data('autocompleter', autocompleter);
Alpine.data('confirmAction', confirmAction);
Alpine.data('freeFields', freeFields);
Alpine.data('bulkSelectionForms', bulkSelectionForms);
Alpine.data('selectableItem', selectableItem);
Alpine.data('globalSearch', globalSearch);
Alpine.data('schemaForm', schemaForm);

// Expose Alpine globally for debugging and morph usage
window.Alpine = Alpine;

// Start Alpine
Alpine.start();

// Initialize baguetteBox on DOM ready
document.addEventListener('DOMContentLoaded', () => {
  const filter = /#image\//;
  const fullScreen = false;
  const animation = false;

  const options = { filter, fullScreen, animation };

  baguetteBox.run('.list-container', options);
  baguetteBox.run('.gallery', options);
});

// Setup paste handler for file inputs
window.addEventListener('paste', e => {
  const fileInput = document.querySelector("input[type='file']");
  if (!fileInput || !e.clipboardData.files || !e.clipboardData.files.length) {
    return;
  }
  fileInput.files = e.clipboardData.files;
});

// Setup bulk selection listeners
setupBulkSelectionListeners();
