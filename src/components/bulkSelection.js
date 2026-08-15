import { setCheckBox } from '../index.js';
import { observeSelectorField } from '../selector/selectorRegistry.ts';
import { createLiveRegion } from '../utils/ariaLiveRegion.js';
import { morphOptionsWithShortcodeElements } from '../utils/shortcodeElementMorph.js';
import { findListContainer } from '../utils/listContainer.js';
import { focusOn } from '../utils/focus.js';

const btnClasses = `bulk-action-btn inline-flex justify-center
      py-1.5 px-3 mt-3
      border
      items-center
      text-sm font-medium rounded-md
      focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-teal-500`;

let currentIndex = 0;
let _bulkLiveRegion = null;

// A rendered selectable row. Rows opt in with `x-data="selectableItem({...})"`
// and Alpine leaves the attribute in place, so this matches them whether they
// are cards (<article> on /tags, /notes, /resources, /groups, /downloads) or the
// detail table's rows (<tr> on /resources/details).
export const SELECTABLE_ITEM_SELECTOR = '[x-data^="selectableItem"]';

export function registerBulkSelectionStore(Alpine) {
  Alpine.store("bulkSelection", {
    selectedIds: new Set(),
    elements: [],
    editors: [],
    options: {},
    activeEditor: null,
    lastSelected: null,

    init() {
      currentIndex = 0;
      if (!_bulkLiveRegion) {
        _bulkLiveRegion = createLiveRegion();
      }
    },

    announce(message) {
      _bulkLiveRegion?.announce(message);
    },

    /**
     * Whether this page has any selectable row — the guard that keeps "Select
     * All" off an empty list (WS6, finding 68).
     *
     * Deliberately not `elements.length > 0` on its own. Rows register from
     * their own Alpine `init()`, and the Select All row precedes them in the
     * DOM, so Alpine evaluates its `x-show` while the registry is still empty
     * and fills it a moment later. Reading only the registry therefore answers
     * "no rows" for the first frame and "rows" immediately after: `x-collapse`
     * takes that as a change and *animates the row open on page load*. Measured
     * on /tags at 1280x900: the row goes 13px -> 46px over ~190ms and everything
     * below it — the whole list and the pagination row in the footer — is shoved
     * down 37px about 200ms after the page settles.
     *
     * The rows are already in the DOM when that first evaluation runs, so count
     * those and the first answer matches the settled one: no flip, no animation,
     * no shift. Selecting and deselecting still animate, because those move the
     * *second* term of the predicate.
     *
     * The registry is consulted first, so once it is populated this is a field
     * read and stays reactive; the DOM query only ever decides the initial
     * frame, and on a genuinely empty list both are empty and the row stays
     * hidden.
     */
    hasSelectableItems() {
      return this.elements.length > 0 || document.querySelector(SELECTABLE_ITEM_SELECTOR) !== null;
    },

    isSelected(id) {
      return this.selectedIds.has(id);
    },

    isAnySelected() {
      return this.selectedIds.size > 0;
    },

    select(id) {
      this.lastSelected = id;
      this.setActiveEditor(null);

      if (this.isSelected(id)) {
        return;
      }

      this.selectedIds.add(id);
      setCheckBox(this.options[id].el, true);
      this.announce(`${this.selectedIds.size} item${this.selectedIds.size === 1 ? '' : 's'} selected`);
    },

    deselect(id) {
      this.lastSelected = id;
      this.setActiveEditor(null);

      if (!this.isSelected(id)) {
        return;
      }

      this.selectedIds.delete(id);
      setCheckBox(this.options[id].el, false);
      this.announce(this.selectedIds.size > 0 ? `${this.selectedIds.size} item${this.selectedIds.size === 1 ? '' : 's'} selected` : 'Selection cleared');
    },

    toggle(id) {
      if (this.isSelected(id)) {
        this.deselect(id);
      } else {
        this.select(id);
      }
    },

    selectUntil(id) {
      if (!this.lastSelected) {
        this.toggle(id);
        return;
      }

      const from = this.options[this.lastSelected].itemNo;
      const to = this.options[id].itemNo;
      const elementsToProcess = [...this.elements].slice(
        Math.min(from, to),
        Math.max(from, to) + 1
      );

      if (this.isSelected(id)) {
        elementsToProcess.forEach((option) => { if (option) this.deselect(option.itemId); });
      } else {
        elementsToProcess.forEach((option) => { if (option) this.select(option.itemId); });
      }
    },

    deselectAll() {
      this.selectedIds.forEach((x) => this.deselect(x));
      this._followFocus('[data-bulk-select-all]');
    },

    selectAll() {
      this.elements.forEach((option) => this.select(option.itemId));
      this._followFocus('[data-bulk-deselect-all]');
    },

    // WS4 finding 66. Both controls live inside an x-show/x-collapse wrapper
    // keyed on the selection being empty, so activating one collapses the
    // element that has focus and the browser drops the reader on <body> — after
    // selecting 50 rows they had to Tab from the top of the document to reach
    // the bulk actions they had just revealed.
    //
    // $nextTick is required, not optional: a synchronous focus() runs before
    // Alpine flips x-show, so the destination is still display:none and refuses
    // focus. It is also sufficient — measured, the replacement is focusable at
    // the start of the transition, so there is no need to wait out x-collapse.
    _followFocus(selector) {
      queueMicrotask(() => {
        requestAnimationFrame(() => {
          const replacement = Array.from(document.querySelectorAll(selector))
            .find((el) => el.offsetParent !== null);
          if (replacement) focusOn(replacement);
        });
      });
    },

    hasActiveEditor() {
      return this.activeEditor !== null;
    },

    toggleEditor(form) {
      this.isActiveEditor(form) ? this.closeEditor() : this.setActiveEditor(form);
    },

    isActiveEditor(el) {
      return this.activeEditor === el;
    },

    /**
     * @param {HTMLFormElement} el
     */
    setActiveEditor(el) {
      this.activeEditor = el;
      setTimeout(() => el?.querySelector("input:not([type='hidden'])")?.focus?.(), 200);
    },

    closeEditor(el) {
      if (el && !this.isActiveEditor(el)) {
        return;
      }

      this.setActiveEditor(null);
    },

    registerOption(option) {
      option.itemNo = option.itemNo || ++currentIndex;
      this.elements[option.itemNo] = option;
      this.options[option.itemId] = option;

      // Registration is not a user action, so it must not move the shift-range
      // anchor. `select` and `deselect` both stamp `lastSelected` *before* they
      // check whether anything actually changed, so syncing each card's initial
      // state through them left the anchor on the last card on the page — and
      // `selectUntil` then read it as "the reader's last click". A reader whose
      // first interaction was shift-clicking the top card therefore selected the
      // entire page instead of that one card, which on /downloads is one keystroke
      // away from a bulk delete.
      const anchor = this.lastSelected;

      if (option.el.checked) {
        this.select(option.itemId);
      } else {
        this.deselect(option.itemId);
      }

      this.lastSelected = anchor;
    },

    registerForm(form) {
      const btn = document.createElement("button");
      const buttonText = form.querySelector("label, button").innerText;

      btn.innerText = buttonText;
      btn.className = btnClasses;
      btn.type = "button";
      btn.setAttribute("aria-expanded", "false");
      btn.setAttribute("aria-label", `Toggle ${buttonText} editor`);
      btn.addEventListener("click", () => this.toggleEditor(form));
      btn.setAttribute("x-effect", `() => {
        const isActive = $store.bulkSelection.isActiveEditor($el.nextElementSibling);
        $el.dataset.active = isActive;
        $el.setAttribute("aria-expanded", isActive);
      }`);

      form.setAttribute("x-show", "$store.bulkSelection.isActiveEditor($el)");
      form.setAttribute("x-collapse", "");
      form.setAttribute(":class", "$store.bulkSelection.isActiveEditor($el) && 'active'");
      form.insertAdjacentElement("beforebegin", btn);

      this.editors.push(form);
    },

    /**
     * Run one bulk editor form over fetch and morph the refreshed list in.
     *
     * Called from a *delegated* submit listener on the toolbar container rather
     * than from a listener on each form — see bulkSelectionForms. This used to
     * be a per-form listener registered here, in the parent component's init,
     * which meant it was always registered before the form's own
     * `x-bind="events"` and therefore always ran first. It calls preventDefault
     * unconditionally, so a confirm the reader *dismissed* and a selection guard
     * that *blocked* the submit were both ignored and the request went out
     * anyway: measured on the tag bulk-merge form, where dismissing "Selected
     * tags will be merged. Are you sure?" still performed the merge, and
     * pressing Merge with no winner still produced a 400 (findings 16/92/153).
     */
    async submitEditorForm(form) {
      try {
        form.parentElement.classList.add("pointer-events-none");
        const response = await fetch(form.action, { method: "POST", body: new FormData(form) });
        if (!response.ok) {
          throw new Error(`Server error: ${response.status}`);
        }
        const url = new URL(window.location);
        url.pathname = url.pathname + ".body";
        const refreshResponse = await fetch(url.toString());
        if (!refreshResponse.ok) {
          throw new Error(`Could not refresh list: ${refreshResponse.status}`);
        }
        const newHtml = await refreshResponse.text();
        const parser = new DOMParser();
        const refreshedDocument = parser.parseFromString(newHtml, 'text/html');
        const listContainer = findListContainer(document);
        const refreshedListContainer = findListContainer(refreshedDocument);

        if (!listContainer || !refreshedListContainer) {
          throw new Error('Could not find refreshed list');
        }

        form.reset();
        this.deselectAll();
        Alpine.morph(
          listContainer,
          refreshedListContainer,
          morphOptionsWithShortcodeElements(),
        );
        this.announce('Bulk operation completed successfully');
      } catch (err) {
        this.announce(`Bulk operation failed: ${err.message}`);
        alert(`Bulk operation failed: ${err.message}`);
      } finally {
        form.parentElement.classList.remove("pointer-events-none");
      }
    },
  });
}

