document.addEventListener("alpine:init", () => {
  let currentIndex = 0;

  Alpine.store("bulkSelection", {
    selectedIds: new Set(),
    elements: [],
    activeEditor: null,

    isSelected(id) {
      return this.selectedIds.has(id);
    },

    isAnySelected() {
      return this.selectedIds.size > 0;
    },

    select(id) {
      if (this.isSelected(id)) {
        return;
      }

      this.selectedIds.add(id);
    },

    deselect(id) {
      if (!this.isSelected(id)) {
        return;
      }

      this.selectedIds.delete(id);
    },

    toggle(id) {
      if (this.isSelected(id)) {
        this.deselect(id);
      } else {
        this.select(id);
      }
    },

    hasActiveEditor() {
      return this.activeEditor !== null;
    },

    isActiveEditor(el) {
      return this.activeEditor === el;
    },

    setActiveEditor(el) {
      this.activeEditor = el;
    },

    closeEditor(el) {
      if (el && !this.isActiveEditor(el)) {
        return;
      }

      this.activeEditor = null;
    },

    registerOption(option) {
      option.itemNo = option.itemNo || ++currentIndex;
      this.elements[option.itemNo] = option;

      if (!window.selectionModeActive) {
        activateSelectionMode();
        window.selectionModeActive = true;
      }
    },
  });

  window.Alpine.data("selectableItem", ({ itemNo, itemId } = {}) => {
    return {
      init() {
        this.$store.bulkSelection.registerOption({ itemNo, itemId });
        this.$root.querySelector("input[type='checkbox']").checked =
          this.$store.bulkSelection.isSelected(itemId);
      },

      selected() {
        return this.$store.bulkSelection.isSelected(itemId);
      },

      events: {
        /**
         * @param {MouseEvent} e
         */
        ["@click"](e) {
          this.$store.bulkSelection.toggle(itemId);

          if (this.selected()) {
            e.target.setAttribute("checked", "checked");
            e.target.checked = true;
          } else {
            e.target.removeAttribute("checked");
            e.target.checked = false;
          }
        },
      },
    };
  });
});

function activateSelectionMode() {
  const selection = new SelectionArea({
    selectionAreaClass: "selection-area",
    selectionContainerClass: "selection-area-container",
    container: "body",
    selectables: [".main [type='checkbox']"],
    startareas: ["body", "html", ".site"],
    boundaries: ["body"],
    behaviour: {
      overlap: "keep",
    },
  });

  selection.on("stop", (e) => {
    let selected = e.store?.changed?.added ?? e.selected;

    if (!selected || selected.length === 0) {
      selected = e.store?.selected ?? [];
    }

    document
      .querySelectorAll(".main [type='checkbox']")
      .forEach((x) => x.checked && x.click());

    setTimeout(() => {
      selected.forEach((target) => {
        if (!target.checked) {
          target.click();
        }
      });
    }, 100);
    document.body.style.userSelect = "";
  });

  selection.on("beforestart", (evt) => {
    let canBeActivated =
      evt.event.target.tagName.toLowerCase() !== "a" &&
      evt.event.target.tagName.toLowerCase() !== "input" &&
      evt.event.target.tagName.toLowerCase() !== "select" &&
      evt.event.target.tagName.toLowerCase() !== "button" &&
      evt.event.target.tagName.toLowerCase() !== "textarea" &&
      evt.event.target.tagName.toLowerCase() !== "h1" &&
      evt.event.target.tagName.toLowerCase() !== "h2" &&
      evt.event.target.tagName.toLowerCase() !== "h3" &&
      evt.event.target.tagName.toLowerCase() !== "h4" &&
      evt.event.target.tagName.toLowerCase() !== "img";

    if (canBeActivated) {
      document.body.style.userSelect = "none";
    }

    return canBeActivated;
  });

  selection.on("start", (e) => {
    document.body.style.userSelect = "none";
    selection.clearSelection();
  });
}
