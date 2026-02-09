---
sidebar_position: 3
title: Reverse Proxy
description: Configuring a reverse proxy with authentication for Mahresources
---

# Reverse Proxy Configuration

:::danger Required for Remote Access
Mahresources has **no built-in authentication or authorization**. You **must** use a reverse proxy with authentication if accessing from outside your local network. Exposing Mahresources directly to the internet will allow anyone to access, modify, and delete all your data.
:::

## Network Restriction

At minimum, ensure Mahresources only binds to localhost:

```bash
# In your .env or command line
BIND_ADDRESS=127.0.0.1:8181
```

This prevents direct external access even if your firewall is misconfigured.

## Nginx with Basic Authentication

### Install Nginx and Create Password File

```bash
# Install nginx and apache2-utils (for htpasswd)
sudo apt install nginx apache2-utils

# Create password file
sudo htpasswd -c /etc/nginx/.htpasswd yourusername
```

### Nginx Configuration

Create `/etc/nginx/sites-available/mahresources`:

```nginx
server {
    listen 80;
    server_name mahresources.example.com;

    # Redirect HTTP to HTTPS
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name mahresources.example.com;

    # SSL certificates (use Let's Encrypt)
    ssl_certificate /etc/letsencrypt/live/mahresources.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/mahresources.example.com/privkey.pem;

    # Basic authentication
    auth_basic "Mahresources";
    auth_basic_user_file /etc/nginx/.htpasswd;

    # Increase body size for file uploads
    client_max_body_size 2G;

    location / {
        proxy_pass http://127.0.0.1:8181;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Timeouts for large file uploads
        proxy_connect_timeout 300;
        proxy_send_timeout 300;
        proxy_read_timeout 300;
    }
}
```

Enable the site:

```bash
sudo ln -s /etc/nginx/sites-available/mahresources /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

## Caddy

Caddy provides automatic HTTPS and simpler configuration.

### Caddyfile

```caddyfile
mahresources.example.com {
    # Basic authentication
    basicauth /* {
        yourusername $2a$14$hashedpasswordhere
    }

    # Reverse proxy to Mahresources
    reverse_proxy localhost:8181 {
        # Increase timeouts for large uploads
        transport http {
            response_header_timeout 300s
        }
    }

    # Increase request body limit for uploads
    request_body {
        max_size 2GB
    }
}
```

Generate the password hash:

```bash
caddy hash-password
```

## Traefik

### Docker Compose with Traefik

```yaml
version: '3.8'

services:
  traefik:
    image: traefik:v2.10
    command:
      - "--api.insecure=true"
      - "--providers.docker=true"
      - "--entrypoints.web.address=:80"
      - "--entrypoints.websecure.address=:443"
      - "--certificatesresolvers.letsencrypt.acme.httpchallenge=true"
      - "--certificatesresolvers.letsencrypt.acme.httpchallenge.entrypoint=web"
      - "--certificatesresolvers.letsencrypt.acme.email=you@example.com"
      - "--certificatesresolvers.letsencrypt.acme.storage=/letsencrypt/acme.json"
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - traefik-certs:/letsencrypt

  mahresources:
    build: .                     # build from local Dockerfile
    volumes:
      - ./data/db:/data/db
      - ./data/files:/data/files
    environment:
      - DB_TYPE=SQLITE
      - DB_DSN=/data/db/mahresources.db
      - FILE_SAVE_PATH=/data/files
      - BIND_ADDRESS=:8181
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.mahresources.rule=Host(`mahresources.example.com`)"
      - "traefik.http.routers.mahresources.entrypoints=websecure"
      - "traefik.http.routers.mahresources.tls.certresolver=letsencrypt"
      - "traefik.http.services.mahresources.loadbalancer.server.port=8181"
      # Basic auth middleware
      - "traefik.http.routers.mahresources.middlewares=mahresources-auth"
      - "traefik.http.middlewares.mahresources-auth.basicauth.users=yourusername:$$apr1$$hashedpass"

volumes:
  traefik-certs:
```

Generate the password hash for Traefik:

```bash
# Install htpasswd
sudo apt install apache2-utils

# Generate hash (note: escape $ as $$ in docker-compose)
htpasswd -nb yourusername yourpassword
```

## Alternative Authentication Methods

### OAuth2 Proxy

For more advanced authentication (Google, GitHub, etc.), consider using [OAuth2 Proxy](https://oauth2-proxy.github.io/oauth2-proxy/):

```bash
# Example with Google OAuth
oauth2-proxy \
  --upstream=http://127.0.0.1:8181 \
  --http-address=0.0.0.0:4180 \
  --provider=google \
  --client-id=your-client-id \
  --client-secret=your-client-secret \
  --email-domain=yourdomain.com
```

### Authelia

For self-hosted SSO, [Authelia](https://www.authelia.com/) provides two-factor authentication and user management.

## Security Checklist

- [ ] Mahresources binds only to localhost (`127.0.0.1:8181`)
- [ ] Reverse proxy requires authentication for all requests
- [ ] HTTPS is enabled with valid certificates
- [ ] Strong passwords are used for basic auth
- [ ] Firewall blocks direct access to port 8181
- [ ] Regular security updates are applied
