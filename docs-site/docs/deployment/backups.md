---
sidebar_position: 4
title: Backups
description: Backing up and restoring Mahresources data
---

# Backup and Restore

Regular backups are essential to protect your data. Mahresources stores data in two locations that must both be backed up.

## What to Back Up

### 1. Database

The database contains all metadata: notes, groups, tags, resource metadata, and relationships.

- **SQLite**: Single `.db` file (e.g., `/opt/mahresources/data/mahresources.db`)
- **PostgreSQL**: The entire database

### 2. Files

The file storage directory contains all uploaded resources (images, documents, videos, etc.).

- Default location: configured via `FILE_SAVE_PATH`
- Includes thumbnails and the original files

:::warning
Both the database AND files must be backed up together. Restoring only one will result in orphaned records or missing files.
:::

## SQLite Backup

### Simple File Copy (When Stopped)

The safest method is to stop the service first:

```bash
# Stop the service
sudo systemctl stop mahresources

# Copy the database
cp /opt/mahresources/data/mahresources.db /backup/mahresources-$(date +%Y%m%d).db

# Start the service
sudo systemctl start mahresources
```

### Online Backup with SQLite CLI

For backups without downtime, use SQLite's backup command:

```bash
sqlite3 /opt/mahresources/data/mahresources.db ".backup '/backup/mahresources-$(date +%Y%m%d).db'"
```

### Automated Backup Script

Create `/opt/mahresources/backup.sh`:

```bash
#!/bin/bash
set -e

BACKUP_DIR="/backup/mahresources"
DATE=$(date +%Y%m%d_%H%M%S)
DB_PATH="/opt/mahresources/data/mahresources.db"
FILES_PATH="/opt/mahresources/files"
RETENTION_DAYS=30

# Create backup directory
mkdir -p "$BACKUP_DIR"

# Backup database (online backup)
sqlite3 "$DB_PATH" ".backup '$BACKUP_DIR/db-$DATE.db'"

# Compress the database backup
gzip "$BACKUP_DIR/db-$DATE.db"

# Backup files (incremental with rsync)
rsync -a --delete "$FILES_PATH/" "$BACKUP_DIR/files/"

# Remove old database backups
find "$BACKUP_DIR" -name "db-*.db.gz" -mtime +$RETENTION_DAYS -delete

echo "Backup completed: $DATE"
```

Add to crontab:

```bash
# Run daily at 3 AM
0 3 * * * /opt/mahresources/backup.sh >> /var/log/mahresources-backup.log 2>&1
```

## PostgreSQL Backup

### pg_dump

```bash
pg_dump -U mahresources -h localhost mahresources > /backup/mahresources-$(date +%Y%m%d).sql
```

### Compressed Backup

```bash
pg_dump -U mahresources -h localhost mahresources | gzip > /backup/mahresources-$(date +%Y%m%d).sql.gz
```

### Custom Format (Recommended)

Custom format allows parallel restore and selective table restoration:

```bash
pg_dump -U mahresources -h localhost -Fc mahresources > /backup/mahresources-$(date +%Y%m%d).dump
```

## File Backup with rsync

### Local Backup

```bash
rsync -av --delete /opt/mahresources/files/ /backup/mahresources-files/
```

### Remote Backup

```bash
rsync -avz --delete /opt/mahresources/files/ user@backup-server:/backup/mahresources-files/
```

### Exclude Thumbnails (Optional)

Thumbnails can be regenerated, so you may exclude them to save space:

```bash
rsync -av --delete --exclude='thumbs/' /opt/mahresources/files/ /backup/mahresources-files/
```

## Cloud Backup with rclone

[rclone](https://rclone.org/) supports many cloud storage providers.

### Configure rclone

```bash
rclone config
# Follow prompts to set up your cloud storage
```

### Backup to Cloud

```bash
# Backup database
rclone copy /backup/mahresources-$(date +%Y%m%d).db.gz remote:mahresources-backups/db/

# Sync files
rclone sync /opt/mahresources/files/ remote:mahresources-backups/files/
```

### Automated Cloud Backup Script

```bash
#!/bin/bash
set -e

DATE=$(date +%Y%m%d)
DB_PATH="/opt/mahresources/data/mahresources.db"
FILES_PATH="/opt/mahresources/files"
REMOTE="remote:mahresources-backups"

# Online database backup
sqlite3 "$DB_PATH" ".backup '/tmp/mahresources-$DATE.db'"
gzip "/tmp/mahresources-$DATE.db"

# Upload database backup
rclone copy "/tmp/mahresources-$DATE.db.gz" "$REMOTE/db/"

# Sync files
rclone sync "$FILES_PATH" "$REMOTE/files/" --progress

# Cleanup
rm "/tmp/mahresources-$DATE.db.gz"

echo "Cloud backup completed: $DATE"
```

## Restore Procedure

### 1. Stop the Service

```bash
sudo systemctl stop mahresources
```

### 2. Restore Database

**SQLite:**

```bash
# Decompress if needed
gunzip /backup/mahresources-20240115.db.gz

# Replace database
cp /backup/mahresources-20240115.db /opt/mahresources/data/mahresources.db
chown mahresources:mahresources /opt/mahresources/data/mahresources.db
```

**PostgreSQL:**

```bash
# Drop and recreate database
psql -U postgres -c "DROP DATABASE mahresources;"
psql -U postgres -c "CREATE DATABASE mahresources OWNER mahresources;"

# Restore from SQL dump
psql -U mahresources -d mahresources < /backup/mahresources-20240115.sql

# Or from custom format
pg_restore -U mahresources -d mahresources /backup/mahresources-20240115.dump
```

### 3. Restore Files

```bash
rsync -av --delete /backup/mahresources-files/ /opt/mahresources/files/
chown -R mahresources:mahresources /opt/mahresources/files/
```

### 4. Start the Service

```bash
sudo systemctl start mahresources
```

### 5. Verify

- Check the logs: `sudo journalctl -u mahresources -f`
- Access the web interface and verify data is present
- Test file access by viewing some resources

## Backup Verification

Regularly test your backups by restoring to a test environment:

```bash
# Create test instance with ephemeral mode using backup
./mahresources \
  -memory-db \
  -seed-db=/backup/mahresources-latest.db \
  -seed-fs=/backup/mahresources-files \
  -bind-address=:8182
```

This starts a read-only test instance without affecting your production data.

## Backup Checklist

- [ ] Database is backed up regularly (daily recommended)
- [ ] Files are backed up or synced
- [ ] Backups are stored off-site (different physical location)
- [ ] Backup retention policy is in place
- [ ] Restore procedure has been tested
- [ ] Backup monitoring/alerting is configured
