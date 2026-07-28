-- testutil/schema.sql
-- Recreates the pgmanager schema for integration tests.
-- Mirrors pgmanager-init.py + InitUserSchema().

-- pgbouncer auth user
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'pgbouncer_auth') THEN
        CREATE USER pgbouncer_auth;
    END IF;
END
$$;

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
      AND r.rolname != 'pgmanager';
END;
$$ LANGUAGE plpgsql;

REVOKE ALL ON FUNCTION public.pgbouncer_get_user(TEXT) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.pgbouncer_get_user(TEXT) TO pgbouncer_auth;

-- Application tables
CREATE TABLE IF NOT EXISTS managed_users (
    username      TEXT NOT NULL,
    database_name TEXT NOT NULL,
    access        TEXT NOT NULL CHECK (access IN ('read', 'write', 'ddl', 'full')),
    allowed_ips   JSONB NOT NULL DEFAULT '["0.0.0.0/0"]'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (username, database_name)
);

CREATE TABLE IF NOT EXISTS auth_users (
    id            SERIAL PRIMARY KEY,
    username      TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL CHECK (role IN ('admin', 'dev', 'viewer')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS dev_databases (
    auth_user_id  INTEGER NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
    database_name TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (auth_user_id, database_name)
);

CREATE TABLE IF NOT EXISTS sessions (
    id         TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '24 hours'
);

CREATE TABLE IF NOT EXISTS audit_log (
    id          SERIAL PRIMARY KEY,
    username    TEXT NOT NULL,
    action      TEXT NOT NULL,
    database    TEXT NOT NULL,
    table_name  TEXT,
    detail      JSONB,
    ip_address  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON audit_log(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_username ON audit_log(username);
CREATE INDEX IF NOT EXISTS idx_audit_log_action ON audit_log(action);

CREATE TABLE IF NOT EXISTS system_config (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS pgbouncer_databases (
    database_name TEXT PRIMARY KEY,
    allowed      BOOLEAN NOT NULL DEFAULT false,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO pgbouncer_databases (database_name, allowed)
VALUES ('pgmanager', false), ('postgres', false), ('template0', false), ('template1', false)
ON CONFLICT (database_name) DO NOTHING;

INSERT INTO system_config (key, value) VALUES
    ('pgbouncer_pool_mode', 'transaction'),
    ('pgbouncer_default_pool_size', '20'),
    ('pgbouncer_max_client_conn', '100')
ON CONFLICT (key) DO NOTHING;

INSERT INTO system_config (key, value) VALUES ('setup_completed', 'true')
ON CONFLICT (key) DO UPDATE SET value = 'true';
