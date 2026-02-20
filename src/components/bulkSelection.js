import { setCheckBox } from '../index.js';
import { createLiveRegion } from '../utils/ariaLiveRegion.js';

const btnClasses = `bulk-action-btn inline-flex justify-center
      py-1.5 px-3 mt-3
      border
      items-center
      text-sm font-medium rounded-md
      focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-teal-500`;

let currentIndex = 0;
let _bulkLiveRegion = null;

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
    },

    selectAll() {
      this.elements.forEach((option) => this.select(option.itemId));
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

      if (option.el.checked) {
        this.select(option.itemId);
      } else {
        this.deselect(option.itemId);
      }
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

      if (form.classList.contains("no-ajax")) {
        return;
      }

      form.addEventListener("submit", async (e) => {
        e.preventDefault();
        try {
          form.parentElement.classList.add("pointer-events-none");
          const response = await fetch(form.action, { method: "POST", body: new FormData(form) });
          if (!response.ok) {
            throw new Error(`Server error: ${response.status}`);
          }
          const url = new URL(window.location);
          url.pathname = url.pathname + ".body";
          const newHtml = await fetch(url.toString()).then(x => x.text());
          form.reset();
          this.deselectAll();
          Alpine.morph(document.querySelector(".list-container, .items-container"), newHtml);
          this.announce('Bulk operation completed successfully');
        } catch (err) {
          this.announce(`Bulk operation failed: ${err.message}`);
          alert(`Bulk operation failed: ${err.message}`);
        } finally {
          form.parentElement.classList.remove("pointer-events-none");
        }
      })
    },
  });
}

export function bulkSelectionForms() {
  return {
    init() {
      this.$root.querySelectorAll("form").forEach(form => this.$store.bulkSelection.registerForm(form));
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

  [...document.querySelectorAll(".list-container .tags")].forEach(async (container) => {
    container.addEventListener("click", async function (e) {
      if (!e.target.classList.contains("edit-in-list")) {
        return;
      }

      e.preventDefault();
      const entityType = e.target.dataset.entityType;

      const res = await (async function() {
        const url = new URL(`${window.location.origin}/partials/autocompleter`);

        url.searchParams.append("selectedItems", "entity.Tags");
        url.searchParams.append("title", "");
        url.searchParams.append("sortBy", `most_used_${entityType}`);
        url.searchParams.append("id", `tagEditor_${Math.random()}`);
        url.searchParams.append("elName", "editedId");
        url.searchParams.append("url", "/v1/tags");
        url.searchParams.append("addUrl", "/v1/tag");

        return fetch(url.toString()).then(x => x.text());
      })();

      const form = document.createElement("form");
      form.addEventListener("multiple-input", e => {
        fetch('/v1/' + entityType + 's/replaceTags', { method: "POST", body: new FormData(form) });
      })
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
