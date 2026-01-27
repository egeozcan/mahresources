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

		if err := w.db.Model(&h).Updates(map[string]any{
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

	// Build content type list from HashableContentTypes
	contentTypes := make([]string, 0, len(HashableContentTypes))
	for ct := range HashableContentTypes {
		contentTypes = append(contentTypes, ct)
	}

	if err := w.db.
		Where("content_type IN ?", contentTypes).
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