export function bulkSelectionForms() {
  return {
    init() {
      // $root dies with the subtree, so capture it while it is attached.
      const root = this.$root;
      const store = this.$store.bulkSelection;
      root.querySelectorAll("form").forEach(form => store.registerForm(form));

      // Delegated on the container, not bound per form. `submit` bubbles, so a
      // listener here runs after every listener on the form itself whatever the
      // registration order — and registration order is exactly the problem:
      // Alpine initialises this component before it walks the forms inside it,
      // so a per-form listener added above would run before the form's own
      // `x-bind="events"` and could not see their preventDefault. This handler
      // therefore honours a dismissed confirm and a blocked empty selection
      // instead of firing the request regardless.
      root.addEventListener("submit", (event) => {
        const form = event.target;
        if (!(form instanceof HTMLFormElement)) return;
        if (form.classList.contains("no-ajax")) return;
        if (!store.editors.includes(form)) return;
        if (event.defaultPrevented) return;
        event.preventDefault();
        void store.submitEditorForm(form);
      });
    }
  }
}

export function selectableItem({ itemNo, itemId } = {}) {
  return {
    init() {
      const el = this.$root.querySelector("input[type='checkbox']");

      this.$store.bulkSelection.registerOption({
        itemNo,
        itemId,
        el,
      });
    },

    selected() {
      return this.$store.bulkSelection.isSelected(itemId);
    },

    events: {
      /**
       * @param {MouseEvent} e
       */
      ["@click"](e) {
        if (e.shiftKey) {
          this.$store.bulkSelection.selectUntil(itemId);
          return;
        }

        this.$store.bulkSelection.toggle(itemId);
      },
      ["@contextmenu"](e) {
        e.preventDefault();
        this.$store.bulkSelection.selectUntil(itemId);
      },
      ["@keydown.space.prevent"]() {
        this.$store.bulkSelection.toggle(itemId);
      },
      ["@keydown.enter.prevent"]() {
        this.$store.bulkSelection.toggle(itemId);
      },
    },
  };
}

