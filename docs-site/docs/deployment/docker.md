---
sidebar_position: 1
title: Docker
description: Running Mahresources in Docker containers
---

# Docker Deployment

:::danger Security Warning
Mahresources has **no built-in authentication**. Never expose Docker containers directly to the internet. Always use a reverse proxy with authentication. See [Reverse Proxy](./reverse-proxy.md) for setup instructions.
:::

## Building the Image

No pre-built Docker image is published. Build it locally from the repository root:

```bash
git clone https://github.com/egeozcan/mahresources.git
cd mahresources
docker build -t mahresources .
```

Or use the [template Dockerfile](#template-dockerfile) below if you want to customize the build.

## Quick Start with Docker Run

```bash
docker run -d \
  --name mahresources \
  -p 8181:8181 \
  -v mahresources-data:/app/data \
  -v mahresources-files:/app/files \
  -e DB_TYPE=SQLITE \
  -e DB_DSN=/app/data/mahresources.db \
  -e FILE_SAVE_PATH=/app/files \
  -e BIND_ADDRESS=0.0.0.0:8181 \
  mahresources
```

## Docker Compose

### SQLite Configuration

Create a `docker-compose.yml` file:

```yaml
services:
  mahresources:
    build: .                     # build from local Dockerfile
    container_name: mahresources
    restart: unless-stopped
    ports:
      - "8181:8181"
    volumes:
      - app-data:/app/data       # SQLite database
      - app-files:/app/files     # uploaded files
    environment:
      - DB_TYPE=SQLITE
      - DB_DSN=/app/data/mahresources.db
      - FILE_SAVE_PATH=/app/files
      - BIND_ADDRESS=0.0.0.0:8181

volumes:
  app-data:    # persist database across restarts
  app-files:   # persist uploaded files across restarts
```

Start the service:

```bash
docker compose up -d
```

### PostgreSQL Configuration

Use PostgreSQL when you need concurrent access or have a large collection:

```yaml
services:
  mahresources:
    build: .                     # build from local Dockerfile
    container_name: mahresources
    restart: unless-stopped
    ports:
      - "8181:8181"
    volumes:
      - ./data/files:/data/files    # uploaded files (bind mount)
    environment:
      - DB_TYPE=POSTGRES
      - DB_DSN=host=postgres user=mahresources password=secretpassword dbname=mahresources sslmode=disable
      - FILE_SAVE_PATH=/data/files
      - BIND_ADDRESS=:8181
    depends_on:
      postgres:
        condition: service_healthy  # wait for DB to be ready

  postgres:
    image: postgres:16-alpine
    container_name: mahresources-postgres
    restart: unless-stopped
    volumes:
      - postgres-data:/var/lib/postgresql/data
    environment:
      - POSTGRES_USER=mahresources
      - POSTGRES_PASSWORD=secretpassword    # change this
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

All configuration options can be set via environment variables. The most common:

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

Two volumes must persist across container restarts:

1. **Database volume** (`/app/data`) -- the SQLite database file
2. **Files volume** (`/app/files`) -- uploaded resources

Use named volumes (as shown above) or bind mounts.

## Updating

Pull the latest source and rebuild:

```bash
git pull
docker compose build
docker compose up -d
```

## Template Dockerfile

The repository includes a Dockerfile. If you need to customize it, here is the template:

```dockerfile
# Stage 1: Build frontend assets
FROM node:20-alpine AS frontend-builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY src/ ./src/
COPY index.css vite.config.js postcss.config.js ./
RUN npm run build-css && npm run build-js

# Stage 2: Build Go binary
FROM golang:1.22-alpine AS go-builder
RUN apk add --no-cache gcc musl-dev sqlite-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-builder /app/public/dist ./public/dist
COPY --from=frontend-builder /app/public/tailwind.css ./public/tailwind.css
RUN CGO_ENABLED=1 go build --tags 'json1 fts5' -o mahresources

# Stage 3: Runtime
FROM alpine:3.19
RUN apk add --no-cache sqlite-libs ca-certificates
WORKDIR /app
COPY --from=go-builder /app/mahresources .
COPY --from=go-builder /app/templates ./templates
COPY --from=go-builder /app/public ./public
RUN mkdir -p /app/data /app/files
ENV DB_TYPE=SQLITE
ENV DB_DSN=/app/data/mahresources.db
ENV FILE_SAVE_PATH=/app/files
ENV BIND_ADDRESS=0.0.0.0:8181
EXPOSE 8181
CMD ["./mahresources"]
```

:::note
The `gcc`, `musl-dev`, and `sqlite-dev` packages are required in the Go build stage because SQLite support requires CGO. The runtime stage only needs `sqlite-libs`.
:::
