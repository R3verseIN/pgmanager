#!/usr/bin/env python3
"""
pgmanager-init: First-boot initialization script.

Runs ONCE on first start (when data directory is empty).
Creates users, databases, and pgbouncer_auth function.
SSL and WAL archiving are managed by the Go app via SSH.
"""

import os
import re
import subprocess
import sys
from pathlib import Path

DATA_DIR = Path("/var/lib/postgresql/data")
PASSWORD_FILE = DATA_DIR / "pgmanager-password"
AUTH_PASSWORD_FILE = DATA_DIR / "pgbouncer-auth-password"


def fatal(msg: str) -> None:
    print(f"FATAL: {msg}", file=sys.stderr)
    sys.exit(1)


def get_super_user() -> str:
    return os.environ.get("POSTGRES_USER", "postgres")


def get_db_name() -> str:
    return os.environ.get("POSTGRES_DB", "postgres")


def run_sql(sql: str, dbname: str = None) -> None:
    super_user = get_super_user()
    if dbname is None:
        dbname = get_db_name()
    result = subprocess.run(
        ["psql", "-v", "ON_ERROR_STOP=1", "-U", super_user, "-d", dbname, "-c", sql],
        capture_output=True,
    )
    if result.returncode != 0:
        raise RuntimeError(f"psql failed: {result.stderr.decode().strip()}")


def validate_password(password: str, name: str) -> None:
    if not password:
        fatal(f"{name} is empty")
    if len(password) < 8 or len(password) > 72:
        fatal(f"{name} must be 8-72 characters (got {len(password)})")
    if not re.match(r'^[a-zA-Z0-9_-]+$', password):
        fatal(f"{name} contains invalid characters (only alphanumeric, _, - allowed)")


def write_password_files(pg_pass: str, auth_pass: str) -> None:
    print("pgmanager-init: writing password files...")
    PASSWORD_FILE.write_text(pg_pass)
    PASSWORD_FILE.chmod(0o600)
    AUTH_PASSWORD_FILE.write_text(auth_pass)
    AUTH_PASSWORD_FILE.chmod(0o600)


def ensure_pgmanager_user(pg_pass: str) -> None:
    super_user = get_super_user()
    print(f"pgmanager-init: ensuring {super_user} user...")
    run_sql(f"ALTER USER {super_user} PASSWORD '{pg_pass}'")


def ensure_database() -> None:
    super_user = get_super_user()
    db_name = get_db_name()
    print(f"pgmanager-init: ensuring {db_name} database...")
    run_sql(f"""
        DO $$
        BEGIN
            IF NOT EXISTS (SELECT FROM pg_database WHERE datname = '{db_name}') THEN
                CREATE DATABASE {db_name};
            END IF;
        END
        $$;
    """)
    run_sql(f"ALTER DATABASE {db_name} OWNER TO {super_user}")


def ensure_pgbouncer_auth(auth_pass: str) -> None:
    super_user = get_super_user()
    db_name = get_db_name()
    print("pgmanager-init: ensuring pgbouncer_auth user and function...")
    sql = f"""
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'pgbouncer_auth') THEN
        CREATE USER pgbouncer_auth WITH PASSWORD '{auth_pass}';
    ELSE
        ALTER USER pgbouncer_auth WITH PASSWORD '{auth_pass}';
    END IF;
END
$$;

GRANT CONNECT ON DATABASE {db_name} TO pgbouncer_auth;
GRANT USAGE ON SCHEMA public TO pgbouncer_auth;

CREATE OR REPLACE FUNCTION public.pgbouncer_get_user(
    p_usename TEXT
)
RETURNS TABLE (
    username TEXT,
    password TEXT
)
SECURITY DEFINER
AS $$
BEGIN
    RETURN QUERY
    SELECT
        r.rolname::TEXT,
        CASE
            WHEN r.rolvaliduntil IS NULL OR r.rolvaliduntil > now()
            THEN r.rolpassword::TEXT
            ELSE NULL
        END
    FROM pg_authid r
    WHERE r.rolname = p_usename
      AND r.rolcanlogin = true
      AND r.rolname != '{super_user}';
END;
$$ LANGUAGE plpgsql;

REVOKE ALL ON FUNCTION public.pgbouncer_get_user(TEXT) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.pgbouncer_get_user(TEXT) TO pgbouncer_auth;
"""
    run_sql(sql)


def ensure_hba_rules() -> None:
    """Replace the default PG17 catch-all scram-sha-256 line with per-user rules."""
    print("pgmanager-init: ensuring HBA rules...")
    hba_file = DATA_DIR / "pg_hba.conf"
    content = hba_file.read_text()

    if "# pgmanager-init: managed" in content:
        print("pgmanager-init: HBA rules already configured")
        return

    if "scram-sha-256" in content:
        new_rules = (
            "# pgmanager-init: managed (scram-sha-256 auth)\n"
            "host all pgmanager 172.16.0.0/12 scram-sha-256\n"
            "host all pgmanager 192.168.0.0/16 scram-sha-256\n"
            "host all pgmanager 10.0.0.0/8 scram-sha-256\n"
            "# Replication from Docker networks\n"
            "host replication pgmanager 172.16.0.0/12 scram-sha-256\n"
            "host replication pgmanager 192.168.0.0/16 scram-sha-256\n"
            "host replication pgmanager 10.0.0.0/8 scram-sha-256\n"
            "# PgBouncer auth (trust - standard pattern)\n"
            "host all pgbouncer_auth 172.16.0.0/12 trust\n"
            "host all pgbouncer_auth 192.168.0.0/16 trust\n"
            "host all pgbouncer_auth 10.0.0.0/8 trust\n"
            "# External connections\n"
            "hostssl all all 0.0.0.0/0 scram-sha-256\n"
            "hostssl all all ::0/0 scram-sha-256\n"
        )
        content = re.sub(
            r'host\s+all\s+all\s+all\s+scram-sha-256',
            new_rules.rstrip(),
            content,
        )
        hba_file.write_text(content)
        print("pgmanager-init: HBA rules updated")
    else:
        print("pgmanager-init: no scram-sha-256 catch-all found, rules may need manual setup")


def revoke_system_db_connect() -> None:
    print("pgmanager-init: revoking CONNECT on system databases from PUBLIC...")
    run_sql("REVOKE CONNECT ON DATABASE postgres FROM PUBLIC", dbname="postgres")
    run_sql("REVOKE CONNECT ON DATABASE template1 FROM PUBLIC", dbname="template1")
    print("pgmanager-init: system database CONNECT revoked from PUBLIC")


def main() -> int:
    print("pgmanager-init: starting...")

    pg_pass = os.environ.get("POSTGRES_PASSWORD")
    auth_pass = os.environ.get("PGBOUNCER_AUTH_PASSWORD")

    if not pg_pass:
        fatal("POSTGRES_PASSWORD is required but not set")
    if not auth_pass:
        fatal("PGBOUNCER_AUTH_PASSWORD is required but not set")

    validate_password(pg_pass, "POSTGRES_PASSWORD")
    validate_password(auth_pass, "PGBOUNCER_AUTH_PASSWORD")

    try:
        write_password_files(pg_pass, auth_pass)
        ensure_pgmanager_user(pg_pass)
        ensure_database()
        ensure_pgbouncer_auth(auth_pass)
        ensure_hba_rules()
        revoke_system_db_connect()
        print("pgmanager-init: all checks passed")
    except Exception as e:
        print(f"pgmanager-init: ERROR: {e}", file=sys.stderr)
        return 1

    print("pgmanager-init: done")
    return 0


if __name__ == "__main__":
    sys.exit(main())
