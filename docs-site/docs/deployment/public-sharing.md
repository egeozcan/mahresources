---
sidebar_position: 5
title: Public Sharing
description: Securely deploy the share server for public access
---

# Public Sharing Deployment

Deploy the note sharing feature for public access while keeping the main instance private.

:::danger Security Implications
The share server makes shared notes accessible to **anyone with the URL** -- no authentication is required. Shared notes expose their full text, embedded resources, and metadata. Only share notes you are comfortable making fully public. The share URL contains an unguessable token, but anyone who obtains it can view the note.
:::

## Architecture Overview

The recommended architecture:

```
Internet → HTTPS Reverse Proxy → Share Server (:8383)
                                      ↓
Private Network → Main Server (:8181) → Database
```

Key principles:
- Main Mahresources instance stays on private network
- Only the share server is exposed publicly
- HTTPS termination at reverse proxy
- Rate limiting on public endpoint

## Configuration Options

### Share Server Flags

| Flag | Env Variable | Description | Default |
|------|--------------|-------------|---------|
| `-share-port` | `SHARE_PORT` | Port for share server | (disabled) |
| `-share-bind-address` | `SHARE_BIND_ADDRESS` | Interface to bind | `0.0.0.0` |

:::caution
The default bind address is `0.0.0.0`, which listens on all interfaces. For production deployments, set this to `127.0.0.1` and use a reverse proxy to control public access.
:::

### Basic Setup

Enable the share server by specifying a port:

```bash
./mahresources \
  -db-type=SQLITE \
  -db-dsn=./data/mahresources.db \
  -file-save-path=./data/files \
  -bind-address=127.0.0.1:8181 \
  -share-port=8383 \
  -share-bind-address=127.0.0.1
```

This starts:
- Main server on `127.0.0.1:8181` (private)
- Share server on `127.0.0.1:8383` (to be proxied)

### Environment Variables

For production deployment:

```bash
# .env file
DB_TYPE=SQLITE
DB_DSN=/data/db/mahresources.db
FILE_SAVE_PATH=/data/files
BIND_ADDRESS=127.0.0.1:8181
SHARE_PORT=8383
SHARE_BIND_ADDRESS=127.0.0.1
```

## Docker Deployment

### Docker Compose with Sharing

```yaml
services:
  mahresources:
    build: .                     # build from local Dockerfile
    container_name: mahresources
    restart: unless-stopped
    ports:
      # Main server - internal only
      - "127.0.0.1:8181:8181"
      # Share server - will be proxied
      - "127.0.0.1:8383:8383"
    volumes:
      - ./data/db:/data/db
      - ./data/files:/data/files
    environment:
      - DB_TYPE=SQLITE
      - DB_DSN=/data/db/mahresources.db
      - FILE_SAVE_PATH=/data/files
      - BIND_ADDRESS=:8181
      - SHARE_PORT=8383
      - SHARE_BIND_ADDRESS=0.0.0.0
```

Note: `SHARE_BIND_ADDRESS=0.0.0.0` inside the container allows connections from the Docker host, while the port mapping `127.0.0.1:8383:8383` keeps it local to the host.

## Reverse Proxy Configuration

### Nginx

Create a configuration for the public share server:

```nginx
# /etc/nginx/sites-available/share.example.com
server {
    listen 443 ssl http2;
    server_name share.example.com;

    ssl_certificate /etc/letsencrypt/live/share.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/share.example.com/privkey.pem;

    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;

    # Rate limiting
    limit_req zone=share burst=20 nodelay;
    limit_conn addr 10;

    location / {
        proxy_pass http://127.0.0.1:8383;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Disable buffering for real-time updates
        proxy_buffering off;
    }

    # Block access to non-share paths
    location ~ ^/(?!s/) {
        return 404;
    }
}
```

Add rate limiting zone in `nginx.conf`:

