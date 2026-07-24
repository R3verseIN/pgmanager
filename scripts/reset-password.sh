#!/bin/sh
set -e

PASSWORD_FILE="/var/lib/postgresql/data/pgmanager-password"

NEW_PASSWORD=$(head -c 32 /dev/urandom | base64 | tr -d '\n=/+' | head -c 24)
echo "$NEW_PASSWORD" > "$PASSWORD_FILE"
chmod 600 "$PASSWORD_FILE"

psql -v ON_ERROR_STOP=1 -U pgmanager -d pgmanager <<-EOSQL
    ALTER USER pgmanager PASSWORD '${NEW_PASSWORD}';
EOSQL

echo "========================================="
echo "Password reset successfully"
echo "New password: $NEW_PASSWORD"
echo "========================================="
