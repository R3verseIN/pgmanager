#!/bin/sh
set -e

# If running as root, re-exec as postgres user (same as Docker postgres image)
if [ "$(id -u)" = '0' ]; then
    exec gosu postgres "$0" "$@"
fi

# Only run repair checks if the data directory already has data
# On first start (empty data dir), docker-entrypoint.sh handles init via initdb
if [ -f /var/lib/postgresql/data/PG_VERSION ]; then
    echo "pgmanager: data directory exists, running repair checks..."
    pg_ctl -D /var/lib/postgresql/data -l /tmp/pgmanager-init.log start -o "-c listen_addresses='' -c ssl=off"

    echo "pgmanager: waiting for PostgreSQL..."
    until pg_isready -U "${POSTGRES_USER}" -q; do
        sleep 0.5
    done

    echo "pgmanager: running init/repair script..."
    python3 /usr/local/bin/pgmanager-init.py

    echo "pgmanager: stopping temporary PostgreSQL..."
    pg_ctl -D /var/lib/postgresql/data stop -m fast
else
    echo "pgmanager: first start, delegating to PostgreSQL init scripts..."
fi

echo "pgmanager: starting PostgreSQL normally..."
exec /usr/local/bin/docker-entrypoint.sh postgres
