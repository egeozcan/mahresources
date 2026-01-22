export function multiSort({ availableColumns, name }) {
  return {
    sortColumns: [],
    availableColumns,
    name,

    init() {
      // Initialize from URL params
      const params = new URLSearchParams(window.location.search);
      const sortByValues = params.getAll(this.name);

      if (sortByValues.length > 0) {
        this.sortColumns = sortByValues.map((val) => this.parseSort(val));
      } else {
        // Start with one empty row
        this.sortColumns = [{ column: "", direction: "desc", metaKey: "" }];
      }
    },

    parseSort(sortStr) {
      const parts = sortStr.trim().split(/\s+/);
      const column = parts[0] || "";
      const direction = parts[1] || "desc";

      // Check if this is a meta sort (e.g., meta->>'key_name')
      const metaMatch = column.match(/^meta->>'([a-z_]+)'$/);
      if (metaMatch) {
        return {
          column: "__meta__",
          direction,
          metaKey: metaMatch[1],
        };
      }

      return {
        column,
        direction,
        metaKey: "",
      };
    },

    formatSort(sort) {
      if (!sort.column) return "";
      if (sort.column === "__meta__") {
        if (!sort.metaKey) return "";
        return `meta->>'${sort.metaKey}' ${sort.direction}`;
      }
      return `${sort.column} ${sort.direction}`;
    },

    addSort() {
      this.sortColumns.push({ column: "", direction: "desc", metaKey: "" });
    },

    removeSort(index) {
      if (this.sortColumns.length > 1) {
        this.sortColumns.splice(index, 1);
      } else {
        // Keep at least one row, but clear it
        this.sortColumns[0] = { column: "", direction: "desc", metaKey: "" };
      }
    },

    isValidMetaKey(key) {
      // Only allow lowercase letters and underscores (matches backend regex)
      return /^[a-z_]+$/.test(key);
    },

    moveUp(index) {
      if (index > 0) {
        const temp = this.sortColumns[index];
        this.sortColumns[index] = this.sortColumns[index - 1];
        this.sortColumns[index - 1] = temp;
      }
    },

    moveDown(index) {
      if (index < this.sortColumns.length - 1) {
        const temp = this.sortColumns[index];
        this.sortColumns[index] = this.sortColumns[index + 1];
        this.sortColumns[index + 1] = temp;
      }
    },

    getColumnName(value) {
      const col = this.availableColumns.find((c) => c.Value === value);
      return col ? col.Name : value;
    },

    getAvailableColumnsForRow(currentIndex) {
      // Return columns not already selected in other rows
      const usedColumns = this.sortColumns
        .filter((_, i) => i !== currentIndex)
        .map((s) => s.column)
        .filter((c) => c);

      return this.availableColumns.filter(
        (c) => !usedColumns.includes(c.Value)
      );
    },
  };
}
