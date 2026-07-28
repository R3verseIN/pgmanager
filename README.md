<p align="center">
  <img src="docs/logo.png" alt="pgmanager logo" width="200">
</p>

<h1 align="center">pgmanager</h1>

<p align="center">
  Self-hosted PostgreSQL management panel with built-in PgBouncer connection pooling.
</p>

<p align="center">
  <a href="#quick-start">Quick Start</a> ·
  <a href="#features">Features</a> ·
  <a href="#architecture">Architecture</a> ·
  <a href="#configuration">Configuration</a> ·
  <a href="#admin-cli">Admin CLI</a> ·
  <a href="#development">Development</a> ·
  <a href="LICENSE">License</a>
</p>

---

## Why pgmanager?

**Security first.** PostgreSQL should never be exposed directly to the internet. pgmanager puts PgBouncer in front as a controlled gateway — handling connection pooling, HBA-based auth, and IP allowlisting. No open ports, no attack surface.

**Low footprint.** The Go backend compiles to a single static binary with the React+Vite UI embedded via `//go:embed`. No nginx, no separate Node process — one container, one process, ~10MB RAM.

**Self-contained.** Three Docker containers. No external services, no cloud dependencies. Clone, configure, run.

## Features

- **Database Management** — Create and delete databases through the web UI
- **Table Operations** — Create tables, add/drop columns, manage schema
- **Data Browser** — View, insert, update, delete rows with pagination and sorting
- **SQL Console** — Run queries with built-in safety guards (dangerous statements rejected, 10s timeout)
- **User Management** — Create PostgreSQL users with granular access levels (read, write, ddl, full) and IP allowlisting
- **Auth Users** — Manage web app login accounts with admin/dev/viewer roles
- **Backup & Restore** — Stream backups via `pg_dump`, restore with `pg_restore`, inspect dump contents
- **S3 Backups (pgBackRest)** — Continuous WAL archiving to S3-compatible storage, automated base (full/incremental) backups, single-database restores, and automatic garbage cleanup
- **PgBouncer Control** — Toggle database access, configure pool mode/size, live reload
- **Audit Logging** — Every action logged with user, IP, timestamp, and detail
- **Admin CLI** — Recovery tool for user management when the web UI is inaccessible

## Quick Start

### Prerequisites

- Docker and Docker Compose

### 1. Clone and configure

```bash
git clone https://github.com/R3verseIN/pgmanager.git
cd pgmanager
cp .env.example .env
```

Edit `.env` and set strong passwords:

```env
POSTGRES_PASSWORD=your-secure-password-here
PGBOUNCER_AUTH_PASSWORD=your-secure-password-here
```

> **Password constraints:** 8-72 characters. Only alphanumeric characters, `_`, and `-` are allowed. Special characters like `!@#$%^&*` are **not supported** and will cause init failures.

### 2. Build and start

```bash
docker compose up --build -d
```

The `--build` flag ensures the latest code is compiled on first run (or after updates).

### 3. Open

Navigate to **http://localhost:8080**. The setup wizard will guide you through creating your first admin account.

**That's it.** The init scripts handle all PostgreSQL configuration automatically on startup.

## Architecture

pgmanager runs three containers in an isolated Docker network:

- **app** (`:8080`) — Go backend with embedded React UI. The admin panel.
- **db** (`:5432` internal) — PostgreSQL 17. Never exposed to the host.
- **pgbouncer** (`:5432` external) — Connection pooler and security gateway.

**Internal path** — Admins log into the web panel at `:8080` to manage everything.

**External path** — App code connects via PgBouncer at `:5432` using credentials created through the admin panel. Similar to how Supabase gives you a connection string.

See [docs/architecture.md](docs/architecture.md) for the full breakdown.

## Configuration

