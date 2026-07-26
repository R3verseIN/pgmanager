# Architecture

pgmanager is a self-hosted PostgreSQL management platform with three core components: a Go backend with an embedded React UI, a PostgreSQL 17 database, and PgBouncer as a connection pooler and security gateway.

## Design Philosophy

### Security First

PostgreSQL should never be exposed directly to the internet. Every open port is an attack surface. pgmanager enforces this by:

- Never exposing the PostgreSQL port — the `db` container's port 5432 is only reachable within the Docker network
- PgBouncer as the sole gateway — all external connections go through PgBouncer, which enforces HBA-based authentication and per-user IP allowlisting
- Protected system databases — `template0`, `template1`, `postgres`, and `pgmanager` cannot be modified through the UI
- SQL Console guards — dangerous statements like `DROP DATABASE`, `ALTER ROLE`, `GRANT`, `REVOKE`, `TRUNCATE` are rejected in the SQL Console to prevent accidental damage

Direct database exposure creates two risks: security (brute force, SQL injection, unauthorized access) and scalability (PostgreSQL allocates shared memory per connection; uncontrolled connections exhaust resources). PgBouncer solves both by multiplexing many client connections through a limited pool of server connections.

> **Password constraint:** Only alphanumeric characters, `_`, and `-` are allowed. Special characters are not supported due to how passwords are passed to PostgreSQL tools (`psql`, `pg_dump`, `pg_restore`) via environment variables.

### Low Memory Footprint

The Go backend compiles to a single static binary. The React+Vite frontend is embedded directly into the binary via `//go:embed`. No nginx or reverse proxy needed, no separate Node.js process for serving the UI, ~7MB RAM usage for the entire backend. One container, one process, minimal attack surface.

### Self-Contained

Everything runs in three Docker containers: `db` (PostgreSQL 17), `app` (Go binary with embedded React UI), and `pgbouncer` (connection pooling gateway). No external dependencies. Data persists via Docker volumes. Passwords are stored as files in the shared volume, not in environment variables.

## System Overview

pgmanager runs three containers in an isolated Docker network. The `app` container exposes port 8080 (web UI) and the `pgbouncer` container exposes port 5432 (client connections). PostgreSQL on the `db` container is never exposed to the host — it's only reachable within the Docker network by the other two containers.

## Two Access Paths

pgmanager serves two distinct use cases through two separate network paths:

### 1. Internal — Admin Panel

```
Browser ──▶ App :8080 ──▶ DB :5432
```

Administrators, developers, and viewers log into the web UI to manage databases and tables, browse and edit data, run SQL queries, create and manage PostgreSQL users, perform backups and restores, configure PgBouncer, and view audit logs. Authentication is session-based (cookie). The Go backend translates UI actions into PostgreSQL operations.

### 2. External — Application Clients

```
App Code ──▶ PgBouncer :5432 ──▶ DB :5432
```

Application code connects to PostgreSQL using credentials created through the admin panel. This is similar to how Supabase provides connection strings:

```
postgresql://app_user:password@your-host:5432/mydb
```

**Connection string host resolution:** When a user is created, the connection string's host is determined automatically:
- If `PGMANAGER_HOST` is set, it is used as-is (e.g., `pg.example.com:5432`). Include the port if non-default.
- If `PGMANAGER_HOST` is not set, the host is auto-detected from the web UI's `Host` header (e.g., accessing via `http://192.168.0.13:8080` yields host `192.168.0.13:5432`).

This means connection strings adapt to how you access the web UI — no hardcoded IPs needed when accessing from the same network.

PgBouncer handles authentication (verifies credentials against PostgreSQL roles), connection pooling (multiplexes 100+ client connections through 20 server connections), IP allowlisting (per-user firewall rules in `pg_hba.conf`), and transaction pooling (releases server connections between transactions for efficiency).

## Authentication Model

### Web UI

Sessions are managed via `session_id` cookies with HttpOnly, SameSite=Strict, and 24-hour expiry. Passwords are hashed with bcrypt. The setup wizard is one-time-only — after the first admin is created, it never appears again. If `auth_users` is dropped while the setup flag exists, you're locked out and must use the Admin CLI.

### External Clients (PgBouncer)

PgBouncer uses HBA-based authentication: the client connects with username/password, PgBouncer queries the `pgbouncer_get_user()` function to look up the role, HBA rules are checked (per-user IP allowlist), and if allowed, the connection is authenticated against PostgreSQL's password hash. HBA files are regenerated on every user change and PgBouncer is reloaded automatically.

The internal PgBouncer HBA rules follow this order:
1. `pgbouncer_auth` — trust (PgBouncer connects as this user to look up password hashes via `auth_query`; SELECT-only on `pgbouncer_get_user()`)
2. `pgmanager` — scram-sha-256 (app superuser; requires password auth)
3. Per-user rules — scram-sha-256 with IPs from the `allowed_ips` JSONB column
4. Catch-all reject — blocks any unmatched connection

