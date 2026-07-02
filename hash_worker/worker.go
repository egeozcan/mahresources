package hash_worker

import (
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	// Register image formats
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"

	"github.com/Nr90/imgsim"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/spf13/afero"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mahresources/models"
)

// AppLogger is an interface for application-level logging that persists to the database.
type AppLogger interface {
	Info(action, entityType string, entityID *uint, entityName, message string, details map[string]interface{})
	Warning(action, entityType string, entityID *uint, entityName, message string, details map[string]interface{})
	Error(action, entityType string, entityID *uint, entityName, message string, details map[string]interface{})
}

// HashWorker processes resources to calculate perceptual hashes and find similarities.
// hashEntry holds both perceptual hashes for a resource in the cache.
type hashEntry struct {
	DHash uint64
	AHash uint64
}

type HashWorker struct {
	db        *gorm.DB
	fs        afero.Fs
	altFS     map[string]afero.Fs
	config    Config
	appLogger AppLogger

	// hashCache is a bounded LRU cache mapping resource ID to both DHash and AHash (BH-018)
	hashCache *lru.Cache[uint, hashEntry]

	// hashQueue receives resource IDs for immediate async processing
	hashQueue chan uint

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// New creates a new HashWorker.
// appLogger is optional - if nil, progress will only be logged to stdout.
func New(db *gorm.DB, fs afero.Fs, altFS map[string]afero.Fs, config Config, appLogger AppLogger) *HashWorker {
	if config.CacheSize <= 0 {
		config.CacheSize = 100000
	}
	cache, _ := lru.New[uint, hashEntry](config.CacheSize)

	return &HashWorker{
		db:        db,
		fs:        fs,
		altFS:     altFS,
		config:    config,
		appLogger: appLogger,
		hashCache: cache,
		hashQueue: make(chan uint, 1000), // Buffer for on-upload async processing
		stopCh:    make(chan struct{}),
	}
}

// logProgress logs progress to both stdout and the app logger (if available).
func (w *HashWorker) logProgress(message string, details map[string]interface{}) {
	log.Print(message)
	if w.appLogger != nil {
		w.appLogger.Info("progress", "hash_worker", nil, "", message, details)
	}
}

// logError logs an error to both stdout and the app logger (if available).
func (w *HashWorker) logError(message string, details map[string]interface{}) {
	log.Print(message)
	if w.appLogger != nil {
		w.appLogger.Error("error", "hash_worker", nil, "", message, details)
	}
}

// Start begins the background hash processing.
func (w *HashWorker) Start() {
	if w.config.Disabled {
		log.Println("Hash worker disabled")
		return
	}

	log.Printf("Starting hash worker: %d workers, batch size %d, poll interval %v, threshold %d",
		w.config.WorkerCount, w.config.BatchSize, w.config.PollInterval, w.config.SimilarityThresholdFn())

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

// GetQueue returns the hash queue channel for external use.
func (w *HashWorker) GetQueue() chan<- uint {
	return w.hashQueue
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
		w.safeProcessBatch()

		select {
		case <-ticker.C:
		case <-w.stopCh:
			return
		}
	}
}

// safeProcessBatch wraps processBatch with panic recovery to prevent the
// batch processor goroutine from dying permanently on unexpected panics.
func (w *HashWorker) safeProcessBatch() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[hash_worker] batch processor recovered from panic: %v", r)
		}
	}()
	w.processBatch()
}

func (w *HashWorker) runQueueProcessor() {
	defer w.wg.Done()

	for {
		select {
		case resourceID := <-w.hashQueue:
			w.safeProcessResource(resourceID)
		case <-w.stopCh:
			// Drain remaining items from queue before exiting
			for {
				select {
				case resourceID := <-w.hashQueue:
					w.safeProcessResource(resourceID)
				default:
					return
				}
			}
		}
	}
}

