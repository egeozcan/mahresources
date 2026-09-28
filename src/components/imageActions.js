// Image actions Alpine component.
//
// Lives on the resource details page (templates/displayResource.tpl): the
// sidebar button that opens the popup holding Recalculate Dimensions, Rotate and
// Crop. The open/close rules are shared with the Custom Thumbnail popup — see
// sidebarPopup.js; this file is only the one action in it that is not a form
// post.
//
// The open and close of the popup is all of it. Two of the three actions are
// native form posts that navigate, so the popup's state is irrelevant by the
// time the browser leaves the page. Crop is the third, and it is the reason this
// component exists.

import { focusOn } from '../utils/focus.js';
import { sidebarPopup } from './sidebarPopup.js';

export function imageActions({ resourceId }) {
  return {
    ...sidebarPopup(),
    resourceId,

    /**
     * Open the crop dialog, which lives outside this popup.
     *
     * Two dialogs at once is the defect `src/utils/modality.js` exists to stop,
     * and this one is a native `<dialog>` rather than another Alpine popup, so
     * the guard that prevents it cannot be the mechanism: the popup goes, the
     * crop dialog comes, in that order.
     *
     * `showModal()` is deferred a tick for the same reason pluginActionModal's
     * jobs-panel hand-off is: Alpine has not torn the x-trap down yet at the end
     * of this call, and a native modal dialog opening over an armed trap pulls
     * focus straight back into the popup behind it.
     *
     * The focus call is not decoration. `<dialog>` restores focus on close to
     * whatever was focused when it opened, and the Crop… button is inside the
     * popup we just removed — so without this the crop dialog would hand the
     * reader to `<body>`. The Image Actions button is the control the reader
     * came from and it is still on the page, which is where they should land
     * when they step back out of the crop they walked away from.
     */
    openCrop() {
      const trigger = this._opener;
      this.close({ handOff: true });
      this.$nextTick(() => {
        focusOn(trigger);
        const dialog = document.getElementById(`crop-modal-${this.resourceId}`);
        if (dialog && typeof dialog.showModal === 'function') dialog.showModal();
      });
    },
  };
}
