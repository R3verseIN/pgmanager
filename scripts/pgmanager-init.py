#!/usr/bin/env python3
"""
pgmanager-init: PostgreSQL-level self-healing script.

Runs on every startup (after PostgreSQL is ready).
Validates env vars, ensures users/functions/password files exist.
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
    print("pgmanager-init: ensuring HBA rules...")
    hba_file = DATA_DIR / "pg_hba.conf"
    content = hba_file.read_text()

    # Only replace if the old catch-all scram-sha-256 line still exists
    if "scram-sha-256" in content and "reject" not in content.split("scram-sha-256")[0].split("\n")[-2]:
        new_rules = (
            "# Internal Docker network (trust - isolated network)\n"
            "host all all 172.16.0.0/12 trust\n"
            "host all all 192.168.0.0/16 trust\n"
            "host all all 10.0.0.0/8 trust\n"
            "# External connections rejected (must go through PgBouncer)\n"
            "host all all 0.0.0.0/0 reject\n"
            "host all all ::0/0 reject\n"
        )
        content = re.sub(
            r'host\s+all\s+all\s+all\s+scram-sha-256',
            new_rules.rstrip(),
            content,
        )
        hba_file.write_text(content)
        print("pgmanager-init: HBA rules updated")
    else:
        print("pgmanager-init: HBA rules already configured")


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
        print("pgmanager-init: all checks passed")
    except Exception as e:
        print(f"pgmanager-init: ERROR: {e}", file=sys.stderr)
        return 1

    print("pgmanager-init: done")
    return 0


if __name__ == "__main__":
    sys.exit(main())