// safeProcessResource wraps processResource with panic recovery to prevent
// queue processor goroutines from dying permanently on unexpected panics.
func (w *HashWorker) safeProcessResource(resourceID uint) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[hash_worker] queue processor recovered from panic for resource %d: %v", resourceID, r)
		}
	}()
	w.processResource(resourceID)
}

func (w *HashWorker) processBatch() {
	// Priority 1: Migrate existing string hashes to uint64
	w.migrateStringHashes()

	// Priority 2: Hash new resources — fresh uploads take precedence over the
	// backfill so their similarity results appear promptly even while a
	// multi-day v2 backfill is in flight.
	w.hashNewResources()

	// Priority 3: Backfill existing rows to v2 (incremental, resumable, pausable)
	w.backfillV2Hashes()
}

func (w *HashWorker) migrateStringHashes() {
	// Count total remaining for progress logging
	var totalRemaining int64
	w.db.Model(&models.ImageHash{}).
		Where("d_hash_int IS NULL AND d_hash IS NOT NULL AND d_hash != ''").
		Count(&totalRemaining)

	if totalRemaining == 0 {
		return
	}

	var toMigrate []models.ImageHash
	if err := w.db.
		Where("d_hash_int IS NULL AND d_hash IS NOT NULL AND d_hash != ''").
		Limit(w.config.BatchSize).
		Find(&toMigrate).Error; err != nil {
		w.logError(fmt.Sprintf("Hash worker: error finding hashes to migrate: %v", err), nil)
		return
	}

	if len(toMigrate) == 0 {
		return
	}

	w.logProgress(fmt.Sprintf("Hash worker: migrating %d hashes (remaining: %d)", len(toMigrate), totalRemaining),
		map[string]interface{}{"batch_size": len(toMigrate), "remaining": totalRemaining})

	for _, h := range toMigrate {
		aHash := h.GetAHash()
		dHash := h.GetDHash()

		// Convert to int64 for PostgreSQL storage (bit-reinterpretation)
		aHashSigned := int64(aHash)
		dHashSigned := int64(dHash)

		if err := w.db.Model(&h).Updates(map[string]any{
			"a_hash_int": aHashSigned,
			"d_hash_int": dHashSigned,
		}).Error; err != nil {
			log.Printf("Hash worker: error migrating hash %d: %v", h.ID, err)
		}
	}
}

func (w *HashWorker) hashNewResources() {
	// Find resources that need hashing using LEFT JOIN (much faster than NOT IN with large datasets)
	var resources []models.Resource

	if err := w.db.
		Joins("LEFT JOIN image_hashes ON image_hashes.resource_id = resources.id").
		Where("image_hashes.id IS NULL").
		Where("resources.content_type IN ?", hashableContentTypesList).
		Limit(w.config.BatchSize).
		Find(&resources).Error; err != nil {
		w.logError(fmt.Sprintf("Hash worker: error finding resources to hash: %v", err), nil)
		return
	}

	if len(resources) == 0 {
		return
	}

	// Count total remaining for progress logging (only if we have work to do)
	var totalRemaining int64
	w.db.Model(&models.Resource{}).
		Joins("LEFT JOIN image_hashes ON image_hashes.resource_id = resources.id").
		Where("image_hashes.id IS NULL").
		Where("resources.content_type IN ?", hashableContentTypesList).
		Count(&totalRemaining)

	w.logProgress(fmt.Sprintf("Hash worker: hashing %d resources (remaining: %d)", len(resources), totalRemaining),
		map[string]interface{}{"batch_size": len(resources), "remaining": totalRemaining})

	// Warm cache if needed
	if w.hashCache.Len() == 0 {
		w.warmCache()
	}

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

	if w.hashCache.Len() == 0 {
		w.warmCache()
	}
	w.hashAndStoreSimilarities(resource)
}