```nginx
http {
    # Rate limiting for share server
    limit_req_zone $binary_remote_addr zone=share:10m rate=10r/s;
    limit_conn_zone $binary_remote_addr zone=addr:10m;

    # ...
}
```

### Caddy

```caddyfile
share.example.com {
    reverse_proxy localhost:8383

    # Rate limiting (requires rate_limit plugin)
    rate_limit {
        zone share {
            key {remote_host}
            events 100
            window 10s
        }
    }

    # Only allow /s/ paths
    @notshare not path /s/*
    respond @notshare 404
}
```

### Traefik

```yaml
# docker-compose.yml addition
services:
  traefik:
    image: traefik:v2.10
    command:
      - "--providers.docker=true"
      - "--entrypoints.websecure.address=:443"
      - "--certificatesresolvers.letsencrypt.acme.tlschallenge=true"
      - "--certificatesresolvers.letsencrypt.acme.email=admin@example.com"
      - "--certificatesresolvers.letsencrypt.acme.storage=/letsencrypt/acme.json"
    ports:
      - "443:443"
    volumes:
      - "/var/run/docker.sock:/var/run/docker.sock:ro"
      - "./letsencrypt:/letsencrypt"

  mahresources:
    # ... other config ...
    labels:
      - "traefik.enable=true"
      # Share server
      - "traefik.http.routers.share.rule=Host(`share.example.com`) && PathPrefix(`/s/`)"
      - "traefik.http.routers.share.entrypoints=websecure"
      - "traefik.http.routers.share.tls.certresolver=letsencrypt"
      - "traefik.http.routers.share.service=share"
      - "traefik.http.services.share.loadbalancer.server.port=8383"
```

## Security Hardening

### Rate Limiting

Prevent abuse by limiting requests per IP:

- **Nginx**: `limit_req` and `limit_conn` directives
- **Caddy**: `rate_limit` plugin
- **Traefik**: Rate limiting middleware
- **Cloudflare**: Use their rate limiting rules

Recommended limits:
- 10-20 requests per second per IP
- 5-10 concurrent connections per IP
- Block IPs that exceed limits

### Firewall Rules

Only expose the share server port through your reverse proxy:

```bash
# UFW example
ufw default deny incoming
ufw allow ssh
ufw allow 443/tcp  # HTTPS for reverse proxy
# Don't allow direct access to 8383
```

### Content Security

Before sharing notes:
1. Review content for sensitive information
2. Check embedded resources for private data
3. Consider who will have access to the URL

### Monitoring

Set up monitoring for the share server:
- Track request rates and patterns
- Alert on unusual traffic spikes
- Log access for audit trails

## Testing Your Setup

### Verify Share Server is Running

```bash
# Check share server is listening
curl http://localhost:8383/
# Should return 404 (no valid share token)

# Share a note via main server
curl -X POST "http://localhost:8181/v1/note/share?noteId=1"
# Returns: {"shareToken":"abc123...","shareUrl":"/s/abc123..."}

# Access shared note
curl http://localhost:8383/s/abc123...
# Should return HTML content
```

### Verify Proxy Configuration

```bash
# Test public endpoint
curl -I https://share.example.com/s/abc123...
# Should return 200 OK with correct headers

# Verify non-share paths are blocked
curl -I https://share.example.com/v1/notes
# Should return 404
```

## Troubleshooting

### Share Server Not Starting

Check logs for errors:
```bash
./mahresources -share-port=8383 2>&1 | grep -i share
```

Common issues:
- Port already in use
- Invalid bind address
- Permission denied (binding to low port)

### Notes Not Accessible

1. Verify the note is shared (check for token in database)
2. Check share server is running on correct port
3. Verify reverse proxy is forwarding correctly
4. Check firewall rules

### HTTPS Certificate Issues

Ensure your certificate covers the share domain:
```bash
openssl s_client -connect share.example.com:443 -servername share.example.com
```

## Performance Considerations

The share server is lightweight. For shared notes with large embedded resources, consider adding reverse proxy caching for static assets.
