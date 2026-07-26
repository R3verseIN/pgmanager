#!/usr/bin/env python3
"""
pgmanager-init: PostgreSQL-level self-healing script.

Runs on every startup (after PostgreSQL is ready).
Validates env vars, ensures users/functions/password files exist.
Configures SSL based on preference file and certificate presence.
"""

import os
import re
import subprocess
import sys
from pathlib import Path

DATA_DIR = Path("/var/lib/postgresql/data")
PASSWORD_FILE = DATA_DIR / "pgmanager-password"
AUTH_PASSWORD_FILE = DATA_DIR / "pgbouncer-auth-password"
SSL_PREF_FILE = DATA_DIR / "pgmanager-ssl-enabled"
PGBOUNCER_SSL_PREF_FILE = DATA_DIR / "pgmanager-pgbouncer-ssl"


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
        # Determine external rule based on SSL preference
        ssl_pref_path = DATA_DIR / "pgmanager-ssl-enabled"
        ssl_enabled = "on"
        if ssl_pref_path.exists():
            ssl_enabled = ssl_pref_path.read_text().strip()

        if ssl_enabled == "off":
            external_rule = "host all all 0.0.0.0/0 scram-sha-256\nhost all all ::0/0 scram-sha-256"
        else:
            external_rule = "hostssl all all 0.0.0.0/0 scram-sha-256\nhostssl all all ::0/0 scram-sha-256"

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
            f"{external_rule}\n"
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


def update_hba_rules_for_ssl(require_ssl: bool) -> None:
    """Update external connection HBA rules based on SSL preference."""
    hba_file = DATA_DIR / "pg_hba.conf"
    if not hba_file.exists():
        return

    content = hba_file.read_text()

    if require_ssl:
        # Replace host with hostssl for external connections
        content = re.sub(
            r'^host\s+all\s+all\s+0\.0\.0\.0/0\s+',
            'hostssl all all 0.0.0.0/0 ',
            content,
            flags=re.MULTILINE,
        )
        content = re.sub(
            r'^host\s+all\s+all\s+::0/0\s+',
            'hostssl all all ::0/0 ',
            content,
            flags=re.MULTILINE,
        )
    else:
        # Replace hostssl with host for external connections
        content = re.sub(
            r'^hostssl\s+all\s+all\s+0\.0\.0\.0/0\s+',
            'host all all 0.0.0.0/0 ',
            content,
            flags=re.MULTILINE,
        )
        content = re.sub(
            r'^hostssl\s+all\s+all\s+::0/0\s+',
            'host all all ::0/0 ',
            content,
            flags=re.MULTILINE,
        )

    hba_file.write_text(content)
    print(f"pgmanager-init: HBA rules updated (require_ssl={require_ssl})")


def revoke_system_db_connect() -> None:
    print("pgmanager-init: revoking CONNECT on system databases from PUBLIC...")
    run_sql("REVOKE CONNECT ON DATABASE postgres FROM PUBLIC", dbname="postgres")
    run_sql("REVOKE CONNECT ON DATABASE template1 FROM PUBLIC", dbname="template1")
    print("pgmanager-init: system database CONNECT revoked from PUBLIC")


def configure_wal_archiving() -> None:
    """Configure PostgreSQL WAL archiving for WAL-G if WALG_S3_PREFIX is set."""
    s3_prefix = os.environ.get("WALG_S3_PREFIX", "")
    if not s3_prefix:
        print("pgmanager-init: WALG_S3_PREFIX not set, skipping WAL archiving config")
        return

    print("pgmanager-init: configuring WAL archiving for WAL-G...")
    archive_timeout = os.environ.get("WALG_ARCHIVE_TIMEOUT", "300")

    settings = {
        "wal_level": "replica",
        "archive_mode": "on",
        "archive_command": "wal-g wal-push %p",
        "archive_timeout": archive_timeout,
    }
    for key, value in settings.items():
        run_sql(f"ALTER SYSTEM SET {key} = '{value}';")
    print(f"pgmanager-init: WAL archiving configured (timeout={archive_timeout}s)")