// warmCache loads existing perceptual hashes into the LRU cache on first use.
// For large deployments (millions of hashed images) this may take several seconds
// at startup due to the database scan. The batched approach avoids loading all rows
// into memory at once, but the total I/O is proportional to the number of hashed images.
func (w *HashWorker) warmCache() {
	// Load hashes in pages to seed the LRU cache without loading everything at once
	batchSize := w.config.CacheSize
	if batchSize > 50000 {
		batchSize = 50000
	}
	offset := 0

	for {
		var hashes []models.ImageHash
		if err := w.db.Select("resource_id, d_hash, d_hash_int, a_hash, a_hash_int").
			Offset(offset).Limit(batchSize).
			Find(&hashes).Error; err != nil {
			log.Printf("Hash worker: error warming cache: %v", err)
			return
		}

		if len(hashes) == 0 {
			break
		}

		for _, h := range hashes {
			if h.ResourceId != nil {
				w.hashCache.Add(*h.ResourceId, hashEntry{DHash: h.GetDHash(), AHash: h.GetAHash()})
			}
		}

		if w.hashCache.Len() >= w.config.CacheSize {
			break // Cache is full
		}

		offset += batchSize
	}

	log.Printf("Hash worker: cache warmed with %d entries (max %d)", w.hashCache.Len(), w.config.CacheSize)
}

// readResourceBytes reads a resource's full file contents from its (possibly
// alternate) filesystem so callers can both decode the image and read EXIF.
func (w *HashWorker) readResourceBytes(resource models.Resource) ([]byte, error) {
	fs := w.fs
	if resource.StorageLocation != nil && *resource.StorageLocation != "" {
		if altFs, ok := w.altFS[*resource.StorageLocation]; ok {
			fs = altFs
		}
	}
	file, err := fs.Open(resource.GetCleanLocation())
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}

func (w *HashWorker) hashAndStoreSimilarities(resource models.Resource) {
	data, err := w.readResourceBytes(resource)
	if err != nil {
		log.Printf("Hash worker: error reading resource %d: %v", resource.ID, err)
		w.markResourceFailed(resource.ID)
		return
	}

	v2, err := ComputeV2Hashes(data)
	if err != nil {
		log.Printf("Hash worker: error hashing resource %d: %v", resource.ID, err)
		w.markResourceFailed(resource.ID)
		return
	}

	// Build the imgsim string representations for the legacy columns from the
	// v2-normalized image so the legacy read/match path keeps working.
	legacyDHash := imgsim.Hash(v2.LegacyDHash)
	legacyAHash := imgsim.Hash(v2.LegacyAHash)

	dHashSigned := int64(v2.LegacyDHash)
	aHashSigned := int64(v2.LegacyAHash)
	pHashSigned := int64(v2.PHash)
	chunks := SplitChunks(v2.PHash)
	c0, c1, c2, c3 := int32(chunks[0]), int32(chunks[1]), int32(chunks[2]), int32(chunks[3])
	ver := HashVersionV2

	// Save hash (dual-write: legacy columns + v2 columns). Upsert on the unique
	// resource_id so a pre-existing row (e.g. a failed placeholder whose file was
	// since restored) is updated in place — the persisted row must match the
	// hashes cached and matched below.
	imgHash := models.ImageHash{
		AHash:       legacyAHash.String(),
		DHash:       legacyDHash.String(),
		AHashInt:    &aHashSigned,
		DHashInt:    &dHashSigned,
		HashVersion: &ver,
		PHashInt:    &pHashSigned,
		PChunk0:     &c0,
		PChunk1:     &c1,
		PChunk2:     &c2,
		PChunk3:     &c3,
		Status:      v2.Status,
		ResourceId:  &resource.ID,
	}

	if err := w.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "resource_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"a_hash", "d_hash", "a_hash_int", "d_hash_int",
			"hash_version", "p_hash_int", "p_chunk0", "p_chunk1", "p_chunk2", "p_chunk3", "status",
		}),
	}).Create(&imgHash).Error; err != nil {
		log.Printf("Hash worker: error saving hash for resource %d: %v", resource.ID, err)
		return
	}

	// Update cache BEFORE finding similarities to avoid race condition
	// where concurrent goroutines miss detecting similarities between
	// resources being processed simultaneously
	w.hashCache.Add(resource.ID, hashEntry{DHash: v2.LegacyDHash, AHash: v2.LegacyAHash})

	// Legacy matching path (LRU cache + dHash threshold) is unchanged, so new
	// v2 uploads still match old v1 images via the cache.
	w.findAndStoreSimilarities(resource.ID, v2.LegacyDHash, v2.LegacyAHash)

	// v2 chunk-index matching against other v2 rows. Skip flat/failed probes,
	// which are excluded from matching entirely. Dedup with the legacy path is
	// handled by the pair upsert in findSimilaritiesV2.
	if v2.Status == models.HashStatusOK {
		w.findSimilaritiesV2(resource.ID, v2.PHash, v2.LegacyDHash, v2.LegacyAHash)
	}
}

