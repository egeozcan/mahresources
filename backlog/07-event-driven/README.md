# Strategy 7: Event-Driven Side Effects

**Complexity:** High
**Impact:** High
**Risk:** High
**Effort:** ~2-3 weeks

## Goal

Decouple side effects (thumbnails, hashes, FTS updates) from CRUD operations using an event-driven architecture. This improves response times and enables independent scaling of processing tasks.

## Problem Statement

Current resource creation does everything synchronously:

```go
func (ctx *MahresourcesContext) AddResource(...) (*models.Resource, error) {
    // 1. Save file to disk (fast)
    // 2. Calculate MD5 hash (medium)
    // 3. Calculate perceptual hash (slow for images)
    // 4. Extract dimensions (medium)
    // 5. Generate thumbnail (slow)
    // 6. Update FTS index (medium)
    // 7. Save to database (fast)
    return resource, nil
}
```

Problems:
- **Slow response times:** User waits for all processing
- **Tight coupling:** Can't add new side effects without modifying CRUD
- **No retry:** If thumbnail fails, entire upload fails
- **Hard to test:** Side effects mixed with core logic

## Proposed Architecture

```
Upload Request
     ↓
Resource Created (DB + File)
     ↓
Event Published: "resource.created"
     ↓
┌─────────────────────────────────────────┐
│          Event Bus                       │
└─────────────────────────────────────────┘
     ↓          ↓           ↓           ↓
  Hasher   Thumbnailer   Indexer    Analyzer
```

## Implementation Details

### Event System Core

**New file:** `events/events.go`

```go
package events

import (
    "time"
)

// Event is the base event structure
type Event interface {
    EventType() string
    Timestamp() time.Time
}

// BaseEvent provides common event fields
type BaseEvent struct {
    Type      string    `json:"type"`
    CreatedAt time.Time `json:"created_at"`
}

func (e BaseEvent) EventType() string    { return e.Type }
func (e BaseEvent) Timestamp() time.Time { return e.CreatedAt }

// EventHandler processes events
type EventHandler interface {
    Handle(event Event) error
    Handles() []string // Event types this handler processes
}

// EventBus manages event publishing and subscription
type EventBus interface {
    Publish(event Event) error
    Subscribe(handler EventHandler)
    Start() error
    Stop() error
}
```

### Domain Events

**New file:** `events/resource_events.go`

```go
package events

import (
    "time"
)

// ResourceCreatedEvent is published when a resource is created
type ResourceCreatedEvent struct {
    BaseEvent
    ResourceID   uint   `json:"resource_id"`
    FilePath     string `json:"file_path"`
    ContentType  string `json:"content_type"`
    OriginalName string `json:"original_name"`
}

func NewResourceCreatedEvent(resourceID uint, filePath, contentType, originalName string) *ResourceCreatedEvent {
    return &ResourceCreatedEvent{
        BaseEvent: BaseEvent{
            Type:      "resource.created",
            CreatedAt: time.Now(),
        },
        ResourceID:   resourceID,
        FilePath:     filePath,
        ContentType:  contentType,
        OriginalName: originalName,
    }
}

// ResourceUpdatedEvent is published when a resource is updated
type ResourceUpdatedEvent struct {
    BaseEvent
    ResourceID uint     `json:"resource_id"`
    Changes    []string `json:"changes"` // Field names that changed
}

func NewResourceUpdatedEvent(resourceID uint, changes []string) *ResourceUpdatedEvent {
    return &ResourceUpdatedEvent{
        BaseEvent: BaseEvent{
            Type:      "resource.updated",
            CreatedAt: time.Now(),
        },
        ResourceID: resourceID,
        Changes:    changes,
    }
}

// ResourceDeletedEvent is published when a resource is deleted
type ResourceDeletedEvent struct {
    BaseEvent
    ResourceID uint   `json:"resource_id"`
    FilePath   string `json:"file_path"`
}

func NewResourceDeletedEvent(resourceID uint, filePath string) *ResourceDeletedEvent {
    return &ResourceDeletedEvent{
        BaseEvent: BaseEvent{
            Type:      "resource.deleted",
            CreatedAt: time.Now(),
        },
        ResourceID: resourceID,
        FilePath:   filePath,
    }
}
```

