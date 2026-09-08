// Locate top-level clauses without mistaking quoted values or field paths for
// keywords. Sorting edits only ORDER BY; authored result bounds stay intact.
export function replaceMRQLSort(query, order) {
    let quote = null, depth = 0, orderAt = -1, boundsAt = -1;
    for (let i = 0; i < query.length; i++) {
        const c = query[i];
        if (quote) {
            if (c === '\\') i++;
            else if (c === quote) quote = null;
            continue;
        }
        if (c === '"' || c === "'") { quote = c; continue; }
        if (c === '(') { depth++; continue; }
        if (c === ')') { depth--; continue; }
        if (depth || (i > 0 && /[\w.$]/.test(query[i - 1]))) continue;
        const rest = query.slice(i);
        if (/^ORDER\s+BY\b/i.test(rest)) orderAt = i;
        if (/^(LIMIT|OFFSET)\s+\d/i.test(rest)) { boundsAt = i; break; }
    }
    const end = orderAt >= 0 ? orderAt : boundsAt >= 0 ? boundsAt : query.length;
    return [query.slice(0, end).trim(), `ORDER BY ${order}`, boundsAt >= 0 ? query.slice(boundsAt).trim() : ''].filter(Boolean).join(' ');
}
