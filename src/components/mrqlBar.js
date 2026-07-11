import { createLiveRegion } from '../utils/ariaLiveRegion.js';

// The word currently being typed: a run of field-name characters ending at the
// cursor. Used to filter suggestions and to compute the replacement range when
// applying one (mirrors the CodeMirror completer's matchBefore on /mrql).
const WORD_RE = /[a-zA-Z_.]*$/;

const FORM_FIELDS = {
    resource: {
        Name: ['name', 'contains'], Description: ['description', 'contains'],
        OriginalName: ['originalName', 'contains'], Hash: ['hash', 'equals'],
        ContentType: ['contentType', 'contains'], OriginalLocation: ['originalLocation', 'contains'],
        ResourceCategoryId: ['category', 'number'],
        CreatedBefore: ['created', '<='], CreatedAfter: ['created', '>='],
        MinWidth: ['width', '>=number'], MaxWidth: ['width', '<=number'],
        MinHeight: ['height', '>=number'], MaxHeight: ['height', '<=number'],
        tags: ['tags', 'relation'], groups: ['groups', 'relation'],
        notes: ['notes', 'relation'], ownerId: ['owner', 'relation'],
    },
    note: {
        Name: ['name', 'contains'], Description: ['description', 'contains'],
        NoteTypeId: ['noteType', 'number'],
        StartDateBefore: ['startDate', '<='], StartDateAfter: ['startDate', '>='],
        EndDateBefore: ['endDate', '<='], EndDateAfter: ['endDate', '>='],
        Shared: ['shared', 'boolean'],
        tags: ['tags', 'relation'], groups: ['groups', 'relation'],
        ownerId: ['owner', 'relation'],
    },
    group: {
        Name: ['name', 'contains'], Description: ['description', 'contains'],
        URL: ['url', 'contains'], CreatedBefore: ['created', '<='], CreatedAfter: ['created', '>='],
        categories: ['category', 'number'], tags: ['tags', 'relation'],
        notes: ['notes', 'relation'], resources: ['resources', 'relation'],
        groups: ['children', 'relation'], ownerId: ['parent', 'relation'],
    },
};

const AUXILIARY_FORM_FIELDS = {
    resource: ['SortBy', 'IncludeSubgroups', 'ShowWithSimilar'],
    note: ['SortBy'],
    group: [
        'SortBy',
        'SearchParentsForName', 'SearchChildrenForName',
        'SearchParentsForTags', 'SearchChildrenForTags',
    ],
};

const SORT_TO_MRQL = {
    created_at: 'created', updated_at: 'updated', name: 'name', file_size: 'fileSize',
};
const SORT_FROM_MRQL = Object.fromEntries(Object.entries(SORT_TO_MRQL).map(([raw, mrql]) => [mrql, raw]));

function quoteMRQL(value) {
    return `"${String(value).replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`;
}

function unquoteMRQL(value) {
    if (!value.startsWith('"') || !value.endsWith('"')) return null;
    try { return JSON.parse(value); } catch (_) { return null; }
}

// Build the canonical MRQL subset represented by a sidebar form. Exported so
// the conversion contract can be covered without mounting Alpine.
const META_TO_MRQL_OPERATOR = {
    EQ: '=', LI: '~', NE: '!=', NL: '!~', GT: '>', GE: '>=', LT: '<', LE: '<=',
};
const MRQL_TO_META_OPERATOR = Object.fromEntries(
    Object.entries(META_TO_MRQL_OPERATOR).map(([meta, mrql]) => [mrql, meta]),
);

function parseMetaParam(raw) {
    const match = String(raw).match(/^([^:]+):([A-Z]{2}):(.*)$/s);
    if (!match || !META_TO_MRQL_OPERATOR[match[2]]) return null;
    let value;
    try { value = JSON.parse(match[3]); } catch (_) { value = match[3]; }
    return { name: match[1], operation: match[2], value };
}

function metaFieldToMRQL(entity, name) {
    if (entity === 'group' && name.startsWith('parent.')) return `parent.meta.${name.slice(7)}`;
    if (entity === 'group' && name.startsWith('child.')) return `children.meta.${name.slice(6)}`;
    return `meta.${name}`;
}

function metaFieldFromMRQL(entity, field) {
    if (field.startsWith('meta.')) return field.slice(5);
    if (entity === 'group' && field.startsWith('parent.meta.')) return `parent.${field.slice(12)}`;
    if (entity === 'group' && field.startsWith('children.meta.')) return `child.${field.slice(14)}`;
    return null;
}

function mrqlMetaLiteral(value) {
    if (value === null) return 'NULL';
    if (typeof value === 'number' || typeof value === 'boolean') return String(value);
    return quoteMRQL(value);
}

// freeFields uses a text model and derives JSON scalar types from its text.
// Keep falsy typed values visible, and quote coercible strings so editing an
// unrelated row cannot change or discard their type.
export function metadataRowsForFreeForm(metadata) {
    return metadata.map((entry) => {
        let value;
        if (entry.value === null) value = 'null';
        else if (typeof entry.value === 'string') {
            value = /^(?:true|false|null|-?\d+(?:\.\d+)?)$/i.test(entry.value)
                ? JSON.stringify(entry.value)
                : entry.value;
        } else value = String(entry.value);
        return { ...entry, value };
    });
}

