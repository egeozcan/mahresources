---
sidebar_position: 1
title: Docker
description: Running Mahresources in Docker containers
---

# Docker Deployment

:::danger Security Warning
Mahresources has **no built-in authentication**. Never expose Docker containers directly to the internet. Always use a reverse proxy with authentication. See [Reverse Proxy](./reverse-proxy.md) for setup instructions.
:::

## Quick Start with Docker Run

```bash
docker run -d \
  --name mahresources \
  -p 8181:8181 \
  -v mahresources-data:/data/db \
  -v mahresources-files:/data/files \
  -e DB_TYPE=SQLITE \
  -e DB_DSN=/data/db/mahresources.db \
  -e FILE_SAVE_PATH=/data/files \
  -e BIND_ADDRESS=:8181 \
  ghcr.io/egeozcan/mahresources:latest
```

## Docker Compose

### SQLite Configuration

Create a `docker-compose.yml` file:

```yaml
version: '3.8'

services:
  mahresources:
    image: ghcr.io/egeozcan/mahresources:latest
    container_name: mahresources
    restart: unless-stopped
    ports:
      - "8181:8181"
    volumes:
      - ./data/db:/data/db
      - ./data/files:/data/files
    environment:
      - DB_TYPE=SQLITE
      - DB_DSN=/data/db/mahresources.db
      - FILE_SAVE_PATH=/data/files
      - BIND_ADDRESS=:8181

volumes:
  mahresources-db:
  mahresources-files:
```

Start the service:

```bash
docker compose up -d
```

### PostgreSQL Configuration

For larger deployments or when you need better concurrent access:

```yaml
version: '3.8'

services:
  mahresources:
    image: ghcr.io/egeozcan/mahresources:latest
    container_name: mahresources
    restart: unless-stopped
    ports:
      - "8181:8181"
    volumes:
      - ./data/files:/data/files
    environment:
      - DB_TYPE=POSTGRES
      - DB_DSN=host=postgres user=mahresources password=secretpassword dbname=mahresources sslmode=disable
      - FILE_SAVE_PATH=/data/files
      - BIND_ADDRESS=:8181
    depends_on:
      postgres:
        condition: service_healthy

  postgres:
    image: postgres:16-alpine
    container_name: mahresources-postgres
    restart: unless-stopped
    volumes:
      - postgres-data:/var/lib/postgresql/data
    environment:
      - POSTGRES_USER=mahresources
      - POSTGRES_PASSWORD=secretpassword
      - POSTGRES_DB=mahresources
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U mahresources"]
      interval: 5s
      timeout: 5s
      retries: 5

volumes:
  postgres-data:
```

## Environment Variables

All configuration options can be set via environment variables:

| Variable | Description | Example |
|----------|-------------|---------|
| `DB_TYPE` | Database type | `SQLITE` or `POSTGRES` |
| `DB_DSN` | Database connection string | `/data/db/mahresources.db` |
| `FILE_SAVE_PATH` | File storage directory | `/data/files` |
| `BIND_ADDRESS` | Server bind address | `:8181` |
| `FFMPEG_PATH` | Path to ffmpeg binary | `/usr/bin/ffmpeg` |
| `LIBREOFFICE_PATH` | Path to LibreOffice | `/usr/bin/soffice` |
| `SKIP_FTS` | Skip full-text search init | `1` |
| `HASH_WORKER_DISABLED` | Disable hash worker | `1` |

## Volume Mounts

Two volumes are essential for persistent data:

1. **Database volume** (`/data/db`): Stores the SQLite database file
2. **Files volume** (`/data/files`): Stores uploaded resources

:::tip
Use named volumes or bind mounts to ensure data persists across container restarts and updates.
:::

## Updating

To update to the latest version:

```bash
docker compose pull
docker compose up -d
```

## Building Your Own Image

If you need to build the image yourself:

```dockerfile
FROM golang:1.22-alpine AS builder
RUN apk add --no-cache git nodejs npm
WORKDIR /app
COPY . .
RUN npm install && npm run build
RUN go build --tags 'json1 fts5' -o mahresources

FROM alpine:latest
RUN apk add --no-cache ffmpeg libreoffice
COPY --from=builder /app/mahresources /usr/local/bin/
COPY --from=builder /app/public /app/public
COPY --from=builder /app/templates /app/templates
WORKDIR /app
EXPOSE 8181
CMD ["mahresources"]
```
