#!/bin/sh
set -eu

echo "pgmanager-pgbouncer: reading passwords from files..."

PGMGR_PASS=$(cat /var/lib/postgresql/data/pgmanager-password)
AUTH_PASS=$(cat /var/lib/postgresql/data/pgbouncer-auth-password)

printf '"pgbouncer_auth" "%s"\n"pgmanager" "%s"\n' "$AUTH_PASS" "$PGMGR_PASS" \
    > /etc/pgbouncer/userlist.txt

echo "pgmanager-pgbouncer: userlist.txt generated"
cat /etc/pgbouncer/userlist.txt

exec pgbouncer /etc/pgbouncer/pgbouncer.ini
