package users

import (
	"context"
	"log"

	"pgmanager/internal/handler/core"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func InitUserSchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS managed_users (
			username      TEXT NOT NULL,
			database_name TEXT NOT NULL,
			access        TEXT NOT NULL CHECK (access IN ('read', 'write', 'ddl', 'full')),
			allowed_ips   JSONB NOT NULL DEFAULT '["0.0.0.0/0"]'::jsonb,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (username, database_name)
		);
	`)
	if err != nil {
		return err
	}

	_, err = pool.Exec(ctx, `
		ALTER TABLE managed_users DROP CONSTRAINT IF EXISTS managed_users_pkey;
		ALTER TABLE managed_users ADD PRIMARY KEY (username, database_name);
		ALTER TABLE managed_users
			ADD COLUMN IF NOT EXISTS allowed_ips JSONB NOT NULL DEFAULT '["0.0.0.0/0"]'::jsonb;
	`)
	if err != nil {
		return err
	}

	_, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS auth_users (
			id            SERIAL PRIMARY KEY,
			username      TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			role          TEXT NOT NULL CHECK (role IN ('admin', 'dev', 'viewer')),
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	if err != nil {
		return err
	}

	_, _ = pool.Exec(ctx, `
		DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'auth_users_role_check' AND conrelid = 'auth_users'::regclass) THEN
				ALTER TABLE auth_users DROP CONSTRAINT auth_users_role_check;
				ALTER TABLE auth_users ADD CONSTRAINT auth_users_role_check CHECK (role IN ('admin', 'dev', 'viewer'));
			END IF;
		END
		$$;
	`)

	_, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS dev_databases (
			auth_user_id  INTEGER NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
			database_name TEXT NOT NULL,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (auth_user_id, database_name)
		);
	`)
	if err != nil {
		return err
	}

	_, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS sessions (
			id         TEXT PRIMARY KEY,
			user_id    INTEGER NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '24 hours'
		)
	`)
	if err != nil {
		return err
	}

	_, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS audit_log (
			id          SERIAL PRIMARY KEY,
			username    TEXT NOT NULL,
			action      TEXT NOT NULL,
			database    TEXT NOT NULL,
			table_name  TEXT,
			detail      JSONB,
			ip_address  TEXT,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return err
	}

	_, _ = pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON audit_log(created_at DESC)`)
	_, _ = pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_audit_log_username ON audit_log(username)`)
	_, _ = pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_audit_log_action ON audit_log(action)`)

	_, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS system_config (
			key        TEXT PRIMARY KEY,
			value      TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return err
	}

	_, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS pgbouncer_databases (
			database_name TEXT PRIMARY KEY,
			allowed      BOOLEAN NOT NULL DEFAULT false,
			created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return err
	}

	_, _ = pool.Exec(ctx, `
		INSERT INTO pgbouncer_databases (database_name, allowed)
		VALUES ('pgmanager', false), ('postgres', false), ('template0', false), ('template1', false)
		ON CONFLICT (database_name) DO NOTHING
	`)

	_, _ = pool.Exec(ctx, `
		INSERT INTO system_config (key, value) VALUES
			('pgbouncer_pool_mode', 'transaction'),
			('pgbouncer_default_pool_size', '20'),
			('pgbouncer_max_client_conn', '100')
		ON CONFLICT (key) DO NOTHING
	`)

	return nil
}

