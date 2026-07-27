#!/bin/sh
set -eu

PIDFILE=/tmp/pgbouncer.pid
SOCKET=/tmp/.s.PGSQL.6432

echo "pgmanager-pgbouncer: reading passwords from files..."

PGMGR_PASS=$(cat /var/lib/postgresql/data/pgmanager-password)
AUTH_PASS=$(cat /var/lib/postgresql/data/pgbouncer-auth-password)

printf '"pgbouncer_auth" "%s"\n"pgmanager" "%s"\n' "$AUTH_PASS" "$PGMGR_PASS" \
    > /etc/pgbouncer/userlist.txt

echo "pgmanager-pgbouncer: userlist.txt generated"
cat /etc/pgbouncer/userlist.txt

# Copy base pgbouncer.ini to shared volume
cp /etc/pgbouncer/pgbouncer.ini /etc/pgbouncer/shared/pgbouncer.ini

SSL_PREF="/var/lib/postgresql/data/pgmanager-pgbouncer-ssl"
CERT_FILE="/var/lib/postgresql/data/server.crt"
KEY_FILE="/var/lib/postgresql/data/server.key"
CA_FILE="/var/lib/postgresql/data/root.crt"

write_tls_config() {
    sed -i '/^client_tls_/d' /etc/pgbouncer/shared/pgbouncer.ini
    sed -i '/^server_tls_/d' /etc/pgbouncer/shared/pgbouncer.ini
    sed -i '/^; Client TLS/d' /etc/pgbouncer/shared/pgbouncer.ini
    sed -i '/^; Server TLS/d' /etc/pgbouncer/shared/pgbouncer.ini

    SSL_ENABLED="on"
    if [ -f "$SSL_PREF" ]; then
        SSL_ENABLED=$(cat "$SSL_PREF")
    fi

    if [ "$SSL_ENABLED" != "off" ] && [ -f "$CERT_FILE" ] && [ -f "$KEY_FILE" ]; then
        cat >> /etc/pgbouncer/shared/pgbouncer.ini <<EOF

; Client TLS (enabled by pgmanager)
client_tls_sslmode = allow
client_tls_cert_file = $CERT_FILE
client_tls_key_file = $KEY_FILE
client_tls_ca_file = $CA_FILE
EOF
        if [ -f "$CA_FILE" ]; then
            cat >> /etc/pgbouncer/shared/pgbouncer.ini <<EOF

; Server TLS (PgBouncer -> PostgreSQL)
server_tls_sslmode = verify-ca
server_tls_ca_file = $CA_FILE
EOF
        fi
        echo "pgmanager-pgbouncer: client TLS enabled"
    else
        echo "pgmanager-pgbouncer: TLS disabled"
    fi
}

start_pgbouncer() {
    cp /etc/pgbouncer/pgbouncer.ini /etc/pgbouncer/shared/pgbouncer.ini
    write_tls_config
    pgbouncer /etc/pgbouncer/shared/pgbouncer.ini &
    echo $! > "$PIDFILE"
    echo "pgmanager-pgbouncer: PgBouncer started (PID $(cat "$PIDFILE"))"
}

stop_pgbouncer() {
    if [ -f "$PIDFILE" ]; then
        OLD_PID=$(cat "$PIDFILE")
        kill -TERM "$OLD_PID" 2>/dev/null || true
        # Wait for process to exit and socket to be released
        for i in $(seq 1 20); do
            if ! kill -0 "$OLD_PID" 2>/dev/null; then
                break
            fi
            sleep 0.5
        done
        wait "$OLD_PID" 2>/dev/null || true
        # Ensure socket is cleaned up
        rm -f "$SOCKET" 2>/dev/null || true
        sleep 0.5
    fi
}

# Start PgBouncer
start_pgbouncer

# Background watcher: restart PgBouncer when signal file is detected
(
    while true; do
        if [ -f /etc/pgbouncer/shared/pgbouncer-restart-signal ]; then
            rm -f /etc/pgbouncer/shared/pgbouncer-restart-signal
            echo "pgmanager-pgbouncer: restart signal detected, restarting PgBouncer..."

            stop_pgbouncer
            start_pgbouncer
        fi
        sleep 2
    done
) &
WATCHER_PID=$!

# Forward signals for clean shutdown
cleanup() {
    echo "pgmanager-pgbouncer: shutting down..."
    kill -TERM "$WATCHER_PID" 2>/dev/null || true
    wait "$WATCHER_PID" 2>/dev/null || true
    if [ -f "$PIDFILE" ]; then
        kill -TERM "$(cat "$PIDFILE")" 2>/dev/null || true
        wait "$(cat "$PIDFILE")" 2>/dev/null || true
    fi
    exit 0
}
trap cleanup SIGTERM SIGINT

# Block — wait for watcher to exit
wait "$WATCHER_PID"
