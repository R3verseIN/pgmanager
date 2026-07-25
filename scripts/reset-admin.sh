#!/bin/sh
set -e

PASSWORD_FILE="/var/lib/postgresql/data/pgmanager-password"
export PGPASSWORD=$(cat "$PASSWORD_FILE")

echo "========================================="
echo "  pgmanager - Reset Auth User Password"
echo "========================================="
echo ""

echo "Available users:"
psql -v ON_ERROR_STOP=1 -U pgmanager -d pgmanager -t -A -c \
    "SELECT username || ' (' || role || ')' FROM auth_users ORDER BY id;"

echo ""
echo "Enter username to reset (or 'q' to quit): "
read -r target_username

if [ "$target_username" = "q" ] || [ -z "$target_username" ]; then
    echo "Aborted."
    exit 0
fi

if ! echo "$target_username" | grep -qE '^[a-zA-Z0-9_-]+$'; then
    echo "Error: username contains invalid characters (only letters, numbers, underscores, hyphens allowed)."
    exit 1
fi

user_exists=$(psql -v ON_ERROR_STOP=1 -U pgmanager -d pgmanager -t -A -c \
    "SELECT COUNT(*) FROM auth_users WHERE username = \$\$${target_username}\$\$;")

if [ "$user_exists" = "0" ]; then
    echo "Error: User '$target_username' not found."
    exit 1
fi

echo ""
echo "Enter new password (or leave blank to generate a random secure one): "
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

psql -v ON_ERROR_STOP=1 -U pgmanager -d pgmanager -c "CREATE EXTENSION IF NOT EXISTS pgcrypto;" > /dev/null

NEW_HASH=$(psql -v ON_ERROR_STOP=1 -U pgmanager -d pgmanager -t -A -c \
    "SELECT crypt(\$\$${NEW_PASSWORD}\$\$, gen_salt('bf'));")

psql -v ON_ERROR_STOP=1 -U pgmanager -d pgmanager <<-EOSQL
    UPDATE auth_users
    SET password_hash = \$\$${NEW_HASH}\$\$, updated_at = NOW()
    WHERE username = \$\$${target_username}\$\$;

    DELETE FROM sessions
    WHERE user_id = (SELECT id FROM auth_users WHERE username = \$\$${target_username}\$\$);
EOSQL

echo ""
echo "========================================="
echo "Password reset successfully"
echo "Username: $target_username"
echo "New password: $NEW_PASSWORD"
echo "========================================="