## Authorization

### Web UI Roles

- **admin** — Full access. All routes, all databases, user management, PgBouncer config.
- **dev** — Assigned databases only. Can read/write data and run queries, but scoped to specific databases via the `dev_databases` table.
- **viewer** — All databases, but read-only. No write operations.

### PostgreSQL User Access Levels

When creating a PostgreSQL user for external connections, admins assign an access level:

- **read** — `SELECT` on all tables
- **write** — `SELECT`, `INSERT`, `UPDATE`, `DELETE`
- **ddl** — Write + `CREATE` on schema
- **full** — All privileges on database and schema

Write access is enforced at the API handler level — only `admin` and `dev` roles can perform writes.

## PgBouncer Integration

### Connection Pooling

PgBouncer runs in transaction pooling mode. Clients connect on port 5432 (mapped from host), PgBouncer maintains a pool of server connections to PostgreSQL (default: 20 server connections, 100 max client connections), and server connections are released after each transaction via `DISCARD ALL`.

### Dynamic HBA File Generation

The Go backend generates PgBouncer's `pg_hba.conf` file dynamically via `RebuildPgBouncerHBA()`. It queries the `managed_users` table for all PostgreSQL users and their `allowed_ips`, builds HBA rules (`host all "username" <ip>/32 scram-sha-256`), writes to `/etc/pgbouncer/shared/pg_hba.conf` via a shared volume, and issues `RELOAD` to PgBouncer via the admin console.

The generated rules follow a strict order:
1. `pgbouncer_auth` trust rules (localhost + Docker CIDRs) — PgBouncer connects as this user to look up password hashes
2. `pgmanager` scram-sha-256 rules (Docker CIDRs) — app superuser auth
3. Per-user scram-sha-256 rules from `managed_users` — each user gets rules for their `allowed_ips`
4. Catch-all reject — blocks any connection not matched above

This runs on startup, every 5 minutes (periodic rebuild), after any user create/update/delete, and after any database create/delete.

### Dynamic Database Configuration

PgBouncer's `[databases]` section is also rebuilt dynamically from the `pgbouncer_databases` table. Allowed databases get entries like `dbname = host=db port=5432 dbname=dbname`, written to `pgbouncer.ini` via shared volume, then PgBouncer is reloaded.

## Database Schema

Seven tables power the system:

- **auth_users** — Web UI login accounts (username, password_hash, role)
- **sessions** — Active sessions (token, user_id, expires_at)
- **managed_users** — PostgreSQL users for external access (username, database, access level, allowed_ips)
- **dev_databases** — Database assignments for dev role users
- **audit_log** — All actions logged (username, action, database, detail, ip, timestamp)
- **system_config** — Key-value config (setup flag, PgBouncer settings)
- **pgbouncer_databases** — Database allowlist for PgBouncer access

Relationships: `auth_users` has many `sessions` and `dev_databases` (cascade delete). `managed_users` uses a composite key on `(username, database_name)`.

## Init Scripts

The startup flow runs through `pg-entrypoint.sh` in the `db` container. On first start, it delegates to Docker's standard `docker-entrypoint.sh` for `initdb`. On subsequent starts, it boots PostgreSQL temporarily, runs `pgmanager-init.py`, then stops the temporary instance and starts PostgreSQL normally.

The init script validates environment variables, writes password files to the shared volume, ensures the PostgreSQL user exists with the correct password, ensures the database exists and is owned correctly, creates the `pgbouncer_auth` user and `pgbouncer_get_user()` function, configures HBA rules (scram-sha-256 for pgmanager, trust for pgbouncer_auth, reject external), and revokes CONNECT on system databases from PUBLIC.

This runs on **every startup**, not just the first. Password files stay in sync with environment variables, the `pgbouncer_auth` function stays up to date, HBA rules are restored if tampered with, and system database protections are always enforced.

PostgreSQL's `pg_hba.conf` is managed by the init script on first start. It replaces the default PG17 catch-all `host all all all scram-sha-256` line with scoped per-user rules and adds a marker comment (`# pgmanager-init: managed`) to skip replacement on subsequent runs. PgBouncer's HBA file is managed separately by the Go app via `RebuildPgBouncerHBA()`.

## WAL-G Backup Architecture

