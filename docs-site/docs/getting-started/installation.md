---
sidebar_position: 1
---

# Installation

:::danger Security Warning

Authentication is **off by default**. Never expose the server directly to the public internet -- run it on a private network or behind a reverse proxy with authentication. Built-in [Authentication & RBAC](../features/authentication.md) is available (opt-in via `-auth`) when you need per-user accounts and roles.

:::

## Prerequisites

### Building from Source
- **Go 1.26+** - [Download Go](https://go.dev/dl/)
- **Node.js 20.19+ or 22.12+** - [Download Node.js](https://nodejs.org/). This is Vite 8's supported range; older 20.x and 22.x releases fail `npm install` with `EBADENGINE`
- **A C compiler** - the SQLite driver is cgo-based, so it needs one: Xcode command line tools on macOS, `build-essential` on Debian or Ubuntu. Do not set `CGO_ENABLED=0`, or the binary builds without working SQLite

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

:::note Run it from the repository root

The binary is not self-contained. It loads Pongo2 templates from `./templates` and serves static assets from `./public`, both relative to the process working directory. Start `./mahresources` from the repository root, or copy the binary together with those two directories and start it from there.

:::

### The `mr` CLI

The `mr` command-line client is a separate binary:

```bash
npm run build-cli     # produces ./mr
make install-cli      # optional: installs it onto your PATH
```

When the server runs with `-auth`, authenticate once with `mr auth login`, or set `MR_TOKEN` to an API token.

## Option 2: Docker

No pre-built image is published. Build it locally from the repository:

```bash
git clone https://github.com/egeozcan/mahresources.git
cd mahresources
docker build -t mahresources .

# Run in ephemeral mode (data lost on exit)
docker run -p 8181:8181 mahresources ./mahresources -ephemeral

# Run with persistent storage
docker run -p 8181:8181 \
  -v mahresources-data:/app/data \
  -v mahresources-files:/app/files \
  mahresources
```

The persistent storage example stores the database as `data/test.db` (the Dockerfile default). See the [Docker deployment guide](../deployment/docker) for compose files, custom database names, and production setup.

:::note
The default Docker image disables full-text search (`SKIP_FTS=1`). To enable search, add `-e SKIP_FTS=0` to your `docker run` command.
:::

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

Either the `magick` command (ImageMagick 7) or the `convert` command (ImageMagick 6) must be available in your PATH. No additional configuration flag is needed.

## Next Steps

Next: [Quick Start](./quick-start) to run the application for the first time.
