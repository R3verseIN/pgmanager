# WAL-G S3 Backup Setup

pgmanager uses [WAL-G](https://github.com/wal-g/wal-g) for continuous WAL archiving and base backups to S3-compatible storage. This guide covers setup with AWS S3, Cloudflare R2, and MinIO.

## How It Works

1. **WAL Archiving** — PostgreSQL archives WAL segments to S3 via `archive_command` (`wal-g wal-push %p`) every `archive_timeout` seconds (default: 10s in test, 300s in production).
2. **Base Backups** — Full base backups run at `WALG_BACKUP_INTERVAL` (default: 3600s). Configurable via the S3 Backups UI.
3. **Point-in-Time Recovery** — Restore to any point in time by replaying WAL segments from a base backup.
4. **Automatic Cleanup** — After each scheduled backup, expired WAL segments and backups beyond the retention period are automatically cleaned up via `wal-g delete garbage`. You can also trigger cleanup manually via the "Clean Garbage" button.

## Common Configuration

These environment variables go in your `.env` file. All providers use the same variables — only the values change.

| Variable | Description | Required |
|----------|-------------|----------|
| `WALG_S3_PREFIX` | S3 bucket URI (e.g., `s3://my-bucket/pgmanager`) | Yes (to enable) |
| `AWS_ACCESS_KEY_ID` | S3 access key ID | Yes |
| `AWS_SECRET_ACCESS_KEY` | S3 secret access key | Yes |
| `AWS_ENDPOINT` | S3 endpoint URL (see provider-specific values below) | Depends on provider |
| `AWS_REGION` | S3 region (default: `us-east-1`) | No |
| `AWS_S3_FORCE_PATH_STYLE` | Set to `true` for non-AWS providers | Depends on provider |
| `WALG_BACKUP_INTERVAL` | Base backup interval in seconds (default: `3600`) | No |
| `WALG_BACKUP_RETENTION_DAYS` | Days to keep backups (default: `7`) | No |

> **Note:** Sensitive credentials (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`) must be set via environment variables in `.env` — they cannot be configured through the web UI for security reasons. Non-sensitive settings (bucket path, endpoint, region, interval, retention) are stored in the database and configurable via the S3 Backups UI.

## AWS S3

The simplest setup — WAL-G has native S3 support.

### 1. Create an S3 bucket

```bash
aws s3 mb s3://my-pgmanager-backups --region us-east-1
```

Or via the AWS Console: S3 → Create bucket → choose a name and region.

### 2. Create an IAM user

Create an IAM user with this policy:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:PutObject",
        "s3:GetObject",
        "s3:ListBucket",
        "s3:DeleteObject"
      ],
      "Resource": [
        "arn:aws:s3:::my-pgmanager-backups",
        "arn:aws:s3:::my-pgmanager-backups/*"
      ]
    }
  ]
}
```

Create access keys for this user and note the Access Key ID and Secret Access Key.

### 3. Set environment variables

```env
WALG_S3_PREFIX=s3://my-pgmanager-backups
AWS_ACCESS_KEY_ID=AKIA...
AWS_SECRET_ACCESS_KEY=...
AWS_REGION=us-east-1
```

`AWS_ENDPOINT` and `AWS_S3_FORCE_PATH_STYLE` are not needed for native AWS S3.

### 4. Restart and verify

```bash
docker compose up --build -d
```

Open the S3 Backups page in the web UI and trigger a backup to verify.

## Cloudflare R2

R2 is S3-compatible and works with WAL-G. No special WAL-G configuration needed — just the right endpoint and settings.

### 1. Create an R2 bucket

1. Go to the Cloudflare Dashboard → R2 → Create bucket
2. Choose a bucket name (e.g., `pgmanager-backups`)
3. Note your **Account ID** from the R2 overview page (or from your dashboard URL: `dash.cloudflare.com/<ACCOUNT_ID>/r2`)

### 2. Create an API token

1. R2 → Manage R2 API Tokens → Create API token
2. Permissions: **Object Read & Write**
3. Scope: your bucket
4. Copy the Access Key ID and Secret Access Key

### 3. Set environment variables

```env
WALG_S3_PREFIX=s3://pgmanager-backups
AWS_ACCESS_KEY_ID=<your-r2-access-key-id>
AWS_SECRET_ACCESS_KEY=<your-r2-secret-access-key>
AWS_ENDPOINT=https://<ACCOUNT_ID>.r2.cloudflarestorage.com
AWS_REGION=auto
AWS_S3_FORCE_PATH_STYLE=true
```

**Critical settings for R2:**

- `AWS_ENDPOINT` — must be `https://<ACCOUNT_ID>.r2.cloudflarestorage.com` (no trailing slash, no bucket name)
- `AWS_REGION=auto` — required by the AWS SDK even though R2 ignores it
- `AWS_S3_FORCE_PATH_STYLE=true` — R2 does not support virtual-hosted-style URLs

### 4. R2-specific gotchas

| Issue | Details |
|-------|---------|
| No SSE/KMS encryption | R2 does not support `WALG_S3_SSE` with `aws:kms`. SSE-C (customer-provided keys) is the only option. |
| No Object Lock | `WALG_S3_RETENTION_PERIOD` and `WALG_S3_RETENTION_MODE` will not work. |
| Versioning | Set `S3_ENABLE_VERSIONING=disabled` in your environment. R2 does not implement `PutBucketVersioning` via the S3 API. |
| Checksums | R2 only supports one checksum per PUT. If you see checksum errors, WAL-G may need configuration adjustment. |
| ListObjectsV2 | R2 supports it, but if listing fails, set `WALG_S3_USE_LIST_OBJECTS_V1=true` as a fallback. |

### 5. Restart and verify

```bash
docker compose up --build -d
```

Open the S3 Backups page in the web UI and trigger a backup. Verify objects appear in your R2 bucket.

## MinIO (Local Development)

MinIO is a local S3-compatible server. Useful for development and testing without cloud dependencies.

### 1. Run MinIO

The test stack includes MinIO:

```bash
docker compose -f docker-compose.test.yml up -d minio minio-init
```

This starts MinIO on `http://localhost:9001` (console) and creates the `pgmanager-test` bucket automatically.

For standalone MinIO:

```bash
docker run -d -p 9000:9000 -p 9001:9001 \
  -e MINIO_ROOT_USER=minioadmin \
  -e MINIO_ROOT_PASSWORD=minioadmin \
  minio/minio server /data --console-address ":9001"
```

### 2. Create a bucket

```bash
mc alias set local http://localhost:9000 minioadmin minioadmin
mc mb local/pgmanager-backups
```

### 3. Set environment variables

```env
WALG_S3_PREFIX=s3://pgmanager-backups
AWS_ACCESS_KEY_ID=minioadmin
AWS_SECRET_ACCESS_KEY=minioadmin
AWS_ENDPOINT=http://minio:9000
AWS_REGION=us-east-1
AWS_S3_FORCE_PATH_STYLE=true
```

**Note:** `AWS_ENDPOINT` uses the Docker service name (`minio`) when running inside Docker, or `http://localhost:9000` when running on the host.

### 4. Restart and verify

```bash
docker compose up --build -d
```

## Verifying Backups Work

### Manual test

```bash
# Trigger a backup via the web UI (S3 Backups → Trigger Backup)

# Or via the API:
curl -X POST http://localhost:8080/api/walg/backup \
  -H "Cookie: session_id=<your-session>"
```

### Check objects in your bucket

**AWS S3:**
```bash
aws s3 ls s3://my-pgmanager-backups/
```

**R2 (using rclone or s3cmd):**
```bash
rclone ls remote:pgmanager-backups/
```

**MinIO (using mc):**
```bash
mc ls local/pgmanager-backups/
```

### Verify restore

1. Open S3 Backups in the web UI
2. Click "Verify" on a backup — should show "OK"
3. Click "Restore" — select a target database
4. Connect to the target database and verify data

## Troubleshooting

### Backup fails with "connection refused"

WAL-G connects to PostgreSQL to start/end the backup. Ensure `PGPASSWORD` is set (the Go app handles this automatically in normal operation).

### Backup fails with "bucket not found"

Check `WALG_S3_PREFIX` — the bucket must exist before WAL-G can write to it.

### "Access Denied" on R2

- Verify the API token has **Object Read & Write** permissions
- Verify the token is scoped to the correct bucket
- Check that `AWS_S3_FORCE_PATH_STYLE=true` is set

### Archive command fails in PostgreSQL logs

Check `docker compose logs db` for WAL-G errors. Common issues:
- Missing `AWS_ENDPOINT` for non-AWS providers
- Wrong `WALG_S3_PREFIX` format (must be `s3://bucket-name`, not `http://host/bucket`)
- Expired or invalid credentials