pgmanager integrates [WAL-G](https://github.com/wal-g/wal-g) for continuous WAL archiving and base backups to S3-compatible storage. This provides point-in-time recovery (PITR) capability alongside the existing pg_dump backup system.

### How It Works

1. **WAL Archiving** — PostgreSQL continuously archives WAL segments to S3 via `archive_command` (`wal-g wal-push %p`), running every `archive_timeout` seconds (default: 60s).
2. **Base Backups** — WAL-G creates full base backups at the configured interval (`WALG_BACKUP_INTERVAL`, default: 3600s). Backups are stored in S3 with metadata (backup name, time, WAL segment).
3. **Point-in-Time Recovery** — Restore to any point in time by replaying WAL segments from the base backup. The `restore_command` (`wal-g wal-fetch %f %p`) retrieves WAL files from S3 during recovery.
4. **Garbage Collection** — Old backups and WAL segments are cleaned up based on `WALG_BACKUP_RETENTION_DAYS` (default: 7 days).
5. **Scheduled Backups** — Base backups run automatically at the configured interval. After each scheduled backup, garbage cleanup runs automatically to remove expired WAL segments and backups beyond retention.
6. **Restore** — Fetches backup from S3 via `backup-fetch`, starts a temporary PostgreSQL instance from the fetched PGDATA with `restore_command = 'wal-g wal-fetch %f %p'` to replay WAL from S3, waits for promotion, runs `pg_dump` on the backup data, then `pg_restore` into the target database on the live server. The temp instance uses process group isolation (`Setpgid`) for clean shutdown — `SIGINT` to the entire process group, with `SIGKILL` fallback after 10s, and orphan reaping via `Wait4`.

### Container Layout

- **db container** — WAL-G binary installed. Runs `archive_command` (local push to S3). PostgreSQL configured with `wal_level=replica`, `archive_mode=on`.
- **app container** — WAL-G binary installed, plus `postgresql17` server, `su-exec`, and `gcompat` (WAL-G glibc compat on Alpine). Remote operations via BASE_BACKUP protocol (list, trigger, restore, delete, verify, test-connection). The Go backend manages WAL-G configuration and backup lifecycle through the S3 Backups UI. On restore, a temporary PostgreSQL instance is started from the fetched backup data.

### S3 Configuration

All WAL-G settings come from environment variables only — there is no database-stored config. Set these in `docker-compose.yml` or `.env`:

| Variable | Description |
|----------|-------------|
| `WALG_S3_PREFIX` | S3 URI, e.g. `s3://my-bucket` (required to enable WAL-G) |
| `AWS_ACCESS_KEY_ID` | S3 access key |
| `AWS_SECRET_ACCESS_KEY` | S3 secret key |
| `AWS_ENDPOINT` | Custom S3 endpoint (MinIO, R2, etc.) |
| `AWS_REGION` | S3 region |
| `AWS_S3_FORCE_PATH_STYLE` | `true` for MinIO and most S3-compatible providers |
| `WALG_BACKUP_INTERVAL` | Seconds between base backups (default: 3600) |
| `WALG_BACKUP_RETENTION_DAYS` | Days to keep backups (default: 7) |

### Supported Providers

All providers use the same environment variables — only the values change.

- **AWS S3** — Default. No `AWS_ENDPOINT` needed. Region should match your bucket.
- **Cloudflare R2** — Set `AWS_ENDPOINT=https://<ACCOUNT_ID>.r2.cloudflarestorage.com`, `AWS_REGION=auto`, `AWS_S3_FORCE_PATH_STYLE=true`. R2 does not support S3 SSE/KMS or Object Lock. See [setup guide](walg-s3-setup.md#cloudflare-r2) for details.
- **MinIO** — Set `AWS_ENDPOINT=http://minio:9000` and `AWS_S3_FORCE_PATH_STYLE=true`. Useful for local development.
- **SeaweedFS** — Same as MinIO configuration.
- **Any S3-compatible storage** — Set the endpoint and path style as needed.

See [docs/walg-s3-setup.md](walg-s3-setup.md) for provider-specific setup instructions, bucket creation, and troubleshooting.

### S3 Backups UI

The S3 Backups page (`/databases/backups`) provides a full management interface:

- **Status banner** — Shows archiving state, backup count, total storage size, last backup time, configured interval, and retention period
- **Test Connection** — Validates S3 connectivity by listing backups from the configured bucket
- **Trigger Backup** — Manually creates a base backup on demand
- **Backup list** — Shows all backups with name, timestamp, WAL segment, size, and status
- **Verify Integrity** — Runs `wal-verify` to check WAL segment continuity
- **Restore** — Opens a dialog to select a target database, fetches the backup, starts a temp PostgreSQL instance, runs `pg_dump`/`pg_restore`, and cleans up
- **Delete** — Removes a specific backup from S3
- **Clean Garbage** — Runs `wal-g delete garbage` to clean expired WAL segments and old backups
- **Configuration** — Edit interval, retention days, S3 prefix, endpoint, region, and path style (credentials are env vars only)

### WAL-G Setup Tool

A standalone CLI tool (`scripts/walg-setup/`) helps users configure WAL-G with any S3-compatible provider. It walks through provider selection (AWS S3, Cloudflare R2, DigitalOcean Spaces, Wasabi, Backblaze B2, Google Cloud Storage, Alibaba OSS, Scaleway, MinIO, Ceph, or custom S3-compatible), validates bucket connectivity, and outputs the environment variables needed for `docker-compose.yml`. The tool is built separately and not included in the Docker image.