export function setupBulkSelectionListeners() {
  document.addEventListener("keypress", function (e) {
    if (e.key !== " ") {
      return;
    }

    const list = new Set();
    const selection = window.getSelection();
    const rangeCount = selection.rangeCount;

    if (selection.type !== "Range") {
      return;
    }

    e.preventDefault();

    for (let i = 0; i < rangeCount; i++) {
      const { startContainer, endContainer } = selection.getRangeAt(i);

      if (startContainer.querySelector) {
        const checkBox = startContainer.querySelector(['[type="checkbox"]']);

        if (checkBox) {
          list.add(checkBox);
        }
      }

      if (endContainer.querySelector) {
        const checkBox = endContainer.querySelector(['[type="checkbox"]']);

        if (checkBox) {
          list.add(checkBox);
        }
      }
    }

    for (const checkBox of list) {
      checkBox.click();
    }

    selection.empty();
  });

  [...document.querySelectorAll(".tags")].forEach(async (container) => {
    container.addEventListener("click", async function (e) {
      if (!e.target.classList.contains("edit-in-list")) {
        return;
      }

      e.preventDefault();
      const entityType = e.target.dataset.entityType;

      // Get the entity's current tags from the Alpine component data
      const xDataEl = e.target.closest('[x-data]');
      let currentTags = [];
      if (xDataEl && window.Alpine) {
        const data = window.Alpine.$data(xDataEl);
        if (data?.entity?.Tags) {
          currentTags = data.entity.Tags;
        }
      }

      const res = await (async function() {
        const url = new URL(`${window.location.origin}/partials/autocompleter`);

        url.searchParams.append("profile", "tag");
        url.searchParams.append("usage", entityType);
        url.searchParams.append("selectedItems", JSON.stringify(currentTags));
        url.searchParams.append("title", "Edit tags");
        url.searchParams.append("id", `tagEditor_${Math.random()}`);
        url.searchParams.append("elName", "editedId");

        return fetch(url.toString()).then(x => x.text());
      })();

      const form = document.createElement("form");
      form.dataset.inlineEditor = "true";
      form.addEventListener("submit", e => {
        e.preventDefault();
      });
      // The inline editor persists on every atomic change of its own tag field. Observing the
      // named field rather than a document-wide event keeps one row's editor from reacting to
      // another editor opened elsewhere on the same list.
      //
      // The write serializes the form, and the field's hidden controls are rendered by Alpine
      // from the new selection. Registry delivery is synchronous -- it happens inside the
      // selector's own state publication -- so the DOM is still one flush behind at this point.
      // Serializing now would post an empty selection and silently clear the entity's tags.
      observeSelectorField(form, "editedId", () => {
        window.Alpine.nextTick(() => {
          fetch('/v1/' + entityType + 's/replaceTags', { method: "POST", body: new FormData(form) });
        });
      });
      form.className = "mb-6 p-4 active";

      const elInput = document.createElement("input");
      elInput.setAttribute(":value", "entity.ID");
      elInput.name = "ID";
      elInput.type = "hidden";

      const parser = new DOMParser();
      const doc = parser.parseFromString(res, 'text/html');
      form.replaceChildren(...doc.body.childNodes);
      form.appendChild(elInput);

      container.innerHTML = "";
      container.appendChild(form);

      window.Alpine.initTree(form);

      setTimeout(() => form.querySelector("[x-ref='autocompleter']")?.focus(), 10);
    })
  });
}