function metadataValuesToMRQL(entity, formData) {
    const entries = [];
    for (const [name, raw] of formData.entries()) {
        if (name !== 'MetaQuery' && !/^MetaQuery\.\d+$/.test(name)) continue;
        const parsed = parseMetaParam(raw);
        if (parsed) entries.push(parsed);
    }

    const groupedEquals = new Map();
    for (const entry of entries) {
        const field = metaFieldToMRQL(entity, entry.name);
        if (!/^[a-zA-Z_][a-zA-Z0-9_.]*$/.test(field)) continue;
        if (entry.operation === 'EQ') {
            const values = groupedEquals.get(field) || [];
            values.push(entry.value);
            groupedEquals.set(field, values);
        }
    }

    const clauses = [];
    const emittedEquals = new Set();
    for (const entry of entries) {
        const field = metaFieldToMRQL(entity, entry.name);
        if (!/^[a-zA-Z_][a-zA-Z0-9_.]*$/.test(field)) continue;
        if (entry.operation === 'EQ') {
            if (emittedEquals.has(field)) continue;
            emittedEquals.add(field);
            const predicates = groupedEquals.get(field).map((value) => value === null
                ? `${field} IS NULL`
                : `${field} = ${mrqlMetaLiteral(value)}`);
            clauses.push(predicates.length > 1 ? `(${predicates.join(' OR ')})` : predicates[0]);
        } else if (entry.value === null) {
            if (entry.operation === 'NE') clauses.push(`${field} IS NOT NULL`);
        } else {
            let value = entry.value;
            if ((entry.operation === 'LI' || entry.operation === 'NL')) value = `*${value}*`;
            clauses.push(`${field} ${META_TO_MRQL_OPERATOR[entry.operation]} ${mrqlMetaLiteral(value)}`);
        }
    }
    return clauses;
}

export function formMetadataIsRepresentable(entity, formData) {
    for (const [name, raw] of formData.entries()) {
        if (name !== 'MetaQuery' && !/^MetaQuery\.\d+$/.test(name)) continue;
        const entry = parseMetaParam(raw);
        if (!entry) return false;
        const field = metaFieldToMRQL(entity, entry.name);
        if (!/^[a-zA-Z_][a-zA-Z0-9_.]*$/.test(field)) return false;
        if (entry.value === null && !['EQ', 'NE'].includes(entry.operation)) return false;
    }
    return true;
}

export function formValuesToMRQL(entity, formData, relationValues = new Map()) {
    const fields = FORM_FIELDS[entity] || {};
    const clauses = [];
    for (const [name, [field, kind]] of Object.entries(fields)) {
        const hasRelationNames = kind === 'relation' && relationValues.has(name);
        const rawValues = hasRelationNames
            ? relationValues.get(name)
            : formData.getAll(name);
        for (const raw of rawValues) {
            const value = String(raw || '').trim();
            if (!value) continue;
            if (entity === 'group' && name === 'Name') {
                const fieldsForName = ['name'];
                if (formData.get('SearchParentsForName')) fieldsForName.push('parent.name');
                if (formData.get('SearchChildrenForName')) fieldsForName.push('children.name');
                const expanded = fieldsForName.map((f) => `${f} ~ ${quoteMRQL(`*${value}*`)}`);
                clauses.push(expanded.length > 1 ? `(${expanded.join(' OR ')})` : expanded[0]);
            } else if (entity === 'group' && name === 'tags') {
                const fieldsForTag = ['tags'];
                if (formData.get('SearchParentsForTags')) fieldsForTag.push('parent.tags');
                if (formData.get('SearchChildrenForTags')) fieldsForTag.push('children.tags');
                const literal = !hasRelationNames && /^\d+$/.test(value)
                    ? Number(value) : quoteMRQL(value);
                const expanded = fieldsForTag.map((f) => `${f} = ${literal}`);
                clauses.push(expanded.length > 1 ? `(${expanded.join(' OR ')})` : expanded[0]);
            } else if (entity === 'resource' && name === 'ownerId' && formData.get('IncludeSubgroups')) {
                const literal = !hasRelationNames && /^\d+$/.test(value)
                    ? Number(value) : quoteMRQL(value);
                const ancestorField = !hasRelationNames && /^\d+$/.test(value)
                    ? 'ancestors.id' : 'ancestors.name';
                clauses.push(`(owner = ${literal} OR ${ancestorField} = ${literal})`);
            } else if (kind === 'contains') clauses.push(`${field} ~ ${quoteMRQL(`*${value}*`)}`);
            else if (kind === 'equals') clauses.push(`${field} = ${quoteMRQL(value)}`);
            else if (kind === 'number') clauses.push(`${field} = ${Number(value)}`);
            else if (kind === 'boolean') clauses.push(`${field} = true`);
            else if (kind === 'relation') {
                clauses.push(!hasRelationNames && /^\d+$/.test(value)
                    ? `${field} = ${Number(value)}`
                    : `${field} = ${quoteMRQL(value)}`);
            } else if (kind === '>=number') clauses.push(`${field} >= ${Number(value)}`);
            else if (kind === '<=number') clauses.push(`${field} <= ${Number(value)}`);
            else clauses.push(`${field} ${kind} ${quoteMRQL(value)}`);
        }
    }
    if (entity === 'resource' && formData.get('Untagged')) clauses.push('tags IS EMPTY');
    if (entity === 'resource' && formData.get('ShowWithSimilar')) clauses.push('similarImages IS NOT EMPTY');
    clauses.push(...metadataValuesToMRQL(entity, formData));
    const orderBy = sortValuesToMRQL(entity, formData.getAll('SortBy'));
    return clauses.join(' AND ') + (orderBy ? `${clauses.length ? ' ' : ''}ORDER BY ${orderBy}` : '');
}

