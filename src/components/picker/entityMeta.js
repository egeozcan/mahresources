// src/components/picker/entityMeta.js

/**
 * Fetch metadata for entities to display in blocks.
 * Returns an object keyed by ID with entity-specific metadata.
 */
export async function fetchEntityMeta(entityType, ids) {
  if (!ids || ids.length === 0) return {};

  const fetchers = {
    resource: fetchResourceMeta,
    group: fetchGroupMeta
  };

  const fetcher = fetchers[entityType];
  if (!fetcher) {
    console.warn(`No metadata fetcher for entity type: ${entityType}`);
    return {};
  }

  return fetcher(ids);
}

async function fetchResourceMeta(ids) {
  const meta = {};
  const toFetch = ids.filter(id => id != null);
  if (toFetch.length === 0) return meta;

  try {
    const promises = toFetch.map(id =>
      fetch(`/v1/resource?id=${id}`).then(r => r.ok ? r.json() : null)
    );
    const results = await Promise.all(promises);
    results.forEach((res, i) => {
      if (res) {
        meta[toFetch[i]] = {
          contentType: res.ContentType || '',
          name: res.Name || '',
          hash: res.Hash || ''
        };
      }
    });
  } catch (err) {
    console.warn('Failed to fetch resource metadata:', err);
  }

  return meta;
}

async function fetchGroupMeta(ids) {
  const meta = {};
  const toFetch = ids.filter(id => id != null);
  if (toFetch.length === 0) return meta;

  try {
    const promises = toFetch.map(id =>
      fetch(`/v1/group?id=${id}`).then(r => r.ok ? r.json() : null)
    );
    const results = await Promise.all(promises);
    results.forEach((res, i) => {
      if (res) {
        meta[toFetch[i]] = {
          name: res.Name || '',
          breadcrumb: buildBreadcrumb(res),
          resourceCount: res.ResourceCount || 0,
          noteCount: res.NoteCount || 0,
          mainResourceId: res.MainResource?.ID || null,
          categoryName: res.Category?.Name || ''
        };
      }
    });
  } catch (err) {
    console.warn('Failed to fetch group metadata:', err);
  }

  return meta;
}

function buildBreadcrumb(group) {
  const parts = [];
  let current = group.Owner;
  let depth = 0;
  const maxDepth = 3;

  while (current && depth < maxDepth) {
    parts.unshift(current.Name);
    current = current.Owner;
    depth++;
  }

  if (current) {
    // More ancestors exist
    parts.unshift('...');
  }

  return parts.join(' > ');
}
