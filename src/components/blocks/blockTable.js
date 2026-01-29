// src/components/blocks/blockTable.js
// editMode is passed as a getter function to maintain reactivity with parent scope
export function blockTable(block, saveContentFn, saveStateFn, getEditMode) {
  return {
    block,
    saveContentFn,
    saveStateFn,
    getEditMode,

    get editMode() {
      return this.getEditMode ? this.getEditMode() : false;
    },
    columns: JSON.parse(JSON.stringify(block?.content?.columns || [])),
    rows: JSON.parse(JSON.stringify(block?.content?.rows || [])),
    queryId: block?.content?.queryId,
    sortColumn: block?.state?.sortColumn || '',
    sortDirection: block?.state?.sortDirection || 'asc',

    toggleSort(colId) {
      this.sortDirection = this.sortColumn === colId && this.sortDirection === 'asc' ? 'desc' : 'asc';
      this.sortColumn = colId;
      this.saveStateFn(this.block.id, { sortColumn: this.sortColumn, sortDirection: this.sortDirection });
    },

    saveContent() {
      this.saveContentFn(this.block.id, { columns: this.columns, rows: this.rows });
    },

    addColumn() {
      const newCol = { id: crypto.randomUUID(), label: 'New Column' };
      this.columns = [...this.columns, newCol];
      this.saveContent();
    },

    removeColumn(idx) {
      const removedCol = this.columns[idx];
      this.columns = this.columns.filter((_, i) => i !== idx);
      // Also remove the column data from rows
      if (removedCol) {
        this.rows = this.rows.map(row => {
          const newRow = { ...row };
          delete newRow[removedCol.id];
          return newRow;
        });
      }
      this.saveContent();
    },

    addRow() {
      const newRow = { id: crypto.randomUUID() };
      this.rows = [...this.rows, newRow];
      this.saveContent();
    },

    removeRow(idx) {
      this.rows = this.rows.filter((_, i) => i !== idx);
      this.saveContent();
    },

    get sortedRows() {
      if (!this.sortColumn) return this.rows;
      const col = this.columns.find(c => c.id === this.sortColumn);
      if (!col) return this.rows;

      return [...this.rows].sort((a, b) => {
        const va = a[this.sortColumn] || '';
        const vb = b[this.sortColumn] || '';
        const cmp = va < vb ? -1 : va > vb ? 1 : 0;
        return this.sortDirection === 'asc' ? cmp : -cmp;
      });
    }
  };
}
