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

# Check if PgBouncer SSL is enabled via pgmanager config
SSL_CONF="/etc/pgbouncer/shared/pgbouncer-ssl.conf"
CERT_FILE="/var/lib/postgresql/data/server.crt"
KEY_FILE="/var/lib/postgresql/data/server.key"
CA_FILE="/var/lib/postgresql/data/root.crt"

SSL_ENABLED="off"
if [ -f "$SSL_CONF" ]; then
    SSL_ENABLED=$(cat "$SSL_CONF")
fi

if [ "$SSL_ENABLED" = "on" ] && [ -f "$CERT_FILE" ] && [ -f "$KEY_FILE" ]; then
    echo "pgmanager-pgbouncer: enabling client TLS..."

    # Remove existing client_tls lines and append active ones
    sed -i '/^client_tls_/d' /etc/pgbouncer/shared/pgbouncer.ini

    cat >> /etc/pgbouncer/shared/pgbouncer.ini <<EOF

; Client TLS (enabled by pgmanager)
client_tls_sslmode = allow
client_tls_cert_file = $CERT_FILE
client_tls_key_file = $KEY_FILE
client_tls_ca_file = $CA_FILE
EOF

    # Also configure server TLS (PgBouncer -> PostgreSQL)
    if [ -f "$CA_FILE" ]; then
        sed -i '/^server_tls_/d' /etc/pgbouncer/shared/pgbouncer.ini
        cat >> /etc/pgbouncer/shared/pgbouncer.ini <<EOF

; Server TLS (PgBouncer -> PostgreSQL)
server_tls_sslmode = verify-ca
server_tls_ca_file = $CA_FILE
EOF
    fi

    echo "pgmanager-pgbouncer: client TLS enabled"
else
    echo "pgmanager-pgbouncer: TLS disabled (ssl=$SSL_ENABLED)"
    # Remove TLS lines if present
    sed -i '/^client_tls_/d' /etc/pgbouncer/shared/pgbouncer.ini
    sed -i '/^server_tls_/d' /etc/pgbouncer/shared/pgbouncer.ini
    # Remove the comment blocks too
    sed -i '/^; Client TLS/d' /etc/pgbouncer/shared/pgbouncer.ini
    sed -i '/^; Server TLS/d' /etc/pgbouncer/shared/pgbouncer.ini
fi

exec pgbouncer /etc/pgbouncer/shared/pgbouncer.ini
