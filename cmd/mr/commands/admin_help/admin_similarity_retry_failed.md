---
outputShape: Object with reset (number of failed rows re-queued)
exitCodes: 0 on success; 1 on error
relatedCmds: admin similarity recompute, admin stats
---

# Long

Reset image_hashes rows that were marked failed (undecodable file at hash time) so the background backfill worker attempts them again. Prints how many rows were re-queued. Use this after fixing missing files or storage configuration.

# Example

  # Re-queue all failed hashes
  mr admin similarity retry-failed

  # Print the raw JSON result
  mr admin similarity retry-failed --json
