#!/bin/sh
set -e

echo "========================================="
echo "  pgmanager - Reset Admin Password"
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

user_exists=$(psql -v ON_ERROR_STOP=1 -U pgmanager -d pgmanager -t -A -c \
    "SELECT COUNT(*) FROM auth_users WHERE username = '$target_username';")

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
fi

NEW_HASH=$(psql -v ON_ERROR_STOP=1 -U pgmanager -d pgmanager -t -A -c \
    "SELECT crypt('$NEW_PASSWORD', gen_salt('bf'));")

psql -v ON_ERROR_STOP=1 -U pgmanager -d pgmanager <<-EOSQL
    UPDATE auth_users
    SET password_hash = '$NEW_HASH', updated_at = NOW()
    WHERE username = '$target_username';

    DELETE FROM sessions
    WHERE user_id = (SELECT id FROM auth_users WHERE username = '$target_username');
EOSQL

echo ""
echo "========================================="
echo "Password reset successfully"
echo "Username: $target_username"
echo "New password: $NEW_PASSWORD"
echo "========================================="
