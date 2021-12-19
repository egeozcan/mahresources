document.addEventListener('alpine:init', () => {
    Alpine.store('bulkSelection', {
        selectedIds: new Set(),
        activeEditor: null,

        isSelected(id) {
            return this.selectedIds.has(id)
        },

        isAnySelected() {
            return this.selectedIds.size > 0
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

        isActiveEditor(el) {
            return this.activeEditor === el;
        },

        setActiveEditor(el) {
            this.activeEditor = el;
        },

        closeEditor(el) {
            if (!this.isActiveEditor(el)) {
                return;
            }

            this.activeEditor = null;
        },

    })
})