// src/components/picker/entityMeta.js

/**
 * Fetch metadata for entities to display in blocks.
 * Returns an object keyed by ID with entity-specific metadata.
 * Uses batched concurrent requests to avoid overwhelming the server.
 * Includes caching to avoid redundant fetches and retry logic for transient failures.
 */

const BATCH_SIZE = 5; // Max concurrent requests per batch
const MAX_RETRIES = 2; // Max retry attempts for failed requests
const RETRY_DELAY_MS = 500; // Delay between retries
const CACHE_TTL_MS = 5 * 60 * 1000; // 5 minute cache TTL

// In-memory cache keyed by entityType:id
const metaCache = new Map();

/**
 * Clear the metadata cache. Useful for testing or when data is known to have changed.
 * @param {string} [entityType] - Optional entity type to clear. If omitted, clears all.
 */
export function clearMetaCache(entityType) {
  if (entityType) {
    for (const key of metaCache.keys()) {
      if (key.startsWith(`${entityType}:`)) {
        metaCache.delete(key);
      }
    }
  } else {
    metaCache.clear();
  }
}

/**
 * Get a cached entry if it exists and is not expired.
 */
function getCached(entityType, id) {
  const key = `${entityType}:${id}`;
  const entry = metaCache.get(key);
  if (entry && Date.now() - entry.timestamp < CACHE_TTL_MS) {
    return entry.data;
  }
  return null;
}

/**
 * Set a cache entry.
 */
function setCache(entityType, id, data) {
  const key = `${entityType}:${id}`;
  metaCache.set(key, { data, timestamp: Date.now() });
}

/**
 * Sleep for a given number of milliseconds.
 */
function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

/**
 * Fetch with retry logic for transient failures.
 */
async function fetchWithRetry(url, retries = MAX_RETRIES) {
  let lastError;
  for (let attempt = 0; attempt <= retries; attempt++) {
    try {
      const res = await fetch(url);
      if (res.ok) {
        return await res.json();
      }
      // Non-retryable HTTP error (4xx)
      if (res.status >= 400 && res.status < 500) {
        return null;
      }
      // Server error (5xx) - retryable
      lastError = new Error(`HTTP ${res.status}`);
    } catch (err) {
      // Network error - retryable
      lastError = err;
    }

    if (attempt < retries) {
      await sleep(RETRY_DELAY_MS * (attempt + 1)); // Exponential backoff
    }
  }
  throw lastError;
}

/**
 * Execute promises in batches to limit concurrency.
 * @param {Array<() => Promise>} promiseFns - Array of functions returning promises
 * @param {number} batchSize - Max concurrent promises
 * @returns {Promise<Array>} - Results in order
 */
async function batchedPromises(promiseFns, batchSize = BATCH_SIZE) {
  const results = [];
  for (let i = 0; i < promiseFns.length; i += batchSize) {
    const batch = promiseFns.slice(i, i + batchSize);
    const batchResults = await Promise.all(batch.map(fn => fn()));
    results.push(...batchResults);
  }
  return results;
}

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
  const toFetch = [];

  // Check cache first
  for (const id of ids) {
    if (id == null) continue;
    const cached = getCached('resource', id);
    if (cached) {
      meta[id] = cached;
    } else {
      toFetch.push(id);
    }
  }

  if (toFetch.length === 0) return meta;

  try {
    const promiseFns = toFetch.map(id => async () => {
      try {
        const res = await fetchWithRetry(`/v1/resource?id=${id}`);
        return { id, res };
      } catch (err) {
        console.warn(`Failed to fetch resource ${id} after retries:`, err);
        return { id, res: null };
      }
    });
    const results = await batchedPromises(promiseFns);
    results.forEach(({ id, res }) => {
      if (res) {
        const data = {
          contentType: res.ContentType || '',
          name: res.Name || '',
          hash: res.Hash || ''
        };
        meta[id] = data;
        setCache('resource', id, data);
      }
    });
  } catch (err) {
    console.warn('Failed to fetch resource metadata:', err);
  }

  return meta;
}

async function fetchGroupMeta(ids) {
  const meta = {};
  const toFetch = [];

  // Check cache first
  for (const id of ids) {
    if (id == null) continue;
    const cached = getCached('group', id);
    if (cached) {
      meta[id] = cached;
    } else {
      toFetch.push(id);
    }
  }

  if (toFetch.length === 0) return meta;

  try {
    const promiseFns = toFetch.map(id => async () => {
      try {
        const res = await fetchWithRetry(`/v1/group?id=${id}`);
        return { id, res };
      } catch (err) {
        console.warn(`Failed to fetch group ${id} after retries:`, err);
        return { id, res: null };
      }
    });
    const results = await batchedPromises(promiseFns);
    results.forEach(({ id, res }) => {
      if (res) {
        const data = {
          name: res.Name || '',
          breadcrumb: buildBreadcrumb(res),
          resourceCount: res.ResourceCount || 0,
          noteCount: res.NoteCount || 0,
          mainResourceId: res.MainResource?.ID || null,
          categoryName: res.Category?.Name || ''
        };
        meta[id] = data;
        setCache('group', id, data);
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