| Variable | Description | Required |
|----------|-------------|----------|
| `POSTGRES_PASSWORD` | PostgreSQL superuser password | Yes |
| `PGBOUNCER_AUTH_PASSWORD` | PgBouncer auth proxy password | Yes |
| `PGMANAGER_HOST` | Hostname/IP for user connection strings (e.g., `pg.example.com:5432`). Auto-detected from the web UI's Host header if not set. | No |
| `PGBACKREST_REPO1_TYPE` | Type of repo (must be `s3` for S3 backups) | No |
| `PGBACKREST_REPO1_S3_BUCKET` | S3 bucket name | If using S3 |
| `PGBACKREST_REPO1_S3_ENDPOINT` | S3 endpoint URL (e.g., s3.us-east-1.amazonaws.com) | If using S3 |
| `PGBACKREST_REPO1_S3_REGION` | S3 region (default: `us-east-1`) | If using S3 |
| `PGBACKREST_REPO1_S3_URI_STYLE` | Set to `path` for MinIO/custom, `host` for AWS | If using S3 |
| `PGBACKREST_REPO1_S3_KEY` | S3 access key ID | If using S3 |
| `PGBACKREST_REPO1_S3_KEY_SECRET` | S3 secret access key | If using S3 |
| `PGBACKREST_REPO1_PATH` | Path inside bucket (e.g., `/backups`) | No |

**Password rules:**
- 8-72 characters
- Only `a-z`, `A-Z`, `0-9`, `_`, `-`
- No special characters (`!@#$%^&*()` etc.)

**S3 Backups:** Configure the `PGBACKREST_REPO1_*` environment variables in your `.env` file to enable automated S3 backups. All other pgBackRest settings (retention, backup schedule, timeouts) are managed via the Web UI.

### Exposed Ports

| Port | Service | Purpose |
|------|---------|---------|
| `8080` | App | Web UI and API |
| `5432` | PgBouncer | External client connections (PostgreSQL protocol) |

> **Note:** The internal PostgreSQL port is not exposed to the host. All external connections must go through PgBouncer on port `5432`.

### Connecting Other Docker Compose Projects

Other Docker Compose projects on the same machine can connect to pgmanager's database via the shared `pgmanager` Docker network.

**Other project's `docker-compose.yml`:**

```yaml
services:
  myapp:
    environment:
      DATABASE_URL: postgres://myuser:mypass@pgbouncer:5432/mydb
    networks:
      - pgmanager

networks:
  pgmanager:
    external: true
```

Use `pgbouncer` as the hostname — Docker resolves it automatically. Credentials are managed through the pgmanager admin panel.

## User Roles

| Role | Permissions |
|------|-------------|
| **admin** | Full access — manage users, databases, settings, backups |
| **dev** | Query assigned databases (reads and writes) |
| **viewer** | Read-only access to all databases |

## First-Time Setup

1. Open `http://localhost:8080`
2. The setup wizard appears automatically on fresh installs
3. Create your admin account (username + password)
4. You're in — create databases, add users, start working

The setup wizard is **one-time-only**. If you get locked out, use the [Admin CLI](#admin-cli) to recover.

## Admin CLI

Interactive recovery tool for user management when the web UI is inaccessible.

```bash
docker compose exec -T db python3 /scripts/admin.py
```

Commands: list users, create user, delete user, reset password, change role.

See [docs/admin-cli.md](docs/admin-cli.md) for full documentation and recovery scenarios.

## Security

- **No direct DB exposure** — PostgreSQL is not accessible from outside the Docker network
- **PgBouncer as gateway** — HBA-based auth, per-user IP allowlisting, transaction pooling
- **SQL Console guards** — Dangerous statements (`DROP DATABASE`, `ALTER ROLE`, `GRANT`, `REVOKE`, `TRUNCATE`) rejected in the SQL Console to prevent accidental damage
- **Statement timeout** — 10-second limit on SQL Console queries
- **Password hashing** — bcrypt
- **Session security** — HttpOnly, SameSite=Strict cookies, 24-hour expiry
- **Protected databases** — `template0`, `template1`, `postgres`, `pgmanager` cannot be created or deleted through the UI
- **Audit trail** — All state-changing actions logged with user, IP, and timestamp



## Development

### Tech Stack

- **Backend:** Go 1.26, pgx/v5, `//go:embed` for UI assets
- **Frontend:** React 18, TypeScript, Vite, Tailwind CSS v4, Radix UI
- **Database:** PostgreSQL 17, PgBouncer (transaction pooling)

### Build locally

```bash
# Frontend
cd backend/ui
bun install
bun run build

# Backend (embeds UI from ui/dist)
cd backend
go build -o pgmanager .
```

### Run tests

```bash
# Backend tests
cd backend
go test ./...

```

### Full Docker build

```bash
docker compose build
docker compose up -d
```

## License

[MIT](LICENSE) — Copyright (c) 2026 r3versein
