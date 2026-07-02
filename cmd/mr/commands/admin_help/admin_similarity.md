---
outputShape: Subcommand group; run a subcommand
exitCodes: 0 on success; 1 on error
relatedCmds: admin similarity recompute, admin similarity retry-failed, admin stats
---

# Long

Image-similarity maintenance actions for the perceptual-hash (v2) engine. Use the subcommands to rebuild similarity pairs from stored hashes or to re-queue images whose hashing previously failed.

# Example

  # Rebuild all v2 similarity pairs
  mr admin similarity recompute

  # Retry images whose hashing failed
  mr admin similarity retry-failed
