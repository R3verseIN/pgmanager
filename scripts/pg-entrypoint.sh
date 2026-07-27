#!/bin/sh
set -e

# === FIRST PASS (root): SSH setup + start sshd ===
if [ "$(id -u)" = '0' ]; then
    ssh-keygen -A 2>/dev/null || true
    mkdir -p /root/.ssh /var/run/sshd /root/.shared-ssh
    ssh-keygen -t ed25519 -f /root/.ssh/pgmanager_ed25519 -N "" -q
    cat /root/.ssh/pgmanager_ed25519.pub > /root/.ssh/authorized_keys
    chmod 700 /root/.ssh
    chmod 600 /root/.ssh/authorized_keys /root/.ssh/pgmanager_ed25519
    cp /root/.ssh/pgmanager_ed25519 /root/.shared-ssh/pgmanager_ed25519
    chmod 600 /root/.shared-ssh/pgmanager_ed25519
    /usr/sbin/sshd
    exec gosu postgres "$0" "$@"
fi

# === SECOND PASS (postgres): PostgreSQL lifecycle ===

SIGNAL_FILE="/var/lib/postgresql/data/pgmanager-restart-signal"
PGDATA="/var/lib/postgresql/data"

# Wait for PostgreSQL to be ready, then grab its PID
wait_for_postgres() {
    for i in $(seq 1 60); do
        if pg_isready -q 2>/dev/null; then
            return 0
        fi
        sleep 0.5
    done
    return 1
}

# First boot: run official entrypoint in background (handles initdb + init scripts)
if [ ! -f "$PGDATA/PG_VERSION" ]; then
    echo "pgmanager: first start, initializing via official entrypoint..."
    /usr/local/bin/docker-entrypoint.sh postgres &
    ENTRYPOINT_PID=$!
    wait_for_postgres
    echo "pgmanager: database initialized and ready"
    # The entrypoint PID IS the postgres PID (exec replaces the shell)
    PG_PID=$ENTRYPOINT_PID
else
    echo "pgmanager: data directory exists, starting PostgreSQL..."
    postgres -D "$PGDATA" &
    PG_PID=$!
    wait_for_postgres
    echo "pgmanager: PostgreSQL started"
fi

echo "pgmanager: PostgreSQL pid=$PG_PID"

# Watch for restart signals — this is the main process (Docker keeps container alive)
while true; do
    if ! kill -0 $PG_PID 2>/dev/null; then
        echo "pgmanager: PostgreSQL died unexpectedly"
        exit 1
    fi

    if [ -f "$SIGNAL_FILE" ]; then
        rm -f "$SIGNAL_FILE"
        echo "pgmanager: restart signal detected, restarting PostgreSQL..."

        # Stop PostgreSQL (we're already running as postgres)
        pg_ctl stop -D "$PGDATA" -m fast -t 30
        wait $PG_PID 2>/dev/null || true

        # Start PostgreSQL again
        postgres -D "$PGDATA" &
        PG_PID=$!

        # Wait for it to be ready
        for i in $(seq 1 30); do
            if pg_isready -q 2>/dev/null; then
                break
            fi
            sleep 0.5
        done

        echo "pgmanager: PostgreSQL restarted (pid=$PG_PID)"
    fi

    sleep 2
done
