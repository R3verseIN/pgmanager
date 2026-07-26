#!/bin/sh
set -eu

echo "pgmanager-pgbouncer: reading passwords from files..."

PGMGR_PASS=$(cat /var/lib/postgresql/data/pgmanager-password)
AUTH_PASS=$(cat /var/lib/postgresql/data/pgbouncer-auth-password)

printf '"pgbouncer_auth" "%s"\n"pgmanager" "%s"\n' "$AUTH_PASS" "$PGMGR_PASS" \
    > /etc/pgbouncer/userlist.txt

echo "pgmanager-pgbouncer: userlist.txt generated"
cat /etc/pgbouncer/userlist.txt

# Copy base pgbouncer.ini to shared volume
cp /etc/pgbouncer/pgbouncer.ini /etc/pgbouncer/shared/pgbouncer.ini

# Check PgBouncer SSL preference and certs
SSL_PREF="/var/lib/postgresql/data/pgmanager-pgbouncer-ssl"
CERT_FILE="/var/lib/postgresql/data/server.crt"
KEY_FILE="/var/lib/postgresql/data/server.key"
CA_FILE="/var/lib/postgresql/data/root.crt"

SSL_ENABLED="on"
if [ -f "$SSL_PREF" ]; then
    SSL_ENABLED=$(cat "$SSL_PREF")
fi

if [ "$SSL_ENABLED" != "off" ] && [ -f "$CERT_FILE" ] && [ -f "$KEY_FILE" ]; then
    echo "pgmanager-pgbouncer: enabling client TLS..."

    # Remove existing tls lines
    sed -i '/^client_tls_/d' /etc/pgbouncer/shared/pgbouncer.ini
    sed -i '/^server_tls_/d' /etc/pgbouncer/shared/pgbouncer.ini
    sed -i '/^; Client TLS/d' /etc/pgbouncer/shared/pgbouncer.ini
    sed -i '/^; Server TLS/d' /etc/pgbouncer/shared/pgbouncer.ini

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
    sed -i '/^client_tls_/d' /etc/pgbouncer/shared/pgbouncer.ini
    sed -i '/^server_tls_/d' /etc/pgbouncer/shared/pgbouncer.ini
    sed -i '/^; Client TLS/d' /etc/pgbouncer/shared/pgbouncer.ini
    sed -i '/^; Server TLS/d' /etc/pgbouncer/shared/pgbouncer.ini
fi

# Start PgBouncer in background (NOT exec — so watcher can restart it)
pgbouncer /etc/pgbouncer/shared/pgbouncer.ini &
PGBOUNCER_PID=$!

echo "pgmanager-pgbouncer: PgBouncer started (PID $PGBOUNCER_PID)"

# Background watcher: restart PgBouncer when signal file is detected
(
    while true; do
        if [ -f /etc/pgbouncer/shared/pgbouncer-restart-signal ]; then
            rm -f /etc/pgbouncer/shared/pgbouncer-restart-signal
            echo "pgmanager-pgbouncer: restart signal detected, restarting PgBouncer..."

            # Re-read SSL preference and rebuild config
            SSL_ENABLED="on"
            if [ -f "$SSL_PREF" ]; then
                SSL_ENABLED=$(cat "$SSL_PREF")
            fi

            cp /etc/pgbouncer/pgbouncer.ini /etc/pgbouncer/shared/pgbouncer.ini

            if [ "$SSL_ENABLED" != "off" ] && [ -f "$CERT_FILE" ] && [ -f "$KEY_FILE" ]; then
                sed -i '/^client_tls_/d' /etc/pgbouncer/shared/pgbouncer.ini
                sed -i '/^server_tls_/d' /etc/pgbouncer/shared/pgbouncer.ini
                sed -i '/^; Client TLS/d' /etc/pgbouncer/shared/pgbouncer.ini
                sed -i '/^; Server TLS/d' /etc/pgbouncer/shared/pgbouncer.ini

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
            else
                sed -i '/^client_tls_/d' /etc/pgbouncer/shared/pgbouncer.ini
                sed -i '/^server_tls_/d' /etc/pgbouncer/shared/pgbouncer.ini
                sed -i '/^; Client TLS/d' /etc/pgbouncer/shared/pgbouncer.ini
                sed -i '/^; Server TLS/d' /etc/pgbouncer/shared/pgbouncer.ini
            fi

            # Stop old PgBouncer
            kill -TERM $PGBOUNCER_PID 2>/dev/null || true
            wait $PGBOUNCER_PID 2>/dev/null || true

            # Start new PgBouncer
            pgbouncer /etc/pgbouncer/shared/pgbouncer.ini &
            PGBOUNCER_PID=$!
            echo "pgmanager-pgbouncer: PgBouncer restarted (PID $PGBOUNCER_PID)"
        fi
        sleep 2
    done
) &
WATCHER_PID=$!

# Forward signals to PgBouncer for clean shutdown
cleanup() {
    echo "pgmanager-pgbouncer: shutting down..."
    kill -TERM $PGBOUNCER_PID 2>/dev/null || true
    kill -TERM $WATCHER_PID 2>/dev/null || true
    wait $PGBOUNCER_PID 2>/dev/null || true
    exit 0
}
trap cleanup SIGTERM SIGINT

# Wait for PgBouncer to exit
wait $PGBOUNCER_PID
