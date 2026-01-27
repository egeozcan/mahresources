# Image Similarity Background Processing - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Integrate perceptual hash calculation into the main app as a background worker, replacing the standalone CLI tool, and add visual similarity matching with Hamming distance thresholds.

**Architecture:** A `HashWorker` runs as a background goroutine with configurable concurrency. It processes unhashed resources in batches, calculates perceptual hashes, finds similar resources using Hamming distance, and stores similarity pairs in a dedicated table. New uploads are queued for immediate async processing.

**Tech Stack:** Go, GORM, `github.com/Nr90/imgsim`, `math/bits` for Hamming distance

---

## Task 1: Add ResourceSimilarity Model

**Files:**
- Create: `models/resource_similarity_model.go`

**Step 1: Create the model file**

```go
package models

// ResourceSimilarity stores pre-computed similarity pairs between resources.
// ResourceID1 is always less than ResourceID2 to avoid duplicate pairs.
type ResourceSimilarity struct {
	ID              uint      `gorm:"primarykey"`
	ResourceID1     uint      `gorm:"index:idx_sim_r1;uniqueIndex:idx_sim_pair"`
	ResourceID2     uint      `gorm:"index:idx_sim_r2;uniqueIndex:idx_sim_pair"`
	HammingDistance uint8
	Resource1       *Resource `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignKey:ResourceID1"`
	Resource2       *Resource `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignKey:ResourceID2"`
}
```

**Step 2: Run build to verify syntax**

Run: `go build --tags 'json1 fts5' ./...`
Expected: No errors

**Step 3: Commit**

```bash
git add models/resource_similarity_model.go
git commit -m "feat: add ResourceSimilarity model for pre-computed similarity pairs"
```

---

## Task 2: Update ImageHash Model with uint64 Columns

**Files:**
- Modify: `models/image_hash_model.go`

**Step 1: Add new uint64 columns alongside existing string columns**

Replace the entire file content:

```go
package models

import "strconv"

