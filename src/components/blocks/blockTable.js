// src/components/blocks/blockTable.js
export function blockTable() {
  return {
    get columns() {
      return this.block?.content?.columns || [];
    },
    get rows() {
      return this.block?.content?.rows || [];
    },
    get queryId() {
      return this.block?.content?.queryId;
    },
    get sortColumn() {
      return this.block?.state?.sortColumn || '';
    },
    get sortDir() {
      return this.block?.state?.sortDir || 'asc';
    },
    sortBy(column) {
      const newDir = this.sortColumn === column && this.sortDir === 'asc' ? 'desc' : 'asc';
      this.$dispatch('update-state', { sortColumn: column, sortDir: newDir });
    },
    get sortedRows() {
      if (!this.sortColumn) return this.rows;
      const colIdx = this.columns.indexOf(this.sortColumn);
      if (colIdx < 0) return this.rows;

      return [...this.rows].sort((a, b) => {
        const va = a[colIdx];
        const vb = b[colIdx];
        const cmp = va < vb ? -1 : va > vb ? 1 : 0;
        return this.sortDir === 'asc' ? cmp : -cmp;
      });
    }
  };
}
