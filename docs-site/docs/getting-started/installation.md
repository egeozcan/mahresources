---
sidebar_position: 1
---

# Installation

:::danger Security Warning

There is **no built-in authentication**. Never expose the server directly to the public internet. Always run it on a private network or behind a reverse proxy with proper authentication.

:::

## Prerequisites

### Building from Source
- **Go 1.22+** - [Download Go](https://go.dev/dl/)
- **Node.js 20.19+** - [Download Node.js](https://nodejs.org/)

### Docker
- **Docker 20+** - [Install Docker](https://docs.docker.com/get-docker/)

## Option 1: Build from Source

```bash
git clone https://github.com/egeozcan/mahresources.git
cd mahresources
npm install
npm run build
```

`npm run build` compiles Tailwind CSS, bundles JavaScript with Vite, and builds the Go binary with `json1` and `fts5` build tags.

## Option 2: Docker

No pre-built image is published. Build it locally from the repository:

:::caution Dockerfile Go version

The Dockerfile currently uses `golang:1.21-alpine`, but the module requires Go 1.22+ (toolchain 1.24). You may need to update line 11 of the Dockerfile to `golang:1.24-alpine` before building.

:::

```bash
git clone https://github.com/egeozcan/mahresources.git
cd mahresources
docker build -t mahresources .

# Run in ephemeral mode (data lost on exit)
docker run -p 8181:8181 mahresources -ephemeral

# Run with persistent storage
docker run -p 8181:8181 \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/files:/app/files \
  mahresources
```

The persistent storage example stores the database as `data/test.db` (the Dockerfile default). See the [Docker deployment guide](../deployment/docker) for compose files, custom database names, and production setup.

## Optional Dependencies

### FFmpeg (Video Thumbnails)

Install FFmpeg to generate thumbnails for video files.

```bash
# macOS
brew install ffmpeg

# Ubuntu/Debian
sudo apt install ffmpeg

# Windows (via Chocolatey)
choco install ffmpeg
```

Then specify the path (if not in PATH):
```bash
./mahresources -ffmpeg-path=/usr/bin/ffmpeg
```

### LibreOffice (Document Thumbnails)

Install LibreOffice to generate thumbnails for Office documents (Word, Excel, PowerPoint, etc.).

```bash
# macOS
brew install --cask libreoffice

# Ubuntu/Debian
sudo apt install libreoffice

# Windows
# Download from https://www.libreoffice.org/download/
```

`soffice` or `libreoffice` in your PATH is auto-detected. To specify a custom path:
```bash
./mahresources -libreoffice-path=/usr/bin/libreoffice
```

### ImageMagick (HEIC/AVIF Thumbnails)

Install ImageMagick to generate thumbnails for HEIC and AVIF images. Mahresources falls back to ImageMagick's `convert` command when the standard Go image decoders cannot handle a format.

```bash
# macOS
brew install imagemagick

# Ubuntu/Debian
sudo apt install imagemagick

# Windows (via Chocolatey)
choco install imagemagick
```

The `convert` command must be available in your PATH. No additional configuration flag is needed.

## Next Steps

Next: [Quick Start](./quick-start) to run the application for the first time.
