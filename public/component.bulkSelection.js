document.addEventListener("alpine:init", () => {
  const btnDefaultClass = "bg-gray-500";
  const btnActiveClass = "bg-indigo-600";
  const btnClasses = `inline-flex justify-center
        py-2 px-4 mt-3
        border border-transparent
        items-center
        shadow-sm text-sm font-medium rounded-md text-white 
        ${btnDefaultClass} focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-gray-500`;

  let currentIndex = 0;

  window.Alpine.store("bulkSelection", {
    selectedIds: new Set(),
    elements: [],
    editors: [],
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
      this.setActiveEditor(null);

      if (this.isSelected(id)) {
        return;
      }

      this.selectedIds.add(id);
      setCheckBox(this.options[id].el, true);
    },

    deselect(id) {
      this.lastSelected = id;
      this.setActiveEditor(null);

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

      btn.innerText = form.querySelector("label, button").innerText;
      btn.className = btnClasses;
      btn.addEventListener("click", () => this.toggleEditor(form));
      btn.setAttribute("x-effect", `() => {
        const isActive = $store.bulkSelection.isActiveEditor($el.nextElementSibling);
        $el.classList.toggle("${btnActiveClass}", isActive);
        $el.classList.toggle("${btnDefaultClass}", !isActive);
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
          await fetch(form.action, { method: "POST", body: new FormData(form) });
          const url = new URL(window.location);
          url.pathname = url.pathname + ".body";
          const newHtml = await fetch(url.toString()).then(x => x.text());
          form.reset();
          this.deselectAll();
          Alpine.morph(document.querySelector(".list-container, .items-container"), newHtml);
        } finally {
          form.parentElement.classList.remove("pointer-events-none");
        }
      })
    },
  });

  window.Alpine.data("bulkSelectionForms", () => {
    return {
      init() {
        this.$root.querySelectorAll("form").forEach(form => this.$store.bulkSelection.registerForm(form));
      }
    }
  })

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

      const res = await (async function() {
        const url = new URL("/partials/autocompleter");

        url.searchParams.append("selectedItems", "entity.Tags");
        url.searchParams.append("title", "");
        url.searchParams.append("id", `tagEditor_${Math.random()}`);
        url.searchParams.append("elName", "editedId");
        url.searchParams.append("url", "/v1/tags");
        url.searchParams.append("addUrl", "/v1/tag");

        return fetch(url.toString()).then(x => x.text());
      })();

      const form = document.createElement("form");
      form.addEventListener("multiple-input", e => {
        fetch('/v1' + location.pathname + '/replaceTags', { method: "POST", body: new FormData(form) });
      })
      form.className = "mb-6 p-4 active";

      const elInput = document.createElement("input");
      elInput.setAttribute(":value", "entity.ID");
      elInput.name = "ID";
      elInput.type = "hidden";

      form.innerHTML = res;
      form.appendChild(elInput);

      container.innerHTML = "";
      container.appendChild(form);
    })
  });
});