function sortValuesToMRQL(entity, sortValues) {
    const parts = [];
    for (const rawValue of sortValues) {
        const match = String(rawValue).trim().match(/^(.*?)\s+(asc|desc)$/i);
        if (!match) continue;
        let field;
        const meta = match[1].match(/^meta->>'([a-z_]+)'$/);
        if (meta) field = `meta.${meta[1]}`;
        else field = SORT_TO_MRQL[match[1]];
        if (!field || (field === 'fileSize' && entity !== 'resource')) continue;
        parts.push(`${field} ${match[2].toUpperCase()}`);
    }
    return parts.join(', ');
}

function splitOrderBy(query) {
    const leading = query.match(/^ORDER\s+BY\s+/i);
    if (leading) return { filter: '', order: query.slice(leading[0].length).trim() };
    let depth = 0, quoted = false, escaped = false;
    for (let i = 0; i < query.length; i++) {
        const ch = query[i];
        if (quoted) {
            if (escaped) escaped = false;
            else if (ch === '\\') escaped = true;
            else if (ch === '"') quoted = false;
            continue;
        }
        if (ch === '"') quoted = true;
        else if (ch === '(') depth++;
        else if (ch === ')') depth--;
        else if (depth === 0) {
            const match = query.slice(i).match(/^\s+ORDER\s+BY\s+/i);
            if (match) return { filter: query.slice(0, i).trim(), order: query.slice(i + match[0].length).trim() };
        }
    }
    return { filter: query.trim(), order: '' };
}

function parseMRQLSort(entity, order) {
    if (!order) return { compatible: true, values: [] };
    const values = [];
    for (const part of order.split(',')) {
        const match = part.trim().match(/^([a-zA-Z_][a-zA-Z0-9_.]*)(?:\s+(ASC|DESC))?$/i);
        if (!match) return { compatible: false, values: [] };
        let raw;
        if (match[1].startsWith('meta.') && /^[a-z_]+$/.test(match[1].slice(5))) {
            raw = `meta->>'${match[1].slice(5)}'`;
        } else raw = SORT_FROM_MRQL[match[1]];
        if (!raw || (match[1] === 'fileSize' && entity !== 'resource')) return { compatible: false, values: [] };
        values.push(`${raw} ${(match[2] || 'ASC').toLowerCase()}`);
    }
    return { compatible: true, values };
}

function combineMRQL(query, formQuery) {
    if (!query) return formQuery;
    if (!formQuery) return query;
    const queryParts = splitOrderBy(query);
    const formParts = splitOrderBy(formQuery);
    const queryClauses = queryParts.filter ? splitAnd(queryParts.filter) : [];
    const formClauses = formParts.filter ? splitAnd(formParts.filter) : [];
    const filter = !queryClauses || !formClauses
        ? `(${queryParts.filter}) AND (${formParts.filter})`
        : [...new Set([...queryClauses, ...formClauses])].join(' AND ');
    const order = formParts.order || queryParts.order;
    return filter + (order ? `${filter ? ' ' : ''}ORDER BY ${order}` : '');
}

function removeTagFromMRQL(query, name) {
    const queryParts = splitOrderBy(query);
    const clauses = queryParts.filter ? splitAnd(queryParts.filter) : [];
    if (!clauses) return query;
    const literal = quoteMRQL(name);
    const kept = clauses.filter((clause) => {
        if (clause.trim() === `tags = ${literal}`) return false;
        const expanded = splitLogical(unwrapParens(clause), 'OR');
        if (!expanded || expanded.length < 2) return true;
        return !expanded.every((part) =>
            /^(?:tags|parent\.tags|children\.tags)\s*=\s*/.test(part) &&
            part.replace(/^[^=]+\s*=\s*/, '').trim() === literal);
    });
    const filter = kept.join(' AND ');
    return filter + (queryParts.order ? `${filter ? ' ' : ''}ORDER BY ${queryParts.order}` : '');
}

export function toggleQuickTagInMRQL(entity, query, name) {
    const translated = mrqlToFormValues(entity, query);
    if (!translated.compatible) return query;
    const active = (translated.values.get('tags') || [])
        .some((value) => String(value).toLowerCase() === String(name).toLowerCase());
    return active
        ? removeTagFromMRQL(query, name)
        : combineMRQL(query, `tags = ${quoteMRQL(name)}`);
}

function splitAnd(query) {
    return splitLogical(query, 'AND');
}

function splitLogical(query, operator) {
    const parts = [];
    let start = 0, depth = 0, quoted = false, escaped = false;
    for (let i = 0; i < query.length; i++) {
        const ch = query[i];
        if (quoted) {
            if (escaped) escaped = false;
            else if (ch === '\\') escaped = true;
            else if (ch === '"') quoted = false;
            continue;
        }
        if (ch === '"') quoted = true;
        else if (ch === '(') depth++;
        else if (ch === ')') depth--;
        else if (depth === 0 && new RegExp(`^\\s+${operator}\\s+`, 'i').test(query.slice(i))) {
            const match = query.slice(i).match(new RegExp(`^\\s+${operator}\\s+`, 'i'))[0];
            parts.push(query.slice(start, i).trim());
            i += match.length - 1;
            start = i + 1;
        }
    }
    if (depth !== 0 || quoted) return null;
    parts.push(query.slice(start).trim());
    return parts;
}

