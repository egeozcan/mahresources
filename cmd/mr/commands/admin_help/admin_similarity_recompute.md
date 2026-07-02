---
outputShape: Object with jobId (the background job identifier)
exitCodes: 0 on success; 1 on error; the API returns 409 if a recompute is already running
relatedCmds: admin similarity retry-failed, admin stats
---

# Long

Submit a background job that deletes every similarity pair whose both endpoints are v2 rows and rebuilds them from the stored perceptual hashes. This performs no image decoding (it reads hashes from the database), so it is cheap enough to run after an algorithm or threshold change. Only one recompute may run at a time; a second request while one is active returns HTTP 409.

Progress is visible in the background jobs list and on the admin overview page.

# Example

  # Start a recompute
  mr admin similarity recompute

  # Start a recompute and print the raw job JSON
  mr admin similarity recompute --json
