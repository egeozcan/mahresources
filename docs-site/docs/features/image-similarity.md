---
sidebar_position: 2
---

# Image Similarity Detection

Mahresources finds visually similar images using perceptual hashing. It detects duplicates, near-duplicates, and related images even when they differ in resolution, compression, or minor edits.

## How It Works

### Perceptual Hashing

Unlike cryptographic hashes (SHA1, MD5) which produce completely different outputs for any change, perceptual hashes (pHash) produce similar outputs for visually similar images.

Mahresources uses two types of perceptual hashes:

| Hash Type | Description |
|-----------|-------------|
| **Average Hash (aHash)** | Compares the average brightness of image blocks |
| **Difference Hash (dHash)** | Compares brightness gradients between adjacent pixels |

Mahresources uses the difference hash (dHash) for similarity comparison because it tolerates small changes (crops, compression, resizing) better than average hash.

### Hamming Distance

Similarity is measured by **Hamming distance** - the number of bits that differ between two hashes. Lower distance means more similar:

| Hamming Distance | Interpretation |
|------------------|----------------|
| 0 | Identical images (perceptually) |
| 1-5 | Near-duplicates (same image, minor edits) |
| 6-10 | Similar images (same subject, different versions) |
| 11-15 | Loosely related (similar composition) |
| 16+ | Different images |

## Background Hash Worker

Mahresources runs a background worker that automatically processes images and calculates their hashes.

### What Gets Processed

The hash worker processes resources with these content types:
- `image/jpeg`
- `image/png`
- `image/gif`
- `image/webp`

Other file types are skipped.

### Processing Flow

1. **Batch discovery** - The worker finds images without hashes
2. **Hash calculation** - Workers compute aHash and dHash for each image
3. **Cache update** - New hashes are added to the in-memory cache
4. **Similarity detection** - New hashes are compared against all cached hashes
5. **Persistence** - Similar pairs are stored in the database

### Worker Configuration

Configure the hash worker using command-line flags or environment variables:

| Flag | Env Variable | Default | Description |
|------|--------------|---------|-------------|
| `-hash-worker-count` | `HASH_WORKER_COUNT` | `4` | Concurrent workers |
| `-hash-batch-size` | `HASH_BATCH_SIZE` | `500` | Images per batch |
| `-hash-poll-interval` | `HASH_POLL_INTERVAL` | `1m` | Time between batches |
| `-hash-similarity-threshold` | `HASH_SIMILARITY_THRESHOLD` | `10` | Max Hamming distance |
| `-hash-worker-disabled` | `HASH_WORKER_DISABLED=1` | `false` | Disable entirely |

### Tuning Examples

**High-performance setup** (fast processing, more strict matching):
```bash
./mahresources \
  -hash-worker-count=8 \
  -hash-batch-size=1000 \
  -hash-poll-interval=30s \
  -hash-similarity-threshold=8 \
  ...
```

**Resource-constrained setup** (slower, gentler on resources):
```bash
./mahresources \
  -hash-worker-count=1 \
  -hash-batch-size=100 \
  -hash-poll-interval=5m \
  ...
```

**Disabled** (no background processing):
```bash
./mahresources -hash-worker-disabled ...
```

## Similarity Threshold Configuration

The `-hash-similarity-threshold` setting controls how similar images must be to be considered matches:

| Threshold | Effect |
|-----------|--------|
| 5 | Strict - only near-identical images match |
| 10 (default) | Balanced - finds similar images with variations |
| 15 | Loose - includes more distant matches |
| 20+ | Very loose - may include false positives |

**Choose based on your use case:**

- **Deduplication** - Use a low threshold (5-8) to find true duplicates
- **Related images** - Use default (10) for variations like crops, resizes
- **Broad discovery** - Use higher threshold (12-15) to find related content

## Viewing Similar Images

On any resource's detail page, if similar images exist, you will see a **Similar Resources** section showing:

- Thumbnails of all similar images
- Links to each similar resource
- A form to merge similar resources into one

### Finding Images with Similarities

Use the resource search with the filter:

```
/resources?ShowWithSimilar=true
```

This shows only resources that have at least one similar image detected.

## Merging Duplicates

When you find duplicates, you can merge them:

1. Navigate to the resource you want to keep (the "winner")
2. Find the **Similar Resources** section
3. Click **Merge Others To This**
4. Confirm the action

Merging:
- Keeps the winner resource with all its metadata
- Transfers all tags, notes, and group associations from merged resources
- Deletes the merged resources
- Preserves the winner's version history

:::warning
Merging is permanent. The merged resources are deleted. Make sure you have selected the correct resource to keep.
:::

## Memory Considerations

The hash worker maintains an in-memory cache of all image hashes for fast similarity lookups. Memory usage depends on your image count:

| Image Count | Estimated Cache Size |
|-------------|---------------------|
| 10,000 | ~0.2 MB |
| 100,000 | ~2.4 MB |
| 1,000,000 | ~24 MB |
| 10,000,000 | ~240 MB |

For very large collections, this is generally acceptable on modern systems. The cache is loaded at startup and updated incrementally.

## On-Upload Processing

When you upload a new image, it is queued for immediate hash processing. This means:

1. Upload completes and resource is created
2. Resource ID is added to the hash queue
3. Worker processes it (usually within seconds)
4. Similar images appear on the resource page

If the queue is full (1000 items), new uploads fall back to batch processing on the next poll interval.

## Hash Migration

If you have images that were uploaded before hash calculation was available, the hash worker automatically processes them during its batch cycles. No manual intervention is required.

The worker also handles migration of hash format changes (string to integer representation) transparently.

## Troubleshooting

### Similar images not appearing

1. Check that the hash worker is running (not disabled)
2. Wait for the next poll interval
3. Check logs for processing errors
4. Verify the image format is supported

### Too many false positives

Lower the similarity threshold:
```bash
./mahresources -hash-similarity-threshold=6 ...
```

### Missing obvious duplicates

Raise the similarity threshold:
```bash
./mahresources -hash-similarity-threshold=15 ...
```

### High memory usage

If the hash cache is too large:
1. Consider if all images need similarity detection
2. Disable the worker if not needed: `-hash-worker-disabled`
3. Add more system memory
