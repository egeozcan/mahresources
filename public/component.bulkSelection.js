document.addEventListener("alpine:init", () => {
  let currentIndex = 0;

  Alpine.store("bulkSelection", {
    selectedIds: new Set(),
    elements: [],
    options: {},
    activeEditor: null,
    lastSelected: null,

    isSelected(id) {
      return this.selectedIds.has(id);
    },

    isAnySelected() {
      return this.selectedIds.size > 0;
    },

    select(id) {
      this.lastSelected = id;

      if (this.isSelected(id)) {
        return;
      }

      this.selectedIds.add(id);
      setCheckBox(this.options[id].el, true);
    },

    deselect(id) {
      this.lastSelected = id;

      if (!this.isSelected(id)) {
        return;
      }

      this.selectedIds.delete(id);
      setCheckBox(this.options[id].el, false);
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
        elementsToProcess.forEach((option) => this.deselect(option.itemId));
      } else {
        elementsToProcess.forEach((option) => this.select(option.itemId));
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
      this.options[option.itemId] = option;

      if (option.el.checked) {
        this.select(option.itemId);
      } else {
        this.deselect(option.itemId);
      }
    },
  });

  window.Alpine.data("selectableItem", ({ itemNo, itemId } = {}) => {
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
      },
    };
  });

  function setCheckBox(checkBox, checked) {
    if (checked) {
      checkBox.setAttribute("checked", "checked");
    } else {
      checkBox.removeAttribute("checked");
    }

    checkBox.checked = checked;
  }

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
});
