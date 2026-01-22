package application_context

import (
	"container/list"
	"mahresources/models/query_models"
	"strings"
	"sync"
	"time"
)

// SearchCache provides an LRU cache for global search results with TTL and type-based invalidation
type SearchCache struct {
	cache     map[string]*list.Element
	lruList   *list.List
	mu        sync.RWMutex
	ttl       time.Duration
	maxSize   int
}

type searchCacheEntry struct {
	key       string
	results   []query_models.SearchResultItem
	timestamp time.Time
	types     map[string]bool // Track which entity types are in this result
}

// NewSearchCache creates a new search cache with the specified TTL and max entries
func NewSearchCache(ttl time.Duration, maxSize int) *SearchCache {
	return &SearchCache{
		cache:   make(map[string]*list.Element),
		lruList: list.New(),
		ttl:     ttl,
		maxSize: maxSize,
	}
}

// Get retrieves cached results for a query, returning nil if not found or expired
func (c *SearchCache) Get(query string) ([]query_models.SearchResultItem, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := strings.ToLower(strings.TrimSpace(query))
	elem, ok := c.cache[key]
	if !ok {
		return nil, false
	}

	entry := elem.Value.(*searchCacheEntry)
	if time.Since(entry.timestamp) > c.ttl {
		// Entry expired - clean it up now
		delete(c.cache, entry.key)
		c.lruList.Remove(elem)
		return nil, false
	}

	// Move to front (most recently used)
	c.lruList.MoveToFront(elem)

	// Return a copy to avoid data races
	resultsCopy := make([]query_models.SearchResultItem, len(entry.results))
	copy(resultsCopy, entry.results)

	return resultsCopy, true
}

// Set stores results for a query in the cache
func (c *SearchCache) Set(query string, results []query_models.SearchResultItem) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := strings.ToLower(strings.TrimSpace(query))

	// Track which entity types are in this result set
	types := make(map[string]bool)
	for _, r := range results {
		types[r.Type] = true
	}

	// Make a copy of the results to store
	resultsCopy := make([]query_models.SearchResultItem, len(results))
	copy(resultsCopy, results)

	entry := &searchCacheEntry{
		key:       key,
		results:   resultsCopy,
		timestamp: time.Now(),
		types:     types,
	}

	// Check if key already exists
	if elem, ok := c.cache[key]; ok {
		// Update existing entry
		c.lruList.MoveToFront(elem)
		elem.Value = entry
		return
	}

	// Evict oldest entries if at capacity
	for c.lruList.Len() >= c.maxSize {
		c.evictOldest()
	}

	// Add new entry
	elem := c.lruList.PushFront(entry)
	c.cache[key] = elem
}

// InvalidateByType removes all cached entries that contain results of the specified entity type
func (c *SearchCache) InvalidateByType(entityType string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var toRemove []*list.Element

	for elem := c.lruList.Front(); elem != nil; elem = elem.Next() {
		entry := elem.Value.(*searchCacheEntry)
		if entry.types[entityType] {
			toRemove = append(toRemove, elem)
		}
	}

	for _, elem := range toRemove {
		entry := elem.Value.(*searchCacheEntry)
		delete(c.cache, entry.key)
		c.lruList.Remove(elem)
	}
}

// Clear removes all entries from the cache
func (c *SearchCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[string]*list.Element)
	c.lruList.Init()
}

// Size returns the current number of entries in the cache
func (c *SearchCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lruList.Len()
}

// evictOldest removes the least recently used entry (must be called with lock held)
func (c *SearchCache) evictOldest() {
	elem := c.lruList.Back()
	if elem != nil {
		entry := elem.Value.(*searchCacheEntry)
		delete(c.cache, entry.key)
		c.lruList.Remove(elem)
	}
}