### In-Memory Event Bus

**New file:** `events/memory_bus.go`

```go
package events

import (
    "log"
    "sync"
)

// MemoryEventBus is a simple in-memory event bus
type MemoryEventBus struct {
    handlers map[string][]EventHandler
    mu       sync.RWMutex
    queue    chan Event
    wg       sync.WaitGroup
    stop     chan struct{}
}

func NewMemoryEventBus(queueSize int) *MemoryEventBus {
    return &MemoryEventBus{
        handlers: make(map[string][]EventHandler),
        queue:    make(chan Event, queueSize),
        stop:     make(chan struct{}),
    }
}

func (b *MemoryEventBus) Subscribe(handler EventHandler) {
    b.mu.Lock()
    defer b.mu.Unlock()

    for _, eventType := range handler.Handles() {
        b.handlers[eventType] = append(b.handlers[eventType], handler)
    }
}

func (b *MemoryEventBus) Publish(event Event) error {
    select {
    case b.queue <- event:
        return nil
    default:
        return ErrQueueFull
    }
}

func (b *MemoryEventBus) Start() error {
    b.wg.Add(1)
    go b.processEvents()
    return nil
}

func (b *MemoryEventBus) Stop() error {
    close(b.stop)
    b.wg.Wait()
    return nil
}

func (b *MemoryEventBus) processEvents() {
    defer b.wg.Done()

    for {
        select {
        case <-b.stop:
            // Drain remaining events
            for {
                select {
                case event := <-b.queue:
                    b.dispatch(event)
                default:
                    return
                }
            }
        case event := <-b.queue:
            b.dispatch(event)
        }
    }
}

func (b *MemoryEventBus) dispatch(event Event) {
    b.mu.RLock()
    handlers := b.handlers[event.EventType()]
    b.mu.RUnlock()

    for _, handler := range handlers {
        go func(h EventHandler) {
            if err := h.Handle(event); err != nil {
                log.Printf("[EVENT ERROR] %s handler failed: %v", event.EventType(), err)
            }
        }(handler)
    }
}
```

### Event Handlers

**New file:** `events/handlers/thumbnail_handler.go`

```go
package handlers

import (
    "log"
    "mahresources/events"
    "mahresources/repositories"
)

type ThumbnailHandler struct {
    resourceRepo repositories.ResourceRepository
    thumbnailer  Thumbnailer
}

func NewThumbnailHandler(repo repositories.ResourceRepository, t Thumbnailer) *ThumbnailHandler {
    return &ThumbnailHandler{
        resourceRepo: repo,
        thumbnailer:  t,
    }
}

func (h *ThumbnailHandler) Handles() []string {
    return []string{"resource.created"}
}

func (h *ThumbnailHandler) Handle(event events.Event) error {
    e, ok := event.(*events.ResourceCreatedEvent)
    if !ok {
        return nil
    }

    // Only generate thumbnails for images/videos
    if !isMediaFile(e.ContentType) {
        return nil
    }

    log.Printf("[THUMBNAIL] Generating for resource %d", e.ResourceID)

    resource, err := h.resourceRepo.FindByID(e.ResourceID)
    if err != nil {
        return err
    }

    thumbnailPath, err := h.thumbnailer.Generate(e.FilePath, e.ContentType)
    if err != nil {
        log.Printf("[THUMBNAIL] Failed for resource %d: %v", e.ResourceID, err)
        return err
    }

    resource.PreviewLoc = thumbnailPath
    return h.resourceRepo.Update(resource)
}
```

**New file:** `events/handlers/hash_handler.go`

