#!/bin/sh
set -e

PASSWORD_FILE="/var/lib/postgresql/data/pgmanager-password"
AUTH_PASSWORD_FILE="/var/lib/postgresql/data/pgbouncer-auth-password"

export PGPASSWORD=$(cat "$PASSWORD_FILE")

echo "Enter new PostgreSQL pgmanager password (or leave blank to generate a random secure one): "
read -s custom_password
echo ""

if [ -z "$custom_password" ]; then
    NEW_PASSWORD=$(head -c 32 /dev/urandom | base64 | tr -d '\n=/+' | head -c 24)
else
    NEW_PASSWORD="$custom_password"
    pwlen=${#NEW_PASSWORD}
    if [ "$pwlen" -lt 8 ]; then
        echo "Error: Password must be at least 8 characters."
        exit 1
    fi
    if [ "$pwlen" -gt 72 ]; then
        echo "Error: Password must be at most 72 characters (bcrypt limit)."
        exit 1
    fi
    if ! echo "$NEW_PASSWORD" | grep -qE '^[a-zA-Z0-9_-]+$'; then
        echo "Error: password contains invalid characters (only letters, numbers, underscores, hyphens allowed)."
        exit 1
    fi
fi

echo "$NEW_PASSWORD" > "$PASSWORD_FILE"
chmod 600 "$PASSWORD_FILE"

psql -v ON_ERROR_STOP=1 -U pgmanager -d pgmanager <<-EOSQL
    ALTER USER pgmanager PASSWORD \$\$${NEW_PASSWORD}\$\$;
EOSQL

NEW_AUTH_PASSWORD=$(head -c 32 /dev/urandom | base64 | tr -d '\n=/+' | head -c 24)
echo "$NEW_AUTH_PASSWORD" > "$AUTH_PASSWORD_FILE"
chmod 600 "$AUTH_PASSWORD_FILE"

psql -v ON_ERROR_STOP=1 -U pgmanager -d pgmanager <<-EOSQL
    ALTER USER pgbouncer_auth WITH PASSWORD \$\$${NEW_AUTH_PASSWORD}\$\$;
EOSQL

echo "========================================="
echo "Password reset successfully"
echo "pgmanager password: $NEW_PASSWORD"
echo "pgbouncer_auth password: $NEW_AUTH_PASSWORD"
echo "Restart app and pgbouncer to pick up changes"
echo "========================================="
