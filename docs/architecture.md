# Architecture

pgmanager is a self-hosted PostgreSQL management platform with three core components: a Go backend with an embedded React UI, a PostgreSQL 17 database, and PgBouncer as a connection pooler and security gateway.

## Design Philosophy

### Security First

PostgreSQL should never be exposed directly to the internet. Every open port is an attack surface. pgmanager enforces this by:

- Never exposing the PostgreSQL port — the `db` container's port 5432 is only reachable within the Docker network
- PgBouncer as the sole gateway — all external connections go through PgBouncer, which enforces HBA-based authentication and per-user IP allowlisting
- Protected system databases — `template0`, `template1`, `postgres`, and `pgmanager` cannot be modified through the UI
- Blocked SQL patterns — dangerous statements like `DROP DATABASE`, `ALTER ROLE`, `GRANT`, `REVOKE`, `TRUNCATE` are blocked in the SQL console

Direct database exposure creates two risks: security (brute force, SQL injection, unauthorized access) and scalability (PostgreSQL allocates shared memory per connection; uncontrolled connections exhaust resources). PgBouncer solves both by multiplexing many client connections through a limited pool of server connections.

> **Password constraint:** Only alphanumeric characters, `_`, and `-` are allowed. Special characters are not supported due to how passwords are passed to PostgreSQL tools (`psql`, `pg_dump`, `pg_restore`) via environment variables.

### Low Memory Footprint

The Go backend compiles to a single static binary. The React+Vite frontend is embedded directly into the binary via `//go:embed`. No nginx or reverse proxy needed, no separate Node.js process for serving the UI, ~10MB RAM usage for the entire backend. One container, one process, minimal attack surface.

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

PgBouncer handles authentication (verifies credentials against PostgreSQL roles), connection pooling (multiplexes 100+ client connections through 20 server connections), IP allowlisting (per-user firewall rules in `pg_hba.conf`), and transaction pooling (releases server connections between transactions for efficiency).

## Authentication Model

### Web UI

Sessions are managed via `session_id` cookies with HttpOnly, SameSite=Strict, and 24-hour expiry. Passwords are hashed with bcrypt. The setup wizard is one-time-only — after the first admin is created, it never appears again. If `auth_users` is dropped while the setup flag exists, you're locked out and must use the Admin CLI.

### External Clients (PgBouncer)

PgBouncer uses HBA-based authentication: the client connects with username/password, PgBouncer queries the `pgbouncer_get_user()` function to look up the role, HBA rules are checked (per-user IP allowlist), and if allowed, the connection is authenticated against PostgreSQL's password hash. HBA files are regenerated on every user change and PgBouncer is reloaded automatically.

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

The Go backend generates PgBouncer's `pg_hba.conf` file dynamically. It queries the `managed_users` table for all PostgreSQL users and their `allowed_ips`, builds HBA rules (`host all "username" <ip>/32 scram-sha-256`), writes to `/etc/pgbouncer/shared/pg_hba.conf` via a shared volume, and issues `RELOAD` to PgBouncer via the admin console.

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

The init script validates environment variables, writes password files to the shared volume, ensures the PostgreSQL user exists with the correct password, ensures the database exists and is owned correctly, creates the `pgbouncer_auth` user and `pgbouncer_get_user()` function, configures HBA rules (trust for Docker network, reject external), and revokes CONNECT on system databases from PUBLIC.

This runs on **every startup**, not just the first. Password files stay in sync with environment variables, the `pgbouncer_auth` function stays up to date, HBA rules are restored if tampered with, and system database protections are always enforced.