```go
package handlers

import (
    "log"
    "mahresources/events"
    "mahresources/repositories"
)

type HashHandler struct {
    resourceRepo repositories.ResourceRepository
    hasher       Hasher
}

func NewHashHandler(repo repositories.ResourceRepository, h Hasher) *HashHandler {
    return &HashHandler{
        resourceRepo: repo,
        hasher:       h,
    }
}

func (h *HashHandler) Handles() []string {
    return []string{"resource.created"}
}

func (h *HashHandler) Handle(event events.Event) error {
    e, ok := event.(*events.ResourceCreatedEvent)
    if !ok {
        return nil
    }

    log.Printf("[HASH] Calculating for resource %d", e.ResourceID)

    resource, err := h.resourceRepo.FindByID(e.ResourceID)
    if err != nil {
        return err
    }

    // Calculate hashes
    hash, err := h.hasher.CalculateMD5(e.FilePath)
    if err != nil {
        return err
    }
    resource.Hash = hash

    // Calculate perceptual hash for images
    if isImageFile(e.ContentType) {
        phash, err := h.hasher.CalculatePerceptualHash(e.FilePath)
        if err == nil {
            resource.HashPerceptual = phash
        }
    }

    return h.resourceRepo.Update(resource)
}
```

**New file:** `events/handlers/fts_handler.go`

```go
package handlers

import (
    "log"
    "mahresources/events"
    "mahresources/repositories"
)

type FTSHandler struct {
    searchRepo repositories.SearchRepository
}

func NewFTSHandler(repo repositories.SearchRepository) *FTSHandler {
    return &FTSHandler{searchRepo: repo}
}

func (h *FTSHandler) Handles() []string {
    return []string{"resource.created", "resource.updated", "resource.deleted"}
}

func (h *FTSHandler) Handle(event events.Event) error {
    switch e := event.(type) {
    case *events.ResourceCreatedEvent:
        log.Printf("[FTS] Indexing resource %d", e.ResourceID)
        return h.searchRepo.IndexResource(e.ResourceID)
    case *events.ResourceUpdatedEvent:
        log.Printf("[FTS] Re-indexing resource %d", e.ResourceID)
        return h.searchRepo.IndexResource(e.ResourceID)
    case *events.ResourceDeletedEvent:
        log.Printf("[FTS] Removing resource %d from index", e.ResourceID)
        return h.searchRepo.RemoveResource(e.ResourceID)
    }
    return nil
}
```

### Updated Resource Service

```go
// services/resource_service.go

type ResourceService struct {
    repo     repositories.ResourceRepository
    fs       afero.Fs
    eventBus events.EventBus
}

func (s *ResourceService) UploadResource(file multipart.File, ...) (*models.Resource, error) {
    // 1. Save file (fast)
    path, err := s.saveFile(file, filename)
    if err != nil {
        return nil, err
    }

    // 2. Create resource record (fast)
    resource := &models.Resource{
        Name:         meta.Name,
        OriginalName: filename,
        Location:     path,
        ContentType:  contentType,
    }
    if err := s.repo.Create(resource); err != nil {
        s.fs.Remove(path)
        return nil, err
    }

    // 3. Publish event for async processing
    event := events.NewResourceCreatedEvent(resource.ID, path, contentType, filename)
    if err := s.eventBus.Publish(event); err != nil {
        log.Printf("Failed to publish resource.created event: %v", err)
    }

    // Return immediately - side effects happen async
    return resource, nil
}
```

## Initialization

```go
// main.go

func main() {
    // ... db setup ...

    // Create event bus
    eventBus := events.NewMemoryEventBus(1000)

    // Create repositories
    resourceRepo := gorm.NewResourceRepository(db)
    searchRepo := gorm.NewSearchRepository(db)

    // Register handlers
    eventBus.Subscribe(handlers.NewThumbnailHandler(resourceRepo, thumbnailer))
    eventBus.Subscribe(handlers.NewHashHandler(resourceRepo, hasher))
    eventBus.Subscribe(handlers.NewFTSHandler(searchRepo))

    // Start event processing
    eventBus.Start()
    defer eventBus.Stop()

    // ... server setup ...
}
```