func GrantAccess(ctx context.Context, pool *pgxpool.Pool, baseDSN string, username, db, access string) error {
	if _, err := pool.Exec(ctx, "GRANT CONNECT ON DATABASE "+core.QuoteIdent(db)+" TO "+core.QuoteIdent(username)); err != nil {
		return err
	}

	return WithDatabase(ctx, baseDSN, db, func(conn *pgx.Conn) error {
		if _, err := conn.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+core.QuoteIdent(username)); err != nil {
			return err
		}

		switch access {
		case "read":
			if _, err := conn.Exec(ctx, "GRANT SELECT ON ALL TABLES IN SCHEMA public TO "+core.QuoteIdent(username)); err != nil {
				return err
			}
			if _, err := conn.Exec(ctx, "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO "+core.QuoteIdent(username)); err != nil {
				return err
			}
		case "write":
			if _, err := conn.Exec(ctx, "GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO "+core.QuoteIdent(username)); err != nil {
				return err
			}
			if _, err := conn.Exec(ctx, "GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO "+core.QuoteIdent(username)); err != nil {
				return err
			}
			if _, err := conn.Exec(ctx, "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO "+core.QuoteIdent(username)); err != nil {
				return err
			}
			if _, err := conn.Exec(ctx, "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO "+core.QuoteIdent(username)); err != nil {
				return err
			}
		case "ddl":
			if _, err := pool.Exec(ctx, "GRANT CREATE ON DATABASE "+core.QuoteIdent(db)+" TO "+core.QuoteIdent(username)); err != nil {
				return err
			}
			if _, err := conn.Exec(ctx, "GRANT USAGE, CREATE ON SCHEMA public TO "+core.QuoteIdent(username)); err != nil {
				return err
			}
			if _, err := conn.Exec(ctx, "GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO "+core.QuoteIdent(username)); err != nil {
				return err
			}
			if _, err := conn.Exec(ctx, "GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO "+core.QuoteIdent(username)); err != nil {
				return err
			}
			if _, err := conn.Exec(ctx, "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO "+core.QuoteIdent(username)); err != nil {
				return err
			}
			if _, err := conn.Exec(ctx, "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO "+core.QuoteIdent(username)); err != nil {
				return err
			}
		case "full":
			if _, err := pool.Exec(ctx, "GRANT ALL PRIVILEGES ON DATABASE "+core.QuoteIdent(db)+" TO "+core.QuoteIdent(username)); err != nil {
				return err
			}
			if _, err := conn.Exec(ctx, "GRANT ALL ON SCHEMA public TO "+core.QuoteIdent(username)); err != nil {
				return err
			}
			if _, err := conn.Exec(ctx, "GRANT ALL ON ALL TABLES IN SCHEMA public TO "+core.QuoteIdent(username)); err != nil {
				return err
			}
			if _, err := conn.Exec(ctx, "GRANT ALL ON ALL SEQUENCES IN SCHEMA public TO "+core.QuoteIdent(username)); err != nil {
				return err
			}
			if _, err := conn.Exec(ctx, "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO "+core.QuoteIdent(username)); err != nil {
				return err
			}
			if _, err := conn.Exec(ctx, "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO "+core.QuoteIdent(username)); err != nil {
				return err
			}
		}
		return nil
	})
}

func RevokeAccess(ctx context.Context, pool *pgxpool.Pool, baseDSN string, username, db string) error {
	if _, err := pool.Exec(ctx, "REVOKE ALL PRIVILEGES ON DATABASE "+core.QuoteIdent(db)+" FROM "+core.QuoteIdent(username)); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, "REVOKE CONNECT ON DATABASE "+core.QuoteIdent(db)+" FROM "+core.QuoteIdent(username)); err != nil {
		return err
	}

	return WithDatabase(ctx, baseDSN, db, func(conn *pgx.Conn) error {
		if _, err := conn.Exec(ctx, "REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM "+core.QuoteIdent(username)); err != nil {
			return err
		}
		if _, err := conn.Exec(ctx, "REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM "+core.QuoteIdent(username)); err != nil {
			return err
		}
		if _, err := conn.Exec(ctx, "REVOKE ALL ON SCHEMA public FROM "+core.QuoteIdent(username)); err != nil {
			return err
		}
		if _, err := conn.Exec(ctx, "ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE ALL ON TABLES FROM "+core.QuoteIdent(username)); err != nil {
			return err
		}
		if _, err := conn.Exec(ctx, "ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE ALL ON SEQUENCES FROM "+core.QuoteIdent(username)); err != nil {
			return err
		}
		return nil
	})
}

func RollbackUser(ctx context.Context, pool *pgxpool.Pool, baseDSN string, username string) {
	databases := GetUserDatabases(ctx, pool, username)
	for _, db := range databases {
		WithDatabase(ctx, baseDSN, db, func(conn *pgx.Conn) error {
			if _, err := conn.Exec(ctx, "DROP OWNED BY "+core.QuoteIdent(username)+" CASCADE"); err != nil {
				log.Printf("RollbackUser: DROP OWNED BY on %s failed: %v", db, err)
			}
			return nil
		})
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		log.Printf("RollbackUser: acquire connection failed: %v", err)
		return
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "DROP OWNED BY "+core.QuoteIdent(username)+" CASCADE"); err != nil {
		log.Printf("RollbackUser: DROP OWNED BY on pool failed: %v", err)
	}
	if _, err := conn.Exec(ctx, "DROP ROLE IF EXISTS "+core.QuoteIdent(username)); err != nil {
		log.Printf("RollbackUser: DROP ROLE failed: %v", err)
	}
}
