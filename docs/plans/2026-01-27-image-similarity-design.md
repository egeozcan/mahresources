# Image Similarity Background Processing Design

## Overview

Integrate perceptual hash calculation and similarity detection into the main application as a background task, replacing the standalone `cmd/perceptualHash` CLI tool. Add visual similarity matching using Hamming distance thresholds.

## Goals

- Background hash calculation that doesn't block CRUD operations
- Visual similarity detection (not just exact matches)
- Performant with ~2 million resources
- Configurable worker settings for different hardware

## Decisions

| Decision | Choice |
|----------|--------|
| Similarity type | Visual (Hamming distance on perceptual hashes) |
| Background processing | Hybrid: on-upload async + periodic batch |
| Similarity storage | Pre-computed pairs in dedicated table |
| Threshold | ≤10 Hamming distance (~84% similarity) |
| Content types | JPEG, PNG, GIF (first frame), WebP |
| Worker config | Configurable via flags/env |

## Data Model

### Updated `image_hashes` table

Migrate from string to uint64 for efficient Hamming distance calculation:

```go
type ImageHash struct {
    ID         uint      `gorm:"primarykey"`
    AHash      string    `gorm:"index"`           // old, kept during migration
    DHash      string    `gorm:"index"`           // old, kept during migration
    AHashInt   *uint64   `gorm:"index"`           // new
    DHashInt   *uint64   `gorm:"index"`           // new
    Resource   *Resource `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
    ResourceId *uint     `gorm:"uniqueIndex"`
}

func (h *ImageHash) GetDHash() uint64 {
    if h.DHashInt != nil {
        return *h.DHashInt
    }
    val, _ := strconv.ParseUint(h.DHash, 16, 64)
    return val
}
```

### New `resource_similarities` table

```go
type ResourceSimilarity struct {
    ID              uint      `gorm:"primarykey"`
    ResourceID1     uint      `gorm:"index:idx_sim_r1;index:idx_sim_pair,unique"`
    ResourceID2     uint      `gorm:"index:idx_sim_r2;index:idx_sim_pair,unique"`
    HammingDistance uint8
    Resource1       *Resource `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignKey:ResourceID1"`
    Resource2       *Resource `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignKey:ResourceID2"`
}
```

- Ordering convention: Always store `ResourceID1 < ResourceID2` to avoid duplicates
- Cascade delete: When either resource is deleted, similarity row is removed

## Background Worker Architecture

### HashWorker

```go
type HashWorker struct {
    ctx           *MahresourcesContext
    workerCount   int           // default: 4
    batchSize     int           // default: 500
    pollInterval  time.Duration // default: 1 minute
    threshold     int           // default: 10
    hashCache     map[uint]uint64
    cacheMutex    sync.RWMutex
    hashQueue     chan uint     // for on-upload async
    stopCh        chan struct{}
}

func (w *HashWorker) Start()
func (w *HashWorker) Stop()
```

### Processing Flow

1. **On-upload hook**: When a resource is created with hashable content type, queue it for immediate async processing via buffered channel
2. **Batch worker**: Runs every poll interval, processes unhashed resources in batches
3. **Migration duty**: Also migrates old string hashes to uint64 during normal operation

### Similarity Calculation

```go
func hammingDistance(a, b uint64) int {
    return bits.OnesCount64(a ^ b)
}
```

When a resource is hashed:
1. Decode image, calculate DHash (uint64)
2. Load all existing hashes into memory cache (~32MB for 2M entries)
3. For each existing hash, if Hamming distance ≤ threshold, record the pair
4. Batch insert similarity pairs
5. Save the new hash

## Query Layer

```go
func (ctx *MahresourcesContext) GetSimilarResources(id uint) ([]*models.Resource, error) {
    var similarIDs []uint
    ctx.db.Model(&models.ResourceSimilarity{}).
        Select("CASE WHEN resource_id_1 = ? THEN resource_id_2 ELSE resource_id_1 END", id).
        Where("resource_id_1 = ? OR resource_id_2 = ?", id, id).
        Order("hamming_distance ASC").
        Pluck("resource_id", &similarIDs)

    if len(similarIDs) == 0 {
        return nil, nil
    }

    var resources []*models.Resource
    err := ctx.db.
        Preload("Tags").
        Joins("Owner").
        Where("id IN ?", similarIDs).
        Find(&resources).Error

    return resources, err
}
```

Results ordered by Hamming distance (most similar first).

## Lifecycle & Cleanup

### Resource deletion
- Cascade delete handles both tables automatically

### Resource file replacement
- Delete old hash (cascade removes similarity pairs)
- Re-queue for hashing

```go
func (ctx *MahresourcesContext) OnResourceFileChanged(resourceID uint) {
    ctx.db.Where("resource_id = ?", resourceID).Delete(&models.ImageHash{})
    ctx.hashQueue <- resourceID
}
```

### Graceful shutdown
- `HashWorker.Stop()` signals workers to finish current batch
- Wait for in-flight work before app exit

## Migration Plan

### Non-blocking dual-column approach

**Step 1: Schema change (instant)**
```sql
ALTER TABLE image_hashes ADD COLUMN IF NOT EXISTS a_hash_int BIGINT;
ALTER TABLE image_hashes ADD COLUMN IF NOT EXISTS d_hash_int BIGINT;
CREATE INDEX IF NOT EXISTS idx_a_hash_int ON image_hashes(a_hash_int);
CREATE INDEX IF NOT EXISTS idx_d_hash_int ON image_hashes(d_hash_int);
```

**Step 2: Background migration**

Hash worker migrates old string hashes to uint64 during normal operation:
```go
// Priority 1: Migrate existing string hashes
var toMigrate []ImageHash
w.db.Where("d_hash_int IS NULL AND d_hash IS NOT NULL").
    Limit(w.batchSize).Find(&toMigrate)

for _, h := range toMigrate {
    aHash, _ := strconv.ParseUint(h.AHash, 16, 64)
    dHash, _ := strconv.ParseUint(h.DHash, 16, 64)
    w.db.Model(&h).Updates(map[string]any{
        "a_hash_int": aHash,
        "d_hash_int": dHash,
    })
}

// Priority 2: Hash new resources
```

**Step 3: Cleanup (future release)**

Once no rows have `d_hash_int IS NULL`, drop old string columns.

## Configuration

New flags/env vars:

| Flag | Env Variable | Default | Description |
|------|--------------|---------|-------------|
| `-hash-worker-count` | `HASH_WORKER_COUNT` | 4 | Concurrent hash workers |
| `-hash-batch-size` | `HASH_BATCH_SIZE` | 500 | Resources per batch |
| `-hash-poll-interval` | `HASH_POLL_INTERVAL` | 1m | Time between batch cycles |
| `-hash-similarity-threshold` | `HASH_SIMILARITY_THRESHOLD` | 10 | Max Hamming distance |
| `-hash-worker-disabled` | `HASH_WORKER_DISABLED` | false | Disable hash worker |

## Deprecation

Delete `cmd/perceptualHash/` directory. All functionality absorbed into main app.

## Content Types

Hashable content types:
- `image/jpeg`
- `image/png`
- `image/gif` (first frame only)
- `image/webp`
