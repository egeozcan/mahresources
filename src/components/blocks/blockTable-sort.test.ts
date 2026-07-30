import { describe, it, expect } from 'vitest';
import { blockTable } from './blockTable.js';

// WS11 round 3, finding 5's client half.
//
// GET /v1/note/block/table/query used to key each row by the raw column name, so
// `SELECT 10 AS dup, 20 AS dup` lost a value and `SELECT 42 AS id` was destroyed
// by the synthetic row key the endpoint adds. The ids are positional now
// (`col_0`, `col_1`, …) with the real name in `label` — the same spelling
// _normalizeColumns already generates for a legacy manual table's columns.
//
// That renames what `sortColumn` refers to, and `sortColumn` is *persisted* block
// state, so a block sorted before the change holds a column name where a column
// id is now expected.

function tableWith(queryColumns: unknown[], queryRows: unknown[], sortColumn: string) {
  const b = blockTable(
    { id: 1, content: { queryId: 7 }, state: { sortColumn, sortDirection: 'asc' } },
    () => {},
    () => {},
    () => false,
    () => {}
  );
  // init() would fire the network fetch; these tests are about the pure sort.
  b.queryColumns = queryColumns;
  b.queryRows = queryRows;
  return b;
}

const columns = [
  { id: 'col_0', label: 'name' },
  { id: 'col_1', label: 'size' },
];
const rows = [
  { id: 'row_0', col_0: 'beta', col_1: 20 },
  { id: 'row_1', col_0: 'alpha', col_1: 100 },
];

describe('table block sorting after positional column ids', () => {
  it('sorts by a positional column id', () => {
    const b = tableWith(columns, rows, 'col_0');
    expect(b.displayRows.map((r: any) => r.col_0)).toEqual(['alpha', 'beta']);
  });

  it('still sorts a block whose saved state names the column instead of its id', () => {
    const b = tableWith(columns, rows, 'name');
    expect(b.displayRows.map((r: any) => r.col_0)).toEqual(['alpha', 'beta']);
  });

  it('leaves the rows alone when the saved column is gone from the query', () => {
    const b = tableWith(columns, rows, 'a_column_the_query_no_longer_selects');
    // Positive control: unsorted means the server's order, which is the reverse
    // of the sorted order above — so "unchanged" is a real observation here.
    expect(b.displayRows.map((r: any) => r.col_0)).toEqual(['beta', 'alpha']);
  });

  it('sorts numerically, not lexically, on the second column', () => {
    const b = tableWith(columns, rows, 'col_1');
    expect(b.displayRows.map((r: any) => r.col_1)).toEqual([20, 100]);
  });
});
