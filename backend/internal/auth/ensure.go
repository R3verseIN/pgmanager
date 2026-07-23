package auth

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

const ensurePgbouncerAuthSQL = `
DO $$
BEGIN
    CREATE USER pgbouncer_auth WITH PASSWORD 'pgbouncer_auth_password';
EXCEPTION WHEN duplicate_object THEN
    RAISE NOTICE 'user pgbouncer_auth already exists, skipping';
END
$$;

GRANT CONNECT ON DATABASE postgres TO pgbouncer_auth;
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
`

const checkFunctionSQL = `
SELECT EXISTS (
    SELECT 1
    FROM pg_proc p
    JOIN pg_namespace n ON p.pronamespace = n.oid
    WHERE p.proname = 'pgbouncer_get_user'
      AND n.nspname = 'public'
      AND p.prosrc LIKE '%rolname != ''pgmanager''%'
)
`

const checkUserSQL = `
SELECT EXISTS (
    SELECT 1
    FROM pg_roles
    WHERE rolname = 'pgbouncer_auth'
)
`

// EnsurePgbouncerAuth verifies the pgbouncer_auth user and pgbouncer_get_user
// function exist and are correct. It recreates them if missing or modified.
func EnsurePgbouncerAuth(ctx context.Context, pool *pgxpool.Pool) error {
	var userExists bool
	if err := pool.QueryRow(ctx, checkUserSQL).Scan(&userExists); err != nil {
		return err
	}

	var funcExists bool
	if err := pool.QueryRow(ctx, checkFunctionSQL).Scan(&funcExists); err != nil {
		return err
	}

	if userExists && funcExists {
		log.Println("pgbouncer_auth: already healthy")
		return nil
	}

	if !userExists {
		log.Println("pgbouncer_auth: user missing, recreating...")
	}
	if !funcExists {
		log.Println("pgbouncer_get_user: function missing or modified, recreating...")
	}

	_, err := pool.Exec(ctx, ensurePgbouncerAuthSQL)
	if err != nil {
		return err
	}

	log.Println("pgbouncer_auth: restored successfully")
	return nil
}
