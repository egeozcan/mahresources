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
