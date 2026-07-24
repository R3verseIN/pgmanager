#!/bin/sh
set -e

PASSWORD_FILE="/var/lib/postgresql/data/pgmanager-password"

if [ ! -f "$PASSWORD_FILE" ] || [ ! -s "$PASSWORD_FILE" ]; then
    NEW_PASSWORD=$(head -c 32 /dev/urandom | base64 | tr -d '\n=/+' | head -c 24)
    echo "$NEW_PASSWORD" > "$PASSWORD_FILE"
    chmod 600 "$PASSWORD_FILE"
    echo "Generated new pgmanager password"
fi

CURRENT_PASSWORD=$(cat "$PASSWORD_FILE")

psql -v ON_ERROR_STOP=1 -U pgmanager -d postgres <<-EOSQL
    ALTER USER pgmanager PASSWORD '${CURRENT_PASSWORD}';

    SELECT 'CREATE DATABASE pgmanager' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'pgmanager')\gexec
EOSQL

psql -v ON_ERROR_STOP=1 -U pgmanager -d pgmanager <<-EOSQL
    DO \$\$
    BEGIN
        CREATE USER pgbouncer_auth WITH PASSWORD 'pgbouncer_auth_password';
    EXCEPTION WHEN duplicate_object THEN
        RAISE NOTICE 'user pgbouncer_auth already exists, skipping';
    END
    \$\$;

    GRANT CONNECT ON DATABASE pgmanager TO pgbouncer_auth;
    GRANT USAGE ON SCHEMA public TO pgbouncer_auth;

    CREATE OR REPLACE FUNCTION public.pgbouncer_get_user(
        p_usename TEXT
    )
    RETURNS TABLE (
        username TEXT,
        password TEXT
    )
    SECURITY DEFINER
    AS \$\$
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
          AND r.rolname != 'pgmanager';
    END;
    \$\$ LANGUAGE plpgsql;

    REVOKE ALL ON FUNCTION public.pgbouncer_get_user(TEXT) FROM PUBLIC;
    GRANT EXECUTE ON FUNCTION public.pgbouncer_get_user(TEXT) TO pgbouncer_auth;
EOSQL

echo "pgmanager database and pgbouncer_auth user created successfully"