function unwrapParens(value) {
    const trimmed = value.trim();
    if (!trimmed.startsWith('(') || !trimmed.endsWith(')')) return trimmed;
    const inner = trimmed.slice(1, -1);
    return splitLogical(inner, 'OR') ? inner : trimmed;
}

function parseGroupExpansion(clause, values, nameLookups) {
    const parts = splitLogical(unwrapParens(clause), 'OR');
    if (!parts || parts.length < 2) return false;
    const matches = parts.map((part) => part.match(/^([a-zA-Z.]+)\s*(=|~)\s*(.+)$/));
    if (matches.some((match) => !match)) return false;
    const fields = matches.map((match) => match[1]);
    const operators = new Set(matches.map((match) => match[2]));
    const literals = new Set(matches.map((match) => match[3].trim()));
    if (operators.size !== 1 || literals.size !== 1) return false;

    const literal = matches[0][3].trim();
    if (fields.every((field) => ['name', 'parent.name', 'children.name'].includes(field)) && matches[0][2] === '~') {
        const parsed = unquoteMRQL(literal);
        if (parsed === null || !parsed.startsWith('*') || !parsed.endsWith('*') || !fields.includes('name')) return false;
        values.set('Name', [parsed.slice(1, -1)]);
        if (fields.includes('parent.name')) values.set('SearchParentsForName', ['1']);
        if (fields.includes('children.name')) values.set('SearchChildrenForName', ['1']);
        return true;
    }

    if (fields.every((field) => ['tags', 'parent.tags', 'children.tags'].includes(field)) && matches[0][2] === '=') {
        if (!fields.includes('tags')) return false;
        let parsed;
        if (/^\d+$/.test(literal)) parsed = literal;
        else {
            parsed = unquoteMRQL(literal);
            if (parsed === null) return false;
            nameLookups.add('tags');
        }
        values.set('tags', [...(values.get('tags') || []), parsed]);
        if (fields.includes('parent.tags')) values.set('SearchParentsForTags', ['1']);
        if (fields.includes('children.tags')) values.set('SearchChildrenForTags', ['1']);
        return true;
    }
    return false;
}

function parseResourceOwnerExpansion(clause, values, nameLookups) {
    const parts = splitLogical(unwrapParens(clause), 'OR');
    if (!parts || parts.length !== 2) return false;
    const matches = parts.map((part) => part.match(/^(owner|ancestors\.(?:id|name))\s*=\s*(.+)$/));
    if (matches.some((match) => !match)) return false;
    const byField = new Map(matches.map((match) => [match[1], match[2].trim()]));
    const ancestorField = byField.has('ancestors.id') ? 'ancestors.id' : 'ancestors.name';
    if (!byField.has('owner') || !byField.has(ancestorField) || byField.get('owner') !== byField.get(ancestorField)) return false;
    const literal = byField.get('owner');
    let parsed;
    if (/^\d+$/.test(literal)) parsed = literal;
    else {
        parsed = unquoteMRQL(literal);
        if (parsed === null) return false;
        nameLookups.add('ownerId');
    }
    values.set('ownerId', [parsed]);
    values.set('IncludeSubgroups', ['1']);
    return true;
}

function parseMetaLiteral(raw) {
    const value = raw.trim();
    if (/^-?\d+(?:\.\d+)?$/.test(value)) return Number(value);
    if (/^(true|false)$/i.test(value)) return value.toLowerCase() === 'true';
    const unquoted = unquoteMRQL(value);
    return unquoted === null ? undefined : unquoted;
}

function parseMetadataPredicate(entity, clause) {
    const isMatch = clause.match(/^([a-zA-Z_][a-zA-Z0-9_.]*)\s+IS\s+(NOT\s+)?NULL$/i);
    if (isMatch) {
        const name = metaFieldFromMRQL(entity, isMatch[1]);
        if (!name) return null;
        return [{ name, operation: isMatch[2] ? 'NE' : 'EQ', value: null }];
    }
    const match = clause.match(/^([a-zA-Z_][a-zA-Z0-9_.]*)\s*(!~|!=|>=|<=|=|~|>|<)\s*(.+)$/);
    if (!match) return null;
    const name = metaFieldFromMRQL(entity, match[1]);
    const operation = MRQL_TO_META_OPERATOR[match[2]];
    if (!name || !operation) return null;
    let value = parseMetaLiteral(match[3]);
    if (value === undefined) return null;
    if (operation === 'LI' || operation === 'NL') {
        if (typeof value !== 'string' || !value.startsWith('*') || !value.endsWith('*')) return null;
        value = value.slice(1, -1);
    }
    return [{ name, operation, value }];
}

function parseMetadataClause(entity, clause) {
    const parts = splitLogical(unwrapParens(clause), 'OR');
    if (!parts) return null;
    if (parts.length === 1) return parseMetadataPredicate(entity, parts[0]);
    const parsed = parts.flatMap((part) => parseMetadataPredicate(entity, part) || []);
    if (parsed.length !== parts.length || parsed.some((entry) => entry.operation !== 'EQ')) return null;
    if (new Set(parsed.map((entry) => entry.name)).size !== 1) return null;
    return parsed;
}

