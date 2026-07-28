import os
import pytest
import psycopg2

pytestmark = [pytest.mark.tables]

PG_DSN = os.getenv("PG_DSN", "postgresql://pgmanager:pgmanager@db:5432/pgmanager?sslmode=disable")


def create_table(pg_cursor, db_name, table_name):
    pg_cursor.execute(f"""
        SELECT 1 FROM pg_database WHERE datname = '{db_name}'
    """)
    if not pg_cursor.fetchone():
        pg_cursor.execute(f"CREATE DATABASE {db_name}")
    dsn = PG_DSN.rsplit("/", 1)[0] + "/" + db_name
    conn = psycopg2.connect(dsn)
    conn.autocommit = True
    cur = conn.cursor()
    cur.execute(f"CREATE TABLE IF NOT EXISTS {table_name} (id SERIAL PRIMARY KEY, name TEXT NOT NULL, value INTEGER DEFAULT 0)")
    conn.close()


def insert_data(db_name, table_name, count):
    dsn = PG_DSN.rsplit("/", 1)[0] + "/" + db_name
    conn = psycopg2.connect(dsn)
    conn.autocommit = True
    cur = conn.cursor()
    for i in range(count):
        cur.execute(f"INSERT INTO {table_name} (name, value) VALUES (%s, %s)", (f"item{i}", i * 10))
    conn.close()


class TestListTables:
    def test_list(self, api, admin_cookies, test_db, pg_cursor):
        create_table(pg_cursor, test_db, "users")
        create_table(pg_cursor, test_db, "orders")
        insert_data(test_db, "users", 5)

        resp = api.get(f"/api/databases/{test_db}/tables", cookies=admin_cookies)
        assert resp.status_code == 200
        tables = resp.json()
        names = [t["name"] for t in tables]
        assert "users" in names
        assert "orders" in names

    def test_empty(self, api, admin_cookies, test_db):
        resp = api.get(f"/api/databases/{test_db}/tables", cookies=admin_cookies)
        assert resp.status_code == 200
        tables = resp.json()
        assert len(tables) == 0


class TestGetColumns:
    def test_columns(self, api, admin_cookies, test_db, pg_cursor):
        create_table(pg_cursor, test_db, "items")
        resp = api.get(f"/api/databases/{test_db}/columns/items", cookies=admin_cookies)
        assert resp.status_code == 200
        cols = resp.json()
        col_names = [c["name"] for c in cols]
        assert "id" in col_names
        assert "name" in col_names
        assert "value" in col_names


class TestCreateTable:
    def test_success(self, api, admin_cookies, test_db):
        resp = api.post(
            f"/api/databases/{test_db}/tables",
            json={
                "name": "newtable",
                "columns": [
                    {"name": "id", "type": "SERIAL", "isPrimaryKey": True},
                    {"name": "title", "type": "TEXT", "nullable": False},
                ],
            },
            cookies=admin_cookies,
        )
        assert resp.status_code == 201

    def test_duplicate(self, api, admin_cookies, test_db, pg_cursor):
        create_table(pg_cursor, test_db, "existing")
        resp = api.post(
            f"/api/databases/{test_db}/tables",
            json={"name": "existing", "columns": [{"name": "id", "type": "SERIAL"}]},
            cookies=admin_cookies,
        )
        assert resp.status_code == 409


class TestAddColumn:
    def test_success(self, api, admin_cookies, test_db, pg_cursor):
        create_table(pg_cursor, test_db, "items")
        resp = api.post(
            f"/api/databases/{test_db}/tables/items/columns",
            json={"name": "description", "type": "TEXT", "nullable": True},
            cookies=admin_cookies,
        )
        assert resp.status_code == 201


class TestDropColumn:
    def test_success(self, api, admin_cookies, test_db, pg_cursor):
        create_table(pg_cursor, test_db, "items")
        resp = api.delete(
            f"/api/databases/{test_db}/tables/items/columns/value",
            cookies=admin_cookies,
        )
        assert resp.status_code == 200

    def test_primary_key(self, api, admin_cookies, test_db, pg_cursor):
        create_table(pg_cursor, test_db, "items")
        resp = api.delete(
            f"/api/databases/{test_db}/tables/items/columns/id",
            cookies=admin_cookies,
        )
        assert resp.status_code == 400
