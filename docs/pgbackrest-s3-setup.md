# pgBackRest S3 Backup Setup

pgmanager uses [pgBackRest](https://pgbackrest.org/) for continuous WAL archiving and base backups to S3-compatible storage. This guide covers setup with AWS S3, Cloudflare R2, and MinIO.

## How It Works

1. **WAL Archiving** — PostgreSQL archives WAL segments to S3 via `archive_command` (`pgbackrest --stanza=pgmanager archive-push %p`) every `archive_timeout` seconds (configured via UI).
2. **Base Backups** — Full and Incremental base backups run automatically via cron. Configurable via the S3 Backups UI.
3. **Point-in-Time Recovery** — Restore to any point in time by replaying WAL segments from a base backup (managed by the backend when restoring databases).
4. **Automatic Cleanup** — Based on the `Retention Days` setting in the UI, pgBackRest automatically cleans up expired backups and WAL segments during the next full or incremental backup.

## Common Configuration

These environment variables go in your `.env` file. All providers use the same variables — only the values change.

| Variable | Description | Required |
|----------|-------------|----------|
| `PGBACKREST_REPO1_TYPE` | Type of repo (must be `s3` for S3 backups) | Yes (to enable) |
| `PGBACKREST_REPO1_S3_BUCKET` | S3 bucket name | Yes |
| `PGBACKREST_REPO1_S3_ENDPOINT` | S3 endpoint URL (see provider-specific values below) | Depends on provider |
| `PGBACKREST_REPO1_S3_REGION` | S3 region (default: `us-east-1`) | Yes |
| `PGBACKREST_REPO1_S3_URI_STYLE` | Set to `path` for MinIO/custom, `host` for AWS | Yes |
| `PGBACKREST_REPO1_S3_KEY` | S3 access key ID | Yes |
| `PGBACKREST_REPO1_S3_KEY_SECRET` | S3 secret access key | Yes |
| `PGBACKREST_REPO1_PATH` | Path inside bucket (e.g., `/backups`) | No |

> **Note:** Sensitive credentials must be set via environment variables in `.env` — they cannot be configured through the web UI for security reasons. Non-sensitive settings (retention, backup schedule, timeouts) are stored in the database and configurable via the S3 Backups UI.

## AWS S3

The simplest setup — pgBackRest has native S3 support.

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
PGBACKREST_REPO1_TYPE=s3
PGBACKREST_REPO1_S3_BUCKET=my-pgmanager-backups
PGBACKREST_REPO1_S3_REGION=us-east-1
PGBACKREST_REPO1_S3_URI_STYLE=host
PGBACKREST_REPO1_S3_KEY=AKIA...
PGBACKREST_REPO1_S3_KEY_SECRET=...
```

### 4. Restart and verify

```bash
docker compose up --build -d
```

Open the S3 Backups page in the web UI and trigger a backup to verify.

## Cloudflare R2

R2 is S3-compatible and works flawlessly with pgBackRest.

### 1. Create an R2 bucket

1. Go to the Cloudflare Dashboard → R2 → Create bucket
2. Choose a bucket name (e.g., `pgmanager-backups`)
3. Note your **Account ID** from the R2 overview page

### 2. Create an API token

1. R2 → Manage R2 API Tokens → Create API token
2. Permissions: **Object Read & Write**
3. Scope: your bucket
4. Copy the Access Key ID and Secret Access Key

### 3. Set environment variables

```env
PGBACKREST_REPO1_TYPE=s3
PGBACKREST_REPO1_S3_BUCKET=pgmanager-backups
PGBACKREST_REPO1_S3_ENDPOINT=<ACCOUNT_ID>.r2.cloudflarestorage.com
PGBACKREST_REPO1_S3_REGION=auto
PGBACKREST_REPO1_S3_URI_STYLE=host
PGBACKREST_REPO1_S3_KEY=<your-r2-access-key-id>
PGBACKREST_REPO1_S3_KEY_SECRET=<your-r2-secret-access-key>
```

**Critical settings for R2:**

- `PGBACKREST_REPO1_S3_ENDPOINT` — must be `<ACCOUNT_ID>.r2.cloudflarestorage.com` (no `https://` prefix)
- `PGBACKREST_REPO1_S3_REGION=auto` — required even though R2 ignores it

## MinIO (Local Development)

MinIO is a local S3-compatible server. Useful for development and testing without cloud dependencies.

### 1. Run MinIO

The test stack includes MinIO:

```bash
docker compose -f docker-compose.test.yml up -d minio minio-init
```

This starts MinIO on `http://localhost:9001` (console) and creates the `pgmanager-test` bucket automatically.

### 2. Create a bucket

```bash
mc alias set local http://localhost:9000 minioadmin minioadmin
mc mb local/pgmanager-backups
```

### 3. Set environment variables

```env
PGBACKREST_REPO1_TYPE=s3
PGBACKREST_REPO1_S3_BUCKET=pgmanager-backups
PGBACKREST_REPO1_S3_ENDPOINT=minio:9000
PGBACKREST_REPO1_S3_REGION=us-east-1
PGBACKREST_REPO1_S3_URI_STYLE=path
PGBACKREST_REPO1_S3_KEY=minioadmin
PGBACKREST_REPO1_S3_KEY_SECRET=minioadmin
PGBACKREST_REPO1_STORAGE_VERIFY_TLS=n
```

**Note:** `PGBACKREST_REPO1_S3_ENDPOINT` uses the Docker service name (`minio`) when running inside Docker. Also, we set `PGBACKREST_REPO1_STORAGE_VERIFY_TLS=n` for local self-signed MinIO instances.

## Verifying Backups Work

### Manual test

Trigger a full backup via the web UI (S3 Backups → Run Full Backup).

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
2. Click "Restore" next to an existing backup
3. Select a target database from the dropdown and confirm
4. Connect to the target database and verify data has been restored