export function expandGroupMRQLFromParams(query, formData) {
    if (!query) return query;
    const expandName = formData.get('SearchParentsForName') || formData.get('SearchChildrenForName');
    const expandTags = formData.get('SearchParentsForTags') || formData.get('SearchChildrenForTags');
    if (!expandName && !expandTags) return query;
    const clauses = splitAnd(query);
    if (!clauses) return query;
    return clauses.map((clause) => {
        if (clause.trim().startsWith('(')) return clause;
        const match = clause.match(/^(name|tags)\s*(=|~)\s*(.+)$/);
        if (!match) return clause;
        const [, field, op, literal] = match;
        const expanded = [`${field} ${op} ${literal}`];
        if (field === 'name' && expandName) {
            if (formData.get('SearchParentsForName')) expanded.push(`parent.name ${op} ${literal}`);
            if (formData.get('SearchChildrenForName')) expanded.push(`children.name ${op} ${literal}`);
        } else if (field === 'tags' && expandTags) {
            if (formData.get('SearchParentsForTags')) expanded.push(`parent.tags ${op} ${literal}`);
            if (formData.get('SearchChildrenForTags')) expanded.push(`children.tags ${op} ${literal}`);
        }
        return expanded.length > 1 ? `(${expanded.join(' OR ')})` : clause;
    }).join(' AND ');
}

export function expandResourceMRQLFromParams(query, formData) {
    if (!query || !formData.get('IncludeSubgroups')) return query;
    const clauses = splitAnd(query);
    if (!clauses) return query;
    return clauses.map((clause) => {
        if (clause.trim().startsWith('(')) return clause;
        const match = clause.match(/^owner\s*=\s*(.+)$/);
        if (!match) return clause;
        const literal = match[1].trim();
        const ancestorField = /^\d+$/.test(literal) ? 'ancestors.id' : 'ancestors.name';
        return `(owner = ${literal} OR ${ancestorField} = ${literal})`;
    }).join(' AND ');
}

// Translate only expressions whose semantics are exactly available in the
// compact form. A false `compatible` result is deliberately conservative.
export function mrqlToFormValues(entity, query) {
    const reverse = new Map(Object.entries(FORM_FIELDS[entity] || {}).map(([name, spec]) => [spec.join('|'), name]));
    const values = new Map();
    const nameLookups = new Set();
    const metadata = [];
    const queryParts = splitOrderBy(query.trim());
    const sort = parseMRQLSort(entity, queryParts.order);
    if (!sort.compatible) return { compatible: false, values, nameLookups, metadata };
    if (sort.values.length) values.set('SortBy', sort.values);
    const trimmed = queryParts.filter;
    if (!trimmed) return { compatible: true, values, nameLookups, metadata };
    const clauses = splitAnd(trimmed);
    if (!clauses) return { compatible: false, values };
    for (const clause of clauses) {
        const metaEntries = parseMetadataClause(entity, clause);
        if (metaEntries) {
            metadata.push(...metaEntries);
            continue;
        }
        if (entity === 'group' && parseGroupExpansion(clause, values, nameLookups)) continue;
        if (entity === 'resource' && parseResourceOwnerExpansion(clause, values, nameLookups)) continue;
        if (entity === 'resource' && /^similarImages\s+IS\s+NOT\s+EMPTY$/i.test(clause)) {
            values.set('ShowWithSimilar', ['1']);
            continue;
        }
        if (entity === 'resource' && /^tags\s+IS\s+EMPTY$/i.test(clause)) {
            values.set('Untagged', ['1']);
            continue;
        }
        const match = clause.match(/^([a-zA-Z_][a-zA-Z0-9_.]*)\s*(!~|!=|>=|<=|=|~|>|<)\s*(.+)$/);
        if (!match) return { compatible: false, values };
        const [, field, op, literal] = match;
        let kind, value;
        if (op === '~') {
            value = unquoteMRQL(literal.trim());
            if (value === null || !value.startsWith('*') || !value.endsWith('*')) return { compatible: false, values };
            value = value.slice(1, -1); kind = 'contains';
        } else if (/^-?\d+(?:\.\d+)?$/.test(literal.trim())) {
            value = literal.trim();
            kind = op === '=' ? 'number' : `${op}number`;
        } else if (/^(true|false)$/i.test(literal.trim())) {
            value = literal.trim().toLowerCase();
            kind = 'boolean';
        } else {
            value = unquoteMRQL(literal.trim());
            if (value === null) return { compatible: false, values };
            kind = op === '=' ? 'equals' : op;
        }
        let name = reverse.get(`${field}|${kind}`);
        if (!name && kind === 'number') name = reverse.get(`${field}|relation`);
        // Relation controls store IDs, but MRQL also accepts an exact related
        // entity name. Resolve that name through the autocompleter endpoint.
        if (!name && kind === 'equals') {
            name = reverse.get(`${field}|relation`);
            if (name) nameLookups.add(name);
        }
        if (name === 'Shared') {
            const selectsShared = (op === '=' && value === 'true') || (op === '!=' && value === 'false');
            if (!selectsShared) return { compatible: false, values, nameLookups, metadata };
            value = '1';
        }
        if (!name) return { compatible: false, values };
        values.set(name, [...(values.get(name) || []), String(value)]);
    }
    return { compatible: true, values, nameLookups, metadata };
}