type ImageHash struct {
	ID         uint      `gorm:"primarykey"`
	AHash      string    `gorm:"index"`  // old, kept during migration
	DHash      string    `gorm:"index"`  // old, kept during migration
	AHashInt   *uint64   `gorm:"index"`  // new uint64 column
	DHashInt   *uint64   `gorm:"index"`  // new uint64 column
	Resource   *Resource `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	ResourceId *uint     `gorm:"uniqueIndex"`
}

// GetDHash returns the DHash as uint64, preferring the new column
// and falling back to parsing the old string column.
func (h *ImageHash) GetDHash() uint64 {
	if h.DHashInt != nil {
		return *h.DHashInt
	}
	if h.DHash == "" {
		return 0
	}
	val, _ := strconv.ParseUint(h.DHash, 16, 64)
	return val
}

// GetAHash returns the AHash as uint64, preferring the new column
// and falling back to parsing the old string column.
func (h *ImageHash) GetAHash() uint64 {
	if h.AHashInt != nil {
		return *h.AHashInt
	}
	if h.AHash == "" {
		return 0
	}
	val, _ := strconv.ParseUint(h.AHash, 16, 64)
	return val
}

// IsMigrated returns true if this hash has been migrated to uint64 format.
func (h *ImageHash) IsMigrated() bool {
	return h.DHashInt != nil
}
```

**Step 2: Run build to verify syntax**

Run: `go build --tags 'json1 fts5' ./...`
Expected: No errors

**Step 3: Commit**

```bash
git add models/image_hash_model.go
git commit -m "feat: add uint64 hash columns to ImageHash model for efficient Hamming distance"
```

---

## Task 3: Add ResourceSimilarity to AutoMigrate

**Files:**
- Modify: `main.go:152-168`

**Step 1: Add ResourceSimilarity to the AutoMigrate call**

Find this section in main.go:

```go
	if err := db.AutoMigrate(
		&models.Query{},
		&models.Resource{},
		&models.ResourceVersion{},
		&models.Note{},
		&models.Tag{},
		&models.Group{},
		&models.Category{},
		&models.NoteType{},
		&models.Preview{},
		&models.GroupRelation{},
		&models.GroupRelationType{},
		&models.ImageHash{},
		&models.LogEntry{},
	); err != nil {
```

Add `&models.ResourceSimilarity{},` after `&models.ImageHash{},`:

```go
	if err := db.AutoMigrate(
		&models.Query{},
		&models.Resource{},
		&models.ResourceVersion{},
		&models.Note{},
		&models.Tag{},
		&models.Group{},
		&models.Category{},
		&models.NoteType{},
		&models.Preview{},
		&models.GroupRelation{},
		&models.GroupRelationType{},
		&models.ImageHash{},
		&models.ResourceSimilarity{},
		&models.LogEntry{},
	); err != nil {
```

**Step 2: Run build to verify**

Run: `go build --tags 'json1 fts5'`
Expected: No errors

**Step 3: Commit**

```bash
git add main.go
git commit -m "feat: add ResourceSimilarity to AutoMigrate"
```

---

## Task 4: Create HashWorker Config Struct

**Files:**
- Create: `hash_worker/config.go`

**Step 1: Create the config file**

```go
package hash_worker

import "time"

// Config holds configuration for the HashWorker.
type Config struct {
	// WorkerCount is the number of concurrent hash calculation workers.
	WorkerCount int
	// BatchSize is the number of resources to process per batch cycle.
	BatchSize int
	// PollInterval is the time between batch processing cycles.
	PollInterval time.Duration
	// SimilarityThreshold is the maximum Hamming distance to consider resources similar.
	SimilarityThreshold int
	// Disabled prevents the hash worker from starting.
	Disabled bool
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		WorkerCount:         4,
		BatchSize:           500,
		PollInterval:        time.Minute,
		SimilarityThreshold: 10,
		Disabled:            false,
	}
}
```

**Step 2: Run build to verify**

Run: `go build --tags 'json1 fts5' ./...`
Expected: No errors

**Step 3: Commit**

```bash
git add hash_worker/config.go
git commit -m "feat: add HashWorker config struct"
```

---

## Task 5: Create Hashable Content Types Helper

**Files:**
- Create: `hash_worker/content_types.go`

**Step 1: Create the content types file**

```go
package hash_worker

// HashableContentTypes is the set of content types that can be perceptually hashed.
var HashableContentTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

// IsHashable returns true if the content type can be perceptually hashed.
func IsHashable(contentType string) bool {
	return HashableContentTypes[contentType]
}
```

**Step 2: Run build to verify**

Run: `go build --tags 'json1 fts5' ./...`
Expected: No errors

**Step 3: Commit**

```bash
git add hash_worker/content_types.go
git commit -m "feat: add hashable content types helper"
```

---

## Task 6: Create Hamming Distance Function

**Files:**
- Create: `hash_worker/hamming.go`
- Create: `hash_worker/hamming_test.go`

**Step 1: Write the failing test**

Create `hash_worker/hamming_test.go`:

```go
package hash_worker

import "testing"

func TestHammingDistance(t *testing.T) {
	tests := []struct {
		name     string
		a, b     uint64
		expected int
	}{
		{"identical", 0xFFFFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF, 0},
		{"completely different", 0x0, 0xFFFFFFFFFFFFFFFF, 64},
		{"one bit different", 0x0, 0x1, 1},
		{"half bits different", 0xAAAAAAAAAAAAAAAA, 0x5555555555555555, 64},
		{"few bits different", 0xFFFFFFFFFFFFFFF0, 0xFFFFFFFFFFFFFFFF, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HammingDistance(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("HammingDistance(%x, %x) = %d, want %d", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./hash_worker/... -v`
Expected: FAIL (function not defined)

**Step 3: Write implementation**

Create `hash_worker/hamming.go`:

```go
package hash_worker

import "math/bits"

// HammingDistance returns the number of bit positions where two uint64 values differ.
func HammingDistance(a, b uint64) int {
	return bits.OnesCount64(a ^ b)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./hash_worker/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add hash_worker/hamming.go hash_worker/hamming_test.go
git commit -m "feat: add HammingDistance function with tests"
```

---

## Task 7: Create HashWorker Core Structure

**Files:**
- Create: `hash_worker/worker.go`

**Step 1: Create the worker structure**

```go
package hash_worker

import (
	"image"
	"log"
	"sync"
	"time"

	// Register image formats
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"

	"github.com/Nr90/imgsim"
	"github.com/spf13/afero"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mahresources/models"
)

// HashWorker processes resources to calculate perceptual hashes and find similarities.
type HashWorker struct {
	db     *gorm.DB
	fs     afero.Fs
	altFS  map[string]afero.Fs
	config Config

	// hashCache maps resource ID to DHash for fast similarity lookups
	hashCache   map[uint]uint64
	cacheMutex  sync.RWMutex
	cacheLoaded bool

	// hashQueue receives resource IDs for immediate async processing
	hashQueue chan uint

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// New creates a new HashWorker.
func New(db *gorm.DB, fs afero.Fs, altFS map[string]afero.Fs, config Config) *HashWorker {
	return &HashWorker{
		db:        db,
		fs:        fs,
		altFS:     altFS,
		config:    config,
		hashCache: make(map[uint]uint64),
		hashQueue: make(chan uint, 1000), // Buffer for on-upload async processing
		stopCh:    make(chan struct{}),
	}
}

// Start begins the background hash processing.
func (w *HashWorker) Start() {
	if w.config.Disabled {
		log.Println("Hash worker disabled")
		return
	}

	log.Printf("Starting hash worker: %d workers, batch size %d, poll interval %v, threshold %d",
		w.config.WorkerCount, w.config.BatchSize, w.config.PollInterval, w.config.SimilarityThreshold)

	w.wg.Add(1)
	go w.runBatchProcessor()

	// Start queue processors for on-upload async hashing
	for i := 0; i < w.config.WorkerCount; i++ {
		w.wg.Add(1)
		go w.runQueueProcessor()
	}
}

// Stop gracefully shuts down the hash worker.
func (w *HashWorker) Stop() {
	close(w.stopCh)
	w.wg.Wait()
	log.Println("Hash worker stopped")
}

// Queue adds a resource ID to the async processing queue.
// Returns true if queued, false if queue is full.
func (w *HashWorker) Queue(resourceID uint) bool {
	select {
	case w.hashQueue <- resourceID:
		return true
	default:
		return false
	}
}

func (w *HashWorker) runBatchProcessor() {
	defer w.wg.Done()

	// Initial delay to let the app start up
	select {
	case <-time.After(10 * time.Second):
	case <-w.stopCh:
		return
	}

	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()

	for {
		w.processBatch()

		select {
		case <-ticker.C:
		case <-w.stopCh:
			return
		}
	}
}

func (w *HashWorker) runQueueProcessor() {
	defer w.wg.Done()

	for {
		select {
		case resourceID := <-w.hashQueue:
			w.processResource(resourceID)
		case <-w.stopCh:
			return
		}
	}
}

func (w *HashWorker) processBatch() {
	// Priority 1: Migrate existing string hashes to uint64
	w.migrateStringHashes()

	// Priority 2: Hash new resources
	w.hashNewResources()
}

func (w *HashWorker) migrateStringHashes() {
	var toMigrate []models.ImageHash
	if err := w.db.
		Where("d_hash_int IS NULL AND d_hash IS NOT NULL AND d_hash != ''").
		Limit(w.config.BatchSize).
		Find(&toMigrate).Error; err != nil {
		log.Printf("Hash worker: error finding hashes to migrate: %v", err)
		return
	}

	if len(toMigrate) == 0 {
		return
	}

	log.Printf("Hash worker: migrating %d string hashes to uint64", len(toMigrate))

	for _, h := range toMigrate {
		aHash := h.GetAHash()
		dHash := h.GetDHash()

		if err := w.db.Model(&h).Updates(map[string]interface{}{
			"a_hash_int": aHash,
			"d_hash_int": dHash,
		}).Error; err != nil {
			log.Printf("Hash worker: error migrating hash %d: %v", h.ID, err)
		}
	}
}

func (w *HashWorker) hashNewResources() {
	// Find resources that need hashing
	var resources []models.Resource
	subQuery := w.db.Table("image_hashes").Select("resource_id")

	if err := w.db.
		Where("content_type IN ?", []string{"image/jpeg", "image/png", "image/gif", "image/webp"}).
		Where("id NOT IN (?)", subQuery).
		Limit(w.config.BatchSize).
		Find(&resources).Error; err != nil {
		log.Printf("Hash worker: error finding resources to hash: %v", err)
		return
	}

	if len(resources) == 0 {
		return
	}

	log.Printf("Hash worker: processing %d new resources", len(resources))

	// Ensure cache is loaded
	w.ensureCacheLoaded()

	// Process with concurrency limit
	sem := make(chan struct{}, w.config.WorkerCount)
	var wg sync.WaitGroup

	for _, resource := range resources {
		sem <- struct{}{}
		wg.Add(1)

		go func(r models.Resource) {
			defer wg.Done()
			defer func() { <-sem }()

			w.hashAndStoreSimilarities(r)
		}(resource)
	}

	wg.Wait()
}

func (w *HashWorker) processResource(resourceID uint) {
	var resource models.Resource
	if err := w.db.First(&resource, resourceID).Error; err != nil {
		log.Printf("Hash worker: error loading resource %d: %v", resourceID, err)
		return
	}

	if !IsHashable(resource.ContentType) {
		return
	}

	// Check if already hashed
	var count int64
	w.db.Model(&models.ImageHash{}).Where("resource_id = ?", resourceID).Count(&count)
	if count > 0 {
		return
	}

	w.ensureCacheLoaded()
	w.hashAndStoreSimilarities(resource)
}

func (w *HashWorker) ensureCacheLoaded() {
	w.cacheMutex.Lock()
	defer w.cacheMutex.Unlock()

	if w.cacheLoaded {
		return
	}

	var hashes []models.ImageHash
	if err := w.db.Select("resource_id, d_hash, d_hash_int").Find(&hashes).Error; err != nil {
		log.Printf("Hash worker: error loading hash cache: %v", err)
		return
	}

	for _, h := range hashes {
		if h.ResourceId != nil {
			w.hashCache[*h.ResourceId] = h.GetDHash()
		}
	}

	w.cacheLoaded = true
	log.Printf("Hash worker: loaded %d hashes into cache", len(w.hashCache))
}

func (w *HashWorker) hashAndStoreSimilarities(resource models.Resource) {
	// Get filesystem for this resource
	fs := w.fs
	if resource.StorageLocation != nil && *resource.StorageLocation != "" {
		if altFs, ok := w.altFS[*resource.StorageLocation]; ok {
			fs = altFs
		}
	}

	// Open and decode image
	file, err := fs.Open(resource.GetCleanLocation())
	if err != nil {
		log.Printf("Hash worker: error opening resource %d: %v", resource.ID, err)
		return
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		log.Printf("Hash worker: error decoding resource %d: %v", resource.ID, err)
		return
	}

	// Calculate hashes
	aHash := imgsim.AverageHash(img)
	dHash := imgsim.DifferenceHash(img)

	aHashInt := uint64(aHash)
	dHashInt := uint64(dHash)

	// Save hash
	imgHash := models.ImageHash{
		AHash:      aHash.String(),
		DHash:      dHash.String(),
		AHashInt:   &aHashInt,
		DHashInt:   &dHashInt,
		ResourceId: &resource.ID,
	}

	if err := w.db.Create(&imgHash).Error; err != nil {
		log.Printf("Hash worker: error saving hash for resource %d: %v", resource.ID, err)
		return
	}

	// Find and store similarities
	w.findAndStoreSimilarities(resource.ID, dHashInt)

	// Update cache
	w.cacheMutex.Lock()
	w.hashCache[resource.ID] = dHashInt
	w.cacheMutex.Unlock()
}

func (w *HashWorker) findAndStoreSimilarities(resourceID uint, dHash uint64) {
	var similarities []models.ResourceSimilarity

	w.cacheMutex.RLock()
	for otherID, otherHash := range w.hashCache {
		if otherID == resourceID {
			continue
		}

		distance := HammingDistance(dHash, otherHash)
		if distance <= w.config.SimilarityThreshold {
			// Ensure ResourceID1 < ResourceID2
			id1, id2 := resourceID, otherID
			if id1 > id2 {
				id1, id2 = id2, id1
			}

			similarities = append(similarities, models.ResourceSimilarity{
				ResourceID1:     id1,
				ResourceID2:     id2,
				HammingDistance: uint8(distance),
			})
		}
	}
	w.cacheMutex.RUnlock()

	if len(similarities) == 0 {
		return
	}

	// Batch insert with conflict handling
	if err := w.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&similarities).Error; err != nil {
		log.Printf("Hash worker: error saving similarities for resource %d: %v", resourceID, err)
	}
}
```

**Step 2: Run build to verify syntax**

Run: `go build --tags 'json1 fts5' ./...`
Expected: No errors

**Step 3: Commit**

```bash
git add hash_worker/worker.go
git commit -m "feat: add HashWorker core implementation"
```

---

## Task 8: Add Configuration Flags to main.go

**Files:**
- Modify: `main.go`

**Step 1: Add imports**

Add to the import block:

```go
	"mahresources/hash_worker"
```

**Step 2: Add flag definitions after existing flags (around line 83)**

Find this section:

```go
	cleanupLogsDays := flag.Int("cleanup-logs-days", parseIntEnv("CLEANUP_LOGS_DAYS", 0), "Delete log entries older than N days on startup (0=disabled) (env: CLEANUP_LOGS_DAYS)")
```

Add after it:

```go
	// Hash worker options
	hashWorkerCount := flag.Int("hash-worker-count", parseIntEnv("HASH_WORKER_COUNT", 4), "Number of concurrent hash calculation workers (env: HASH_WORKER_COUNT)")
	hashBatchSize := flag.Int("hash-batch-size", parseIntEnv("HASH_BATCH_SIZE", 500), "Resources to process per batch cycle (env: HASH_BATCH_SIZE)")
	hashPollInterval := flag.Duration("hash-poll-interval", parseDurationEnv("HASH_POLL_INTERVAL", time.Minute), "Time between batch processing cycles (env: HASH_POLL_INTERVAL)")
	hashSimilarityThreshold := flag.Int("hash-similarity-threshold", parseIntEnv("HASH_SIMILARITY_THRESHOLD", 10), "Maximum Hamming distance for similarity (env: HASH_SIMILARITY_THRESHOLD)")
	hashWorkerDisabled := flag.Bool("hash-worker-disabled", os.Getenv("HASH_WORKER_DISABLED") == "1", "Disable hash worker (env: HASH_WORKER_DISABLED=1)")
```

**Step 3: Start hash worker before server (around line 238)**

Find this section at the end of main():

```go
	log.Fatal(server.CreateServer(context, mainFs, context.Config.AltFileSystems).ListenAndServe())
```

Add before it:

```go
	// Start hash worker for background perceptual hash calculation
	hashWorkerConfig := hash_worker.Config{
		WorkerCount:         *hashWorkerCount,
		BatchSize:           *hashBatchSize,
		PollInterval:        *hashPollInterval,
		SimilarityThreshold: *hashSimilarityThreshold,
		Disabled:            *hashWorkerDisabled,
	}
	hw := hash_worker.New(db, mainFs, context.Config.AltFileSystems, hashWorkerConfig)
	hw.Start()
	defer hw.Stop()

```

**Step 4: Create altFileSystems map with afero.Fs values for hash worker**

The hash worker needs `map[string]afero.Fs` but `context.Config.AltFileSystems` is `map[string]string`. We need to pass the correct type.

Find where `cfg` is created and add a new variable after the server creation for the hash worker. Actually, looking at the code, we need to convert. Let's modify the hash worker initialization:

Replace the hash worker section with:

```go
	// Start hash worker for background perceptual hash calculation
	hashWorkerConfig := hash_worker.Config{
		WorkerCount:         *hashWorkerCount,
		BatchSize:           *hashBatchSize,
		PollInterval:        *hashPollInterval,
		SimilarityThreshold: *hashSimilarityThreshold,
		Disabled:            *hashWorkerDisabled,
	}

	// Build alt filesystems map for hash worker
	altFsMap := make(map[string]afero.Fs)
	for name, path := range context.Config.AltFileSystems {
		altFsMap[name] = storage.CreateStorage(path)
	}

	hw := hash_worker.New(db, mainFs, altFsMap, hashWorkerConfig)
	hw.Start()
	defer hw.Stop()

```

Also add to imports:

```go
	"mahresources/storage"
```

**Step 5: Run build to verify**

Run: `go build --tags 'json1 fts5'`
Expected: No errors

**Step 6: Commit**

```bash
git add main.go
git commit -m "feat: add hash worker configuration flags and startup"
```

---

## Task 9: Update GetSimilarResources to Use Pre-computed Similarities

**Files:**
- Modify: `application_context/resource_crud_context.go:17-36`

**Step 1: Replace GetSimilarResources function**

Replace the existing `GetSimilarResources` function:

```go
func (ctx *MahresourcesContext) GetSimilarResources(id uint) (*[]*models.Resource, error) {
	var resources []*models.Resource

	// Find all resource IDs similar to this one from pre-computed similarities
	var similarIDs []uint

	// Query both directions since we store with ResourceID1 < ResourceID2
	rows, err := ctx.db.Raw(`
		SELECT CASE WHEN resource_id_1 = ? THEN resource_id_2 ELSE resource_id_1 END as similar_id
		FROM resource_similarities
		WHERE resource_id_1 = ? OR resource_id_2 = ?
		ORDER BY hamming_distance ASC
	`, id, id, id).Rows()

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var similarID uint
		if err := rows.Scan(&similarID); err != nil {
			return nil, err
		}
		similarIDs = append(similarIDs, similarID)
	}

	if len(similarIDs) == 0 {
		// Fall back to exact hash match for resources not yet processed by worker
		hashQuery := ctx.db.Table("image_hashes rootHash").
			Select("d_hash").
			Where("rootHash.resource_id = ?", id).
			Limit(1)

		sameHashIdsQuery := ctx.db.Table("image_hashes").
			Select("resource_id").
			Group("resource_id").
			Where("d_hash = (?)", hashQuery)

		return &resources, ctx.db.
			Preload("Tags").
			Joins("Owner").
			Where("resources.id IN (?)", sameHashIdsQuery).
			Where("resources.id <> ?", id).
			Find(&resources).Error
	}

	return &resources, ctx.db.
		Preload("Tags").
		Joins("Owner").
		Where("resources.id IN ?", similarIDs).
		Find(&resources).Error
}
```

**Step 2: Run build to verify**

Run: `go build --tags 'json1 fts5' ./...`
Expected: No errors

**Step 3: Run tests**

Run: `go test ./... -v`
Expected: All tests pass

**Step 4: Commit**

```bash
git add application_context/resource_crud_context.go
git commit -m "feat: update GetSimilarResources to use pre-computed similarities"
```

---

## Task 10: Integrate Hash Queue with Resource Upload

**Files:**
- Modify: `application_context/context.go`
- Modify: `application_context/resource_upload_context.go`

**Step 1: Add hash queue channel to MahresourcesContext**

In `application_context/context.go`, add to the `MahresourcesContext` struct:

```go
	// hashQueue is a channel to queue resources for async hash processing
	hashQueue chan<- uint
```

**Step 2: Add method to set hash queue**

Add this method to `context.go`:

```go
// SetHashQueue sets the channel for queueing resources for hash processing.
func (ctx *MahresourcesContext) SetHashQueue(queue chan<- uint) {
	ctx.hashQueue = queue
}

// QueueForHashing queues a resource ID for async hash processing.
// Returns true if queued, false if queue is nil or full.
func (ctx *MahresourcesContext) QueueForHashing(resourceID uint) bool {
	if ctx.hashQueue == nil {
		return false
	}
	select {
	case ctx.hashQueue <- resourceID:
		return true
	default:
		return false
	}
}
```

**Step 3: Queue new resources after creation**

In `application_context/resource_upload_context.go`, find the end of `AddResource` function (around line 555-558):

```go
	ctx.Logger().Info(models.LogActionCreate, "resource", &res.ID, res.Name, "Created resource", nil)

	ctx.InvalidateSearchCacheByType(EntityTypeResource)
	return res, nil
```

Add hash queue call:

```go
	ctx.Logger().Info(models.LogActionCreate, "resource", &res.ID, res.Name, "Created resource", nil)

	ctx.InvalidateSearchCacheByType(EntityTypeResource)

	// Queue for async hash processing if it's a hashable image type
	if IsHashableContentType(fileMime.String()) {
		ctx.QueueForHashing(res.ID)
	}

	return res, nil
```

**Step 4: Add IsHashableContentType helper**

Add at the top of `resource_upload_context.go` (after imports):

```go
// hashableContentTypes is the set of content types that can be perceptually hashed.
var hashableContentTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

// IsHashableContentType returns true if the content type can be perceptually hashed.
func IsHashableContentType(contentType string) bool {
	return hashableContentTypes[contentType]
}
```

**Step 5: Update main.go to wire up hash queue**

In `main.go`, after creating the hash worker, add:

```go
	context.SetHashQueue(hw.GetQueue())
```

And add this method to the HashWorker in `hash_worker/worker.go`:

```go
// GetQueue returns the hash queue channel for external use.
func (w *HashWorker) GetQueue() chan<- uint {
	return w.hashQueue
}
```

**Step 6: Run build to verify**

Run: `go build --tags 'json1 fts5' ./...`
Expected: No errors

**Step 7: Commit**

```bash
git add application_context/context.go application_context/resource_upload_context.go hash_worker/worker.go main.go
git commit -m "feat: integrate hash queue with resource upload for async processing"
```

---

## Task 11: Handle Resource Deletion (Cleanup Similarities)

**Files:**
- Observe cascade delete behavior

**Step 1: Verify cascade delete works**

The `ResourceSimilarity` model has cascade delete constraints:
```go
Resource1 *Resource `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignKey:ResourceID1"`
Resource2 *Resource `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignKey:ResourceID2"`
```

This means when a resource is deleted, all its similarity entries are automatically deleted. No additional code needed.

**Step 2: Commit documentation update if needed**

No code changes needed - cascade delete handles cleanup automatically.

---

## Task 12: Delete the Old CLI Tool

**Files:**
- Delete: `cmd/perceptualHash/main.go`
- Delete: `cmd/perceptualHash/` directory

**Step 1: Remove the directory**

```bash
rm -rf cmd/perceptualHash
```

**Step 2: Verify build still works**

Run: `go build --tags 'json1 fts5' ./...`
Expected: No errors

**Step 3: Commit**

```bash
git add -A
git commit -m "chore: remove standalone perceptualHash CLI tool (replaced by hash worker)"
```

---

## Task 13: Update CLAUDE.md with New Configuration

**Files:**
- Modify: `CLAUDE.md`

**Step 1: Add hash worker flags to the Configuration table**

Find the configuration table and add:

```markdown
| `-hash-worker-count` | `HASH_WORKER_COUNT` | Concurrent hash calculation workers (default: 4) |
| `-hash-batch-size` | `HASH_BATCH_SIZE` | Resources to process per batch (default: 500) |
| `-hash-poll-interval` | `HASH_POLL_INTERVAL` | Time between batch cycles (default: 1m) |
| `-hash-similarity-threshold` | `HASH_SIMILARITY_THRESHOLD` | Max Hamming distance for similarity (default: 10) |
| `-hash-worker-disabled` | `HASH_WORKER_DISABLED=1` | Disable background hash worker |
```

**Step 2: Remove references to cmd/perceptualHash if any**

Search for and remove any references to the old CLI tool.

**Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md with hash worker configuration"
```

---

## Task 14: Add Integration Test

**Files:**
- Create: `hash_worker/worker_test.go`

**Step 1: Write integration test**

```go
package hash_worker

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"mahresources/models"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	if err := db.AutoMigrate(
		&models.Resource{},
		&models.ImageHash{},
		&models.ResourceSimilarity{},
	); err != nil {
		t.Fatalf("Failed to migrate: %v", err)
	}

	return db
}

func TestHashWorker_MigrateStringHashes(t *testing.T) {
	db := setupTestDB(t)

	// Create a hash with old string format
	resourceID := uint(1)
	hash := models.ImageHash{
		AHash:      "ff00ff00ff00ff00",
		DHash:      "00ff00ff00ff00ff",
		ResourceId: &resourceID,
	}
	if err := db.Create(&hash).Error; err != nil {
		t.Fatalf("Failed to create hash: %v", err)
	}

	// Create worker and run migration
	w := New(db, afero.NewMemMapFs(), nil, Config{
		WorkerCount:         1,
		BatchSize:           100,
		PollInterval:        time.Hour,
		SimilarityThreshold: 10,
	})

	w.migrateStringHashes()

	// Verify migration
	var updated models.ImageHash
	if err := db.First(&updated, hash.ID).Error; err != nil {
		t.Fatalf("Failed to load hash: %v", err)
	}

	if updated.AHashInt == nil || updated.DHashInt == nil {
		t.Error("Hash not migrated to uint64")
	}

	expectedAHash := uint64(0xff00ff00ff00ff00)
	expectedDHash := uint64(0x00ff00ff00ff00ff)

	if *updated.AHashInt != expectedAHash {
		t.Errorf("AHashInt = %x, want %x", *updated.AHashInt, expectedAHash)
	}
	if *updated.DHashInt != expectedDHash {
		t.Errorf("DHashInt = %x, want %x", *updated.DHashInt, expectedDHash)
	}
}

func TestHashWorker_FindSimilarities(t *testing.T) {
	db := setupTestDB(t)
	w := New(db, afero.NewMemMapFs(), nil, Config{
		WorkerCount:         1,
		BatchSize:           100,
		PollInterval:        time.Hour,
		SimilarityThreshold: 10,
	})

	// Seed cache with some hashes
	w.hashCache[1] = 0xFF00FF00FF00FF00 // Base hash
	w.hashCache[2] = 0xFF00FF00FF00FF01 // 1 bit different (similar)
	w.hashCache[3] = 0x00FF00FF00FF00FF // 64 bits different (not similar)
	w.cacheLoaded = true

	// Find similarities for a new hash that's similar to #1 and #2
	newHash := uint64(0xFF00FF00FF00FF00) // Identical to #1
	w.findAndStoreSimilarities(100, newHash)

	// Verify similarities were stored
	var similarities []models.ResourceSimilarity
	if err := db.Find(&similarities).Error; err != nil {
		t.Fatalf("Failed to query similarities: %v", err)
	}

	if len(similarities) != 2 {
		t.Errorf("Expected 2 similarities, got %d", len(similarities))
	}

	// Verify ordering (ResourceID1 < ResourceID2)
	for _, sim := range similarities {
		if sim.ResourceID1 >= sim.ResourceID2 {
			t.Errorf("Similarity has incorrect ordering: %d >= %d", sim.ResourceID1, sim.ResourceID2)
		}
	}
}
```

**Step 2: Run tests**

Run: `go test ./hash_worker/... -v`
Expected: All tests pass

**Step 3: Commit**

```bash
git add hash_worker/worker_test.go
git commit -m "test: add integration tests for hash worker"
```

---

## Task 15: Final Build and Test

**Step 1: Run full build**

Run: `npm run build`
Expected: Success

**Step 2: Run all Go tests**

Run: `go test ./... -v`
Expected: All tests pass

**Step 3: Run E2E tests**

Run: `cd e2e && npm run test:with-server`
Expected: All tests pass

**Step 4: Final commit if any cleanup needed**

```bash
git status
# If clean, no action needed
```

---

## Summary

This plan implements:

1. **Data model changes**: New `ResourceSimilarity` table, updated `ImageHash` with uint64 columns
2. **Background worker**: `HashWorker` with configurable concurrency, batch processing, and on-upload async queueing
3. **Similarity calculation**: Hamming distance with configurable threshold (default 10)
4. **Non-blocking migration**: Gradual migration of existing string hashes to uint64
5. **Query updates**: `GetSimilarResources` uses pre-computed similarities with fallback
6. **Cleanup**: Removed old CLI tool, cascade delete handles resource removal
7. **Configuration**: New flags for worker tuning
