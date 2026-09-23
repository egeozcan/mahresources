---
outputShape: Legacy download queue response
exitCodes: 0 on success; 1 on any error
relatedCmds: jobs list, job submit
---

# Long

Read the legacy in-memory download queue at `/v1/jobs/queue`. This command
preserves the response used by older scripts. The canonical `jobs list`
command reads durable Job records when the Job Center API is enabled.

# Example

  # Inspect the legacy download queue response
  mr jobs queue --json

  # Keep only legacy download IDs
  mr jobs queue --json | jq -r '.jobs[].id'

  # mr-doctest: the compatibility queue returns an array of jobs
  mr jobs queue --json | jq -e 'has("jobs") and (.jobs | type == "array")'
