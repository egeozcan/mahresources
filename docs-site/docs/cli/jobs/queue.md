---
title: mr jobs queue
description: Read the legacy download queue
sidebar_label: queue
---

# mr jobs queue

Read the legacy in-memory download queue at `/v1/jobs/queue`. This command
preserves the response used by older scripts. The canonical `jobs list`
command reads durable Job records when the Job Center API is enabled.

## Usage

```bash
mr jobs queue
```

## Examples

**Inspect the legacy download queue response**

```bash
mr jobs queue --json
```

**Keep only legacy download IDs**

```bash
mr jobs queue --json | jq -r '.jobs[].id'
```


## Flags

This command has no local flags.
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Legacy download queue response

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr jobs list`](./list.md)
- [`mr job submit`](../job/submit.md)
