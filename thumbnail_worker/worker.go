package thumbnail_worker

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"mahresources/models"
)

// ThumbnailGenerator is the interface needed to generate thumbnails.
type ThumbnailGenerator interface {
	LoadOrCreateThumbnailForResource(resourceId, width, height uint, ctx context.Context) (*models.Preview, error)
}

// ThumbnailWorker processes video resources to pre-generate null thumbnails in the background.
type ThumbnailWorker struct {
	db     *gorm.DB
	gen    ThumbnailGenerator
	config Config

	// thumbQueue receives resource IDs for immediate async processing
	thumbQueue chan uint

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// New creates a new ThumbnailWorker.
func New(db *gorm.DB, gen ThumbnailGenerator, config Config) *ThumbnailWorker {
	return &ThumbnailWorker{
		db:         db,
		gen:        gen,
		config:     config,
		thumbQueue: make(chan uint, 1000),
		stopCh:     make(chan struct{}),
	}
}

// Start begins the background thumbnail processing.
func (w *ThumbnailWorker) Start() {
	if w.config.Disabled {
		log.Println("Thumbnail worker disabled")
		return
	}

	log.Printf("Starting thumbnail worker: %d workers, backfill=%v",
		w.config.WorkerCount, w.config.Backfill)

	// Start queue processors for on-upload async thumbnailing
	for i := 0; i < w.config.WorkerCount; i++ {
		w.wg.Add(1)
		go w.runQueueProcessor()
	}

	// Start backfill processor if enabled
	if w.config.Backfill {
		w.wg.Add(1)
		go w.runBackfillProcessor()
	}
}

// Stop gracefully shuts down the thumbnail worker.
func (w *ThumbnailWorker) Stop() {
	close(w.stopCh)
	w.wg.Wait()
	log.Println("Thumbnail worker stopped")
}

// GetQueue returns the thumbnail queue channel for external use.
func (w *ThumbnailWorker) GetQueue() chan<- uint {
	return w.thumbQueue
}

func (w *ThumbnailWorker) runQueueProcessor() {
	defer w.wg.Done()

	for {
		select {
		case resourceID := <-w.thumbQueue:
			w.processResource(resourceID)
		case <-w.stopCh:
			// Drain remaining items from queue before exiting
			for {
				select {
				case resourceID := <-w.thumbQueue:
					w.processResource(resourceID)
				default:
					return
				}
			}
		}
	}
}

func (w *ThumbnailWorker) runBackfillProcessor() {
	defer w.wg.Done()

	// Initial delay to let the app start up
	select {
	case <-time.After(30 * time.Second):
	case <-w.stopCh:
		return
	}

	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()

	for {
		w.processBackfillBatch()

		select {
		case <-ticker.C:
		case <-w.stopCh:
			return
		}
	}
}

func (w *ThumbnailWorker) processResource(resourceID uint) {
	// Check if this resource already has a null thumbnail
	var count int64
	w.db.Model(&models.Preview{}).
		Where("resource_id = ? AND width = 0 AND height = 0", resourceID).
		Count(&count)
	if count > 0 {
		return
	}

	// Verify the resource is a video
	var resource models.Resource
	if err := w.db.Select("id, content_type").First(&resource, resourceID).Error; err != nil {
		log.Printf("Thumbnail worker: error loading resource %d: %v", resourceID, err)
		return
	}

	if !strings.HasPrefix(resource.ContentType, "video/") {
		return
	}

	// Generate thumbnail at a default size (the null thumbnail will be created as a side effect)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	_, err := w.gen.LoadOrCreateThumbnailForResource(resourceID, 200, 0, ctx)
	if err != nil {
		log.Printf("Thumbnail worker: error generating thumbnail for resource %d: %v", resourceID, err)
	}
}

func (w *ThumbnailWorker) processBackfillBatch() {
	// Find video resources without null thumbnails, prioritizing recent uploads
	var resources []models.Resource

	if err := w.db.
		Select("resources.id").
		Joins("LEFT JOIN previews ON previews.resource_id = resources.id AND previews.width = 0 AND previews.height = 0").
		Where("previews.id IS NULL").
		Where("resources.content_type LIKE 'video/%'").
		Order("resources.id DESC").
		Limit(w.config.BatchSize).
		Find(&resources).Error; err != nil {
		log.Printf("Thumbnail worker: error finding videos to backfill: %v", err)
		return
	}

	if len(resources) == 0 {
		return
	}

	log.Printf("Thumbnail worker: backfilling %d videos", len(resources))

	for _, resource := range resources {
		select {
		case <-w.stopCh:
			return
		default:
		}
		w.processResource(resource.ID)
	}
}