// mrqlBar is the list-page MRQL filter bar (package 5). A plain <input> (not
// CodeMirror — the list pages are hot and must not pull the editor chunks) with
// server-driven autocomplete and validation in filter mode, wired as an ARIA
// combobox. Submitting navigates the surrounding list to ?mrql=<expr>.
export function mrqlBar({ entity = 'resource', value = '', error = '' } = {}) {
    return {
        entity,
        query: value,
        // error holds the current inline validation message, seeded from the
        // server fail-closed banner and replaced by client-side validation.
        error,
        suggestions: [],
        selectedIndex: -1,
        open: false,
        _completeTimer: null,
        _validateTimer: null,
        _formSyncTimer: null,
        _liveRegion: null,
        filterForm: null,
        formCompatible: true,
        _formQuerySnapshot: '',
        _formMutationObserver: null,
        _applyingMRQL: false,

        init() {
            this._liveRegion = createLiveRegion();
            this.$nextTick(() => this.initFormSync());
        },

        destroy() {
            this._liveRegion?.destroy();
            clearTimeout(this._completeTimer);
            clearTimeout(this._validateTimer);
            clearTimeout(this._formSyncTimer);
            this._formMutationObserver?.disconnect();
            if (this.filterForm) {
                this.filterForm.removeEventListener('input', this._formChangeHandler);
                this.filterForm.removeEventListener('change', this._formChangeHandler);
                this.filterForm.removeEventListener('click', this._formChangeHandler);
                this.filterForm.removeEventListener('click', this._quickTagClickHandler);
                this.filterForm.removeEventListener('submit', this._formSubmitHandler);
            }
        },

        onInput() {
            this.scheduleComplete();
            this.scheduleValidate();
            clearTimeout(this._formSyncTimer);
            this._formSyncTimer = setTimeout(() => this.syncFormFromMRQL(), 550);
        },

        initFormSync() {
            this.filterForm = document.querySelector(`form[aria-label="Filter ${this.entity}s"]`);
            if (!this.filterForm) return;
            this._formChangeHandler = () => {
                if (this._applyingMRQL || !this.formCompatible) return;
                clearTimeout(this._formSyncTimer);
                this._formSyncTimer = setTimeout(() => this.syncMRQLFromForm(), 0);
            };
            this._quickTagClickHandler = (event) => this.onQuickTagClick(event);
            this.filterForm.addEventListener('click', this._quickTagClickHandler);
            this.filterForm.addEventListener('input', this._formChangeHandler);
            this.filterForm.addEventListener('change', this._formChangeHandler);
            this.filterForm.addEventListener('click', this._formChangeHandler);
            this._formSubmitHandler = () => {
                this.updateHiddenMRQL();
                // The canonical MRQL already contains these predicates. Keep
                // list-only controls (notably SortBy) but avoid double filters.
                for (const name of this.synchronizedFormFields()) {
                    if (name === 'SortBy') continue;
                    for (const control of this.filterForm.querySelectorAll(`[name="${CSS.escape(name)}"]`)) {
                        control.disabled = true;
                    }
                }
                for (const control of this.filterForm.querySelectorAll('[name="MetaQuery"], [name^="MetaQuery."]')) {
                    control.disabled = true;
                }
            };
            this.filterForm.addEventListener('submit', this._formSubmitHandler);
            this._formMutationObserver = new MutationObserver(this._formChangeHandler);
            this._formMutationObserver.observe(this.filterForm, { childList: true, subtree: true });

            const formQuery = formValuesToMRQL(
                this.entity, new FormData(this.filterForm), this.relationFormValues());
            this._formQuerySnapshot = formQuery;
            if (!formMetadataIsRepresentable(this.entity, new FormData(this.filterForm))) {
                this.query = formQuery;
                this.formCompatible = false;
                this.setFormDisabled(true);
                this.updateHiddenMRQL();
                return;
            }
            const explicitQuery = this.entity === 'group'
                ? expandGroupMRQLFromParams(this.query.trim(), new FormData(this.filterForm))
                : this.entity === 'resource'
                    ? expandResourceMRQLFromParams(this.query.trim(), new FormData(this.filterForm))
                    : this.query.trim();
            this.query = combineMRQL(explicitQuery, formQuery);
            if (this.query) {
                this.updateHiddenMRQL();
                this.syncFormFromMRQL();
            } else {
                this.updateHiddenMRQL();
                this.broadcastQuickTags();
            }
        },

        relationFormValues() {
            const values = new Map();
            if (!this.filterForm || !window.Alpine?.$data) return values;
            for (const [name, [, kind]] of Object.entries(FORM_FIELDS[this.entity] || {})) {
                if (kind !== 'relation') continue;
                const control = this.filterForm.querySelector(`[name="${CSS.escape(name)}"]`);
                const root = control?.closest('[x-data^="autocompleter"]');
                if (!root) continue;
                const selected = window.Alpine.$data(root).selectedResults || [];
                values.set(name, selected.map((item) => item.Name).filter(Boolean));
            }
            return values;
        },

        onQuickTagClick(event) {
            const anchor = event.target.closest?.('[data-filter-tag-name]');
            if (!anchor || !this.filterForm?.contains(anchor)) return;
            event.preventDefault();
            if (!this.formCompatible) return;
            const name = anchor.dataset.filterTagName;
            this.query = toggleQuickTagInMRQL(this.entity, this.query.trim(), name);
            this.updateHiddenMRQL();
            this.scheduleValidate();
            this.syncFormFromMRQL();
        },

        synchronizedFormFields() {
            return [
                ...Object.keys(FORM_FIELDS[this.entity] || {}),
                ...(AUXILIARY_FORM_FIELDS[this.entity] || []),
                'Untagged',
            ];
        },

        syncMetadataForm(metadata) {
            if (!this.filterForm || !window.Alpine?.$data) return;
            const freeRoot = [...this.filterForm.querySelectorAll('[x-data^="freeFields"]')]
                .find((root) => window.Alpine.$data(root).name === 'MetaQuery');
            if (freeRoot) {
                const data = window.Alpine.$data(freeRoot);
                data.fields = metadataRowsForFreeForm(metadata);
            }
            const schema = this.filterForm.querySelector('schema-search-mode');
            schema?.setMetaQueryEntries?.(metadata);
        },

        syncMRQLFromForm() {
            if (!this.filterForm || this._applyingMRQL) return;
            if (!formMetadataIsRepresentable(this.entity, new FormData(this.filterForm))) {
                this.formCompatible = false;
                this.setFormDisabled(true);
                return;
            }
            const query = formValuesToMRQL(
                this.entity, new FormData(this.filterForm), this.relationFormValues());
            this.query = query;
            this._formQuerySnapshot = query;
            this.updateHiddenMRQL();
            this.scheduleValidate();
            this.broadcastQuickTags();
        },

        broadcastQuickTags() {
            const names = this.relationFormValues().get('tags') || [];
            window.dispatchEvent(new CustomEvent('mrql-tags-change', { detail: { names } }));
        },

        async syncFormFromMRQL() {
            if (!this.filterForm) return;
            const translated = mrqlToFormValues(this.entity, this.query);
            if (!translated.compatible) {
                this.formCompatible = false;
                this.setFormDisabled(true);
                return;
            }
            this.formCompatible = true;
            this.setFormDisabled(false);
            this._applyingMRQL = true;
            try {
                this.syncMetadataForm(translated.metadata || []);
                for (const name of this.synchronizedFormFields()) {
                    const controls = [...this.filterForm.querySelectorAll(`[name="${CSS.escape(name)}"]`)];
                    // Autocompleters own dynamically-created hidden controls. If
                    // the requested IDs differ, update their Alpine state too.
                    const root = controls[0]?.closest('[x-data^="autocompleter"]');
                    const sortRoot = controls[0]?.closest('[x-data^="multiSort"]') ||
                        (name === 'SortBy' ? this.filterForm.querySelector('[x-data^="multiSort"]') : null);
                    if (name === 'SortBy' && sortRoot && window.Alpine?.$data) {
                        const data = window.Alpine.$data(sortRoot);
                        const rawValues = translated.values.get(name) || [];
                        data.sortColumns = rawValues.length
                            ? rawValues.map((value) => data.parseSort(value))
                            : [{ column: '', direction: 'desc', metaKey: '' }];
                        continue;
                    }
                    if (root && window.Alpine?.$data) {
                        const data = window.Alpine.$data(root);
                        const rawValues = translated.values.get(name) || [];
                        let selected;
                        if (translated.nameLookups.has(name)) {
                            selected = [];
                            for (const value of rawValues) {
                                const separator = data.url.includes('?') ? '&' : '?';
                                const response = await fetch(`${data.url}${separator}Name=${encodeURIComponent(value)}`);
                                if (!response.ok) { this.formCompatible = false; this.setFormDisabled(true); return; }
                                const results = await response.json();
                                const match = Array.isArray(results)
                                    ? results.find((item) => String(item.Name).toLowerCase() === value.toLowerCase())
                                    : null;
                                if (!match) { this.formCompatible = false; this.setFormDisabled(true); return; }
                                selected.push(match);
                            }
                        } else {
                            selected = rawValues.map(Number).map((ID) => ({ ID, Name: `#${ID}` }));
                        }
                        data.resetSelectedResults(selected);
                        continue;
                    }
                    for (const control of controls) {
                        if (control.type === 'checkbox') control.checked = translated.values.has(name);
                        else control.value = translated.values.get(name)?.[0] || '';
                    }
                }
                this._formQuerySnapshot = this.query.trim();
                this.updateHiddenMRQL();
            } catch (_) {
                this.formCompatible = false;
                this.setFormDisabled(true);
            } finally {
                this.$nextTick(() => {
                    this._applyingMRQL = false;
                    this.broadcastQuickTags();
                });
            }
        },

        setFormDisabled(disabled) {
            if (!this.filterForm) return;
            for (const control of this.filterForm.elements) {
                if (control.name === 'mrql') continue;
                control.disabled = disabled;
            }
            this.filterForm.inert = disabled;
            this.filterForm.setAttribute('aria-disabled', disabled ? 'true' : 'false');
        },

        useFormValues() {
            this.query = this._formQuerySnapshot;
            this.formCompatible = true;
            this.setFormDisabled(false);
            this.updateHiddenMRQL();
            this.scheduleValidate();
            // A failed async relation lookup may have partially updated earlier
            // controls. Re-apply the preserved canonical snapshot atomically.
            this.$nextTick(async () => {
                await this.syncFormFromMRQL();
                this.$refs.input?.focus();
            });
        },

        updateHiddenMRQL() {
            if (!this.filterForm) return;
            let hidden = this.filterForm.querySelector('input[type="hidden"][name="mrql"]');
            if (!hidden) {
                hidden = document.createElement('input');
                hidden.type = 'hidden';
                hidden.name = 'mrql';
                this.filterForm.appendChild(hidden);
            }
            hidden.value = splitOrderBy(this.query.trim()).filter;
        },

        scheduleComplete() {
            clearTimeout(this._completeTimer);
            this._completeTimer = setTimeout(() => this.fetchSuggestions(), 150);
        },

        scheduleValidate() {
            clearTimeout(this._validateTimer);
            this._validateTimer = setTimeout(() => this.validate(), 500);
        },

        cursorPos() {
            return this.$refs.input ? this.$refs.input.selectionStart : this.query.length;
        },

        currentWord(cursor) {
            const m = this.query.slice(0, cursor).match(WORD_RE);
            return m ? m[0] : '';
        },

        async fetchSuggestions() {
            const cursor = this.cursorPos();
            try {
                const resp = await fetch('/v1/mrql/complete', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ query: this.query, cursor, entityType: this.entity, filter: true }),
                });
                if (!resp.ok) { this.closeSuggestions(); return; }
                const data = await resp.json();
                let sugg = data.suggestions || [];
                // Narrow to the token currently being typed (the server returns
                // the full candidate list; the client filters, as /mrql does).
                const word = this.currentWord(cursor);
                if (word) {
                    const lw = word.toLowerCase();
                    sugg = sugg.filter((s) => s.value.toLowerCase().startsWith(lw));
                }
                this.suggestions = sugg.slice(0, 20);
                this.open = this.suggestions.length > 0;
                this.selectedIndex = this.open ? 0 : -1;
                if (this.open) {
                    this._liveRegion?.announce(
                        `${this.suggestions.length} suggestion${this.suggestions.length === 1 ? '' : 's'} available.`);
                }
            } catch (_) {
                this.closeSuggestions();
            }
        },

        async validate() {
            const query = splitOrderBy(this.query.trim()).filter;
            if (!query) { this.error = ''; return; }
            try {
                const resp = await fetch('/v1/mrql/validate', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ query, entityType: this.entity, filter: true }),
                });
                if (!resp.ok) return;
                const data = await resp.json();
                if (data.valid) {
                    this.error = '';
                } else if (data.errors && data.errors.length > 0) {
                    this.error = data.errors.map((e) => e.message || 'Invalid filter').join('; ');
                } else {
                    this.error = 'Invalid filter';
                }
            } catch (_) {
                // Network error — leave the last known state untouched.
            }
        },

        navigateDown() {
            if (this.suggestions.length === 0) return;
            this.open = true;
            this.selectedIndex = (this.selectedIndex + 1) % this.suggestions.length;
            this.announceOption();
        },

        navigateUp() {
            if (this.suggestions.length === 0 || !this.open) return;
            this.selectedIndex = this.selectedIndex <= 0
                ? this.suggestions.length - 1
                : this.selectedIndex - 1;
            this.announceOption();
        },

        announceOption() {
            const s = this.suggestions[this.selectedIndex];
            if (s) {
                this._liveRegion?.announce(
                    `${s.value}${s.label ? ', ' + s.label : ''}, ${this.selectedIndex + 1} of ${this.suggestions.length}`);
            }
        },

        applySuggestion(i) {
            const s = this.suggestions[i];
            if (!s) return;
            const cursor = this.cursorPos();
            const before = this.query.slice(0, cursor);
            const wordMatch = before.match(WORD_RE);
            const start = cursor - (wordMatch ? wordMatch[0].length : 0);
            const after = this.query.slice(cursor);
            this.query = this.query.slice(0, start) + s.value + after;
            this.closeSuggestions();
            this.$nextTick(() => {
                const pos = start + s.value.length;
                if (this.$refs.input) {
                    this.$refs.input.focus();
                    this.$refs.input.setSelectionRange(pos, pos);
                }
                this.scheduleValidate();
            });
        },

        onEnter(e) {
            // With the suggestion popup open and an option highlighted, Enter
            // accepts it; otherwise it falls through to submit the GET form.
            if (this.open && this.selectedIndex >= 0) {
                e.preventDefault();
                this.applySuggestion(this.selectedIndex);
            }
        },

        onBlur() {
            // Delay so a mousedown on an option registers before the list hides.
            setTimeout(() => this.closeSuggestions(), 150);
        },

        closeSuggestions() {
            this.open = false;
            this.selectedIndex = -1;
            this.suggestions = [];
        },

        // submit navigates the current list to ?mrql=<expr>, preserving every
        // existing sidebar parameter and resetting to page 1. Clearing the input
        // and submitting removes the parameter.
        submit() {
            const params = new URLSearchParams(window.location.search);
            const queryParts = splitOrderBy(this.query.trim());
            const val = queryParts.filter;
            // MRQL is now the single filter source of truth. Remove sidebar
            // predicates so the same filter is not applied twice on navigation.
            for (const name of this.synchronizedFormFields()) params.delete(name);
            for (const name of [...params.keys()]) {
                if (name === 'MetaQuery' || name.startsWith('MetaQuery.')) params.delete(name);
            }
            params.delete('Untagged');
            const sort = parseMRQLSort(this.entity, queryParts.order);
            if (sort.compatible) for (const value of sort.values) params.append('SortBy', value);
            if (val) {
                params.set('mrql', val);
            } else {
                params.delete('mrql');
            }
            params.delete('page');
            const qs = params.toString();
            window.location.assign(window.location.pathname + (qs ? '?' + qs : ''));
        },

        // editorLink graduates the current filter to the full /mrql editor by
        // wrapping it with the page's implied entity type.
        editorLink() {
            const queryParts = splitOrderBy(this.query.trim());
            const inner = (queryParts.filter
                ? `type = ${this.entity} AND (${queryParts.filter})`
                : `type = ${this.entity}`) + (queryParts.order ? ` ORDER BY ${queryParts.order}` : '');
            return '/mrql?q=' + encodeURIComponent(inner);
        },

        activeDescendant() {
            return this.open && this.selectedIndex >= 0
                ? `${this.$id('mrql-bar')}-opt-${this.selectedIndex}`
                : null;
        },
    };
}