## Directory Structure

```
mahresources/
├── events/
│   ├── events.go              # Event interfaces and base types
│   ├── memory_bus.go          # In-memory event bus implementation
│   ├── resource_events.go     # Resource domain events
│   ├── note_events.go         # Note domain events
│   ├── group_events.go        # Group domain events
│   └── handlers/
│       ├── thumbnail_handler.go
│       ├── hash_handler.go
│       ├── fts_handler.go
│       └── dimension_handler.go
```

## Event Flow Example

```
1. User uploads image.jpg
2. ResourceService saves file and DB record
3. ResourceService publishes ResourceCreatedEvent
4. ResourceService returns immediately (fast response)

5. EventBus dispatches to handlers (async):
   - ThumbnailHandler: generates thumbnail, updates DB
   - HashHandler: calculates MD5 + perceptual hash, updates DB
   - FTSHandler: indexes resource for full-text search
```

## Handling Failures

### Retry Logic

```go
type RetryingHandler struct {
    handler    EventHandler
    maxRetries int
    delay      time.Duration
}

func (h *RetryingHandler) Handle(event events.Event) error {
    var lastErr error
    for i := 0; i < h.maxRetries; i++ {
        if err := h.handler.Handle(event); err == nil {
            return nil
        } else {
            lastErr = err
            time.Sleep(h.delay * time.Duration(i+1))
        }
    }
    return lastErr
}
```

### Dead Letter Queue

```go
type DeadLetterHandler struct {
    repo DeadLetterRepository
}

func (h *DeadLetterHandler) Handle(event events.Event, err error) {
    h.repo.Save(&DeadLetter{
        Event:     event,
        Error:     err.Error(),
        Timestamp: time.Now(),
    })
}
```

## Testing

### Handler Tests

```go
func TestThumbnailHandler_Handle(t *testing.T) {
    mockRepo := &MockResourceRepository{
        resource: &models.Resource{ID: 1, ContentType: "image/jpeg"},
    }
    mockThumbnailer := &MockThumbnailer{path: "/thumb.jpg"}

    handler := NewThumbnailHandler(mockRepo, mockThumbnailer)

    event := events.NewResourceCreatedEvent(1, "/test.jpg", "image/jpeg", "test.jpg")
    err := handler.Handle(event)

    assert.NoError(t, err)
    assert.Equal(t, "/thumb.jpg", mockRepo.resource.PreviewLoc)
}
```

### Integration Tests

```go
func TestEventFlow_ResourceCreated(t *testing.T) {
    bus := events.NewMemoryEventBus(100)

    var processed sync.WaitGroup
    processed.Add(3) // thumbnail, hash, fts

    // Track handler calls
    // ... setup handlers with processed.Done() ...

    bus.Start()
    bus.Publish(events.NewResourceCreatedEvent(1, "/test.jpg", "image/jpeg", "test.jpg"))

    // Wait for all handlers
    processed.Wait()

    // Verify all handlers ran
}
```

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Eventual consistency | Document which fields may not be immediately available |
| Event loss | Use persistent event store in production (optional upgrade) |
| Processing failures | Implement retry logic and dead letter queue |
| Debugging complexity | Add correlation IDs and detailed logging |
| Race conditions | Design handlers to be idempotent |

## Future Enhancements

1. **Persistent Event Store:** Use Redis/Kafka for durability
2. **Event Sourcing:** Store all events for full audit trail
3. **Distributed Processing:** Multiple workers processing events
4. **Priority Queues:** Process thumbnails before hashes

## Success Metrics

- [ ] Event system core implemented
- [ ] All domain events defined
- [ ] Handlers for thumbnail, hash, dimensions, FTS
- [ ] Resource upload returns immediately
- [ ] Handlers process events asynchronously
- [ ] Retry logic for failed handlers
- [ ] All E2E tests passing
- [ ] Upload response time improved by >50%