def generate_self_signed_certs() -> None:
    """Generate self-signed CA and server certificates using openssl."""
    print("pgmanager-init: generating self-signed SSL certificates...")

    cert_path = DATA_DIR / "server.crt"
    key_path = DATA_DIR / "server.key"
    ca_cert_path = DATA_DIR / "root.crt"
    ca_key_path = DATA_DIR / "root.key"

    # Generate CA key
    subprocess.run(
        ["openssl", "ecparam", "-genkey", "-name", "prime256v1",
         "-out", str(ca_key_path)],
        check=True, capture_output=True,
    )
    ca_key_path.chmod(0o600)

    # Generate CA cert (valid 10 years)
    subprocess.run(
        ["openssl", "req", "-new", "-x509",
         "-key", str(ca_key_path),
         "-out", str(ca_cert_path),
         "-days", "3650",
         "-subj", "/CN=pgmanager-ca/O=pgmanager"],
        check=True, capture_output=True,
    )

    # Generate server key
    subprocess.run(
        ["openssl", "ecparam", "-genkey", "-name", "prime256v1",
         "-out", str(key_path)],
        check=True, capture_output=True,
    )
    key_path.chmod(0o600)

    # Generate server CSR
    csr_path = DATA_DIR / "server.csr"
    subprocess.run(
        ["openssl", "req", "-new",
         "-key", str(key_path),
         "-out", str(csr_path),
         "-subj", "/CN=pgmanager-server/O=pgmanager"],
        check=True, capture_output=True,
    )

    # Sign server cert with CA (valid 5 years)
    subprocess.run(
        ["openssl", "x509", "-req",
         "-in", str(csr_path),
         "-CA", str(ca_cert_path),
         "-CAkey", str(ca_key_path),
         "-CAcreateserial",
         "-out", str(cert_path),
         "-days", "1825"],
        check=True, capture_output=True,
    )

    # Clean up CSR and serial
    csr_path.unlink(missing_ok=True)
    (DATA_DIR / "root.srl").unlink(missing_ok=True)

    print("pgmanager-init: self-signed SSL certificates generated")


def configure_ssl() -> None:
    """Configure SSL based on preference file and certificate presence."""
    print("pgmanager-init: configuring SSL...")

    cert_path = DATA_DIR / "server.crt"
    key_path = DATA_DIR / "server.key"
    ca_path = DATA_DIR / "root.crt"

    # Read user's SSL preference (default: "on" for first boot)
    ssl_enabled = "on"
    if SSL_PREF_FILE.exists():
        ssl_enabled = SSL_PREF_FILE.read_text().strip()

    if ssl_enabled == "off":
        print("pgmanager-init: SSL preference is off, disabling SSL")
        run_sql("ALTER SYSTEM SET ssl = 'off';")
        update_hba_rules_for_ssl(require_ssl=False)
        return

    # SSL is enabled (default or explicit "on")
    # Generate self-signed certs if none exist
    if not cert_path.exists() or not key_path.exists():
        generate_self_signed_certs()

    print("pgmanager-init: enabling SSL...")
    run_sql("ALTER SYSTEM SET ssl = 'on';")
    run_sql(f"ALTER SYSTEM SET ssl_cert_file = '{cert_path}';")
    run_sql(f"ALTER SYSTEM SET ssl_key_file = '{key_path}';")
    if ca_path.exists():
        run_sql(f"ALTER SYSTEM SET ssl_ca_file = '{ca_path}';")

    # Update HBA rules to require SSL for external connections
    update_hba_rules_for_ssl(require_ssl=True)
    print("pgmanager-init: SSL enabled")


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
        configure_wal_archiving()
        configure_ssl()
        print("pgmanager-init: all checks passed")
    except Exception as e:
        print(f"pgmanager-init: ERROR: {e}", file=sys.stderr)
        return 1

    print("pgmanager-init: done")
    return 0


if __name__ == "__main__":
    sys.exit(main())
