#!/bin/bash
set -e

echo "Setting up admin user..."
COOKIE_JAR=$(mktemp)
curl -s -c $COOKIE_JAR -X POST http://localhost:8080/api/auth/setup -H "Content-Type: application/json" -d '{"username":"admin","password":"password"}' > /dev/null

echo "Logging in..."
curl -s -c $COOKIE_JAR -X POST http://localhost:8080/api/auth/login -H "Content-Type: application/json" -d '{"username":"admin","password":"password"}' > /dev/null

echo "Applying pgbackrest settings..."
curl -s -b $COOKIE_JAR -X POST http://localhost:8080/api/pgbackrest/settings \
  -H "Content-Type: application/json" \
  -d '{"enabled":true,"archive_timeout":60,"retention_days":7,"full_backup_day":0,"backup_hour":2}'

# Wait a moment for postgres to reload
sleep 5

echo "Waiting for pgbouncer to be ready..."
for i in {1..30}; do
    if psql postgres://pgmanager:pgmanager@localhost:5433/pgmanager?sslmode=disable -c '\q' 2>/dev/null; then
        echo "PgBouncer is ready!"
        break
    fi
    sleep 1
done

echo "Creating test data..."
psql postgres://pgmanager:pgmanager@localhost:5433/pgmanager?sslmode=disable -c "DROP DATABASE IF EXISTS e2e_test WITH (FORCE);"
psql postgres://pgmanager:pgmanager@localhost:5433/pgmanager?sslmode=disable -c "CREATE DATABASE e2e_test;"
psql postgres://pgmanager:pgmanager@localhost:5433/e2e_test?sslmode=disable -c "CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT);"
psql postgres://pgmanager:pgmanager@localhost:5433/e2e_test?sslmode=disable -c "INSERT INTO users (name) VALUES ('Alice');"

echo "Triggering full backup..."
curl -s -b $COOKIE_JAR -X POST http://localhost:8080/api/pgbackrest/trigger -H "Content-Type: application/json" -d '{"type":"full"}'

echo "Adding more test data..."
psql postgres://pgmanager:pgmanager@localhost:5433/e2e_test?sslmode=disable -c "INSERT INTO users (name) VALUES ('Bob');"

echo "Triggering incremental backup..."
curl -s -b $COOKIE_JAR -X POST http://localhost:8080/api/pgbackrest/trigger -H "Content-Type: application/json" -d '{"type":"incr"}'

echo "Dropping table..."
psql postgres://pgmanager:pgmanager@localhost:5433/e2e_test?sslmode=disable -c "DROP TABLE users;"

echo "Triggering restore..."
curl -s -b $COOKIE_JAR -X POST http://localhost:8080/api/pgbackrest/restore -H "Content-Type: application/json" -d '{"database":"e2e_test"}'

echo "Verifying data..."
COUNT=$(psql postgres://pgmanager:pgmanager@localhost:5433/e2e_test?sslmode=disable -t -c "SELECT count(*) FROM users;")
COUNT=$(echo $COUNT | xargs)

if [ "$COUNT" = "2" ]; then
    echo "Restore successful. Count is 2."
else
    echo "Restore failed. Count is $COUNT."
    exit 1
fi

echo "Checking timeout update..."
psql postgres://pgmanager:pgmanager@localhost:5433/e2e_test?sslmode=disable -c "INSERT INTO users (name) VALUES ('Charlie');"

echo "Listing backups..."
curl -s -b $COOKIE_JAR http://localhost:8080/api/pgbackrest/list

rm $COOKIE_JAR
echo ""
echo "E2E Tests Passed!"