// AreSimilar returns true when two images should be recorded as perceptually similar.
//
// BH-018: The imgsim library produces DHash=0 and AHash=0 for any uniform (solid-color)
// image, regardless of actual color. This means every solid-color image appears identical
// to every other solid-color image (distance 0). To prevent these false positives, we skip
// similarity when either image has both DHash==0 and AHash==0 (indicating a solid-color
// image that cannot be meaningfully compared by perceptual hashing).
//
// Additionally, when aHashThr>0, the AHash Hamming distance must be within aHashThr.
// When aHashThr is 0, the secondary check is skipped (backward-compatible behavior).
func AreSimilar(dHashA, aHashA, dHashB, aHashB, dHashThr, aHashThr uint64) bool {
	// Skip solid-color images: both hashes are 0, meaning any color matches any other
	if dHashA == 0 && aHashA == 0 {
		return false
	}
	if dHashB == 0 && aHashB == 0 {
		return false
	}

	dDist := uint64(HammingDistance(dHashA, dHashB))
	if dDist > dHashThr {
		return false
	}
	if aHashThr == 0 {
		return true
	}
	aDist := uint64(HammingDistance(aHashA, aHashB))
	return aDist <= aHashThr
}

// findAndStoreSimilarities compares a newly hashed resource against all entries in
// the in-memory cache. This is O(N) per resource (O(N^2) overall when hashing N new
// resources in a batch). For typical deployments (< 1M images) this is fast enough
// since Hamming distance is a single XOR + popcount. For very large deployments, a
// BK-tree or VP-tree index on Hamming distance would reduce this to O(log N) per lookup.
func (w *HashWorker) findAndStoreSimilarities(resourceID uint, dHash, aHash uint64) {
	var similarities []models.ResourceSimilarity

	for _, otherID := range w.hashCache.Keys() {
		if otherID == resourceID {
			continue
		}
		otherEntry, ok := w.hashCache.Peek(otherID)
		if !ok {
			continue
		}

		if !AreSimilar(dHash, aHash, otherEntry.DHash, otherEntry.AHash,
			uint64(w.config.SimilarityThresholdFn()), w.config.AHashThresholdFn()) {
			continue
		}

		distance := HammingDistance(dHash, otherEntry.DHash)
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

	if len(similarities) == 0 {
		return
	}

	// Batch insert with conflict handling
	if err := w.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&similarities).Error; err != nil {
		log.Printf("Hash worker: error saving similarities for resource %d: %v", resourceID, err)
	}
}

// markResourceFailed creates a placeholder hash record to mark a resource as processed
// but unhashable (corrupt file, unsupported format, etc). This prevents the worker
// from retrying the same failed resources every cycle.
func (w *HashWorker) markResourceFailed(resourceID uint) {
	ver := HashVersionV2
	imgHash := models.ImageHash{
		ResourceId:  &resourceID,
		HashVersion: &ver,
		Status:      models.HashStatusFailed,
		// Leave hash fields empty/nil to indicate failure
	}

	if err := w.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&imgHash).Error; err != nil {
		log.Printf("Hash worker: error marking resource %d as failed: %v", resourceID, err)
	}
}
