---
sidebar_position: 2
title: Systemd Service
description: Running Mahresources as a systemd service on Linux
---

# Systemd Service

Run the application as a systemd service on Linux.

## Create a Service User

Create a dedicated user for the service:

```bash
sudo useradd -r -s /bin/false -d /opt/mahresources mahresources
```

## Install the Binary

Download or build the binary and install it:

```bash
# Create directories
sudo mkdir -p /opt/mahresources/{bin,data,files}

# Copy the binary
sudo cp mahresources /opt/mahresources/bin/

# Set ownership
sudo chown -R mahresources:mahresources /opt/mahresources
```

## Create Environment File

Create `/opt/mahresources/.env` with your configuration:

```bash
# Database configuration
DB_TYPE=SQLITE
DB_DSN=/opt/mahresources/data/mahresources.db

# File storage
FILE_SAVE_PATH=/opt/mahresources/files

# Server settings
BIND_ADDRESS=127.0.0.1:8181

# Optional: Video thumbnail support
FFMPEG_PATH=/usr/bin/ffmpeg

# Optional: Office document thumbnails
LIBREOFFICE_PATH=/usr/bin/soffice
```

Set appropriate permissions:

```bash
sudo chown mahresources:mahresources /opt/mahresources/.env
sudo chmod 600 /opt/mahresources/.env
```

## Create Systemd Unit File

Create `/etc/systemd/system/mahresources.service`:

```ini
[Unit]
Description=Mahresources Personal Information Manager
After=network.target
# If using PostgreSQL, uncomment the following:
# After=network.target postgresql.service
# Requires=postgresql.service

[Service]
Type=simple
User=mahresources
Group=mahresources
WorkingDirectory=/opt/mahresources

# Load environment variables
EnvironmentFile=/opt/mahresources/.env

# Run the application
ExecStart=/opt/mahresources/bin/mahresources

# Restart on failure
Restart=on-failure
RestartSec=5

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/mahresources/data /opt/mahresources/files

# Resource limits (adjust as needed)
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

## Enable and Start the Service

```bash
# Reload systemd to recognize the new service
sudo systemctl daemon-reload

# Enable the service to start on boot
sudo systemctl enable mahresources

# Start the service
sudo systemctl start mahresources

# Check status
sudo systemctl status mahresources
```

## Managing the Service

```bash
# View logs
sudo journalctl -u mahresources -f

# Restart the service
sudo systemctl restart mahresources

# Stop the service
sudo systemctl stop mahresources

# Disable autostart
sudo systemctl disable mahresources
```

## PostgreSQL Configuration

If using PostgreSQL instead of SQLite, update the environment file:

```bash
DB_TYPE=POSTGRES
DB_DSN=host=localhost user=mahresources password=yourpassword dbname=mahresources sslmode=disable
```

And ensure the PostgreSQL service dependency is enabled in the unit file.

## Updating

To update to a new version:

```bash
# Stop the service
sudo systemctl stop mahresources

# Replace the binary
sudo cp mahresources-new /opt/mahresources/bin/mahresources
sudo chown mahresources:mahresources /opt/mahresources/bin/mahresources

# Start the service
sudo systemctl start mahresources
```

## Troubleshooting

Check logs for errors:

```bash
sudo journalctl -u mahresources --since "1 hour ago"
```

Verify the service is running:

```bash
sudo systemctl status mahresources
curl http://127.0.0.1:8181/
```

If the service fails to start, check file permissions:

```bash
sudo ls -la /opt/mahresources/
sudo namei -l /opt/mahresources/data/mahresources.db
```
