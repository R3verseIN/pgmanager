import os
import pytest
import psycopg2

pytestmark = [pytest.mark.sql]

PG_DSN = os.getenv("PG_DSN", "postgresql://pgmanager:pgmanager@db:5432/pgmanager?sslmode=disable")


def setup_db(db_name):
    dsn = PG_DSN.rsplit("/", 1)[0] + "/" + db_name
    conn = psycopg2.connect(dsn)
    conn.autocommit = True
    cur = conn.cursor()
    cur.execute("CREATE TABLE IF NOT EXISTS test_t (id SERIAL PRIMARY KEY, name TEXT)")
    cur.execute("INSERT INTO test_t (name) VALUES ('hello'), ('world')")
    conn.close()


class TestExecuteQuery:
    def test_select(self, api, admin_cookies, test_db):
        setup_db(test_db)
        resp = api.post(
            f"/api/databases/{test_db}/query",
            json={"sql": "SELECT * FROM test_t ORDER BY id"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["rowCount"] == 2
        assert len(data["columns"]) == 2
        assert data["duration"] > 0

    def test_insert(self, api, admin_cookies, test_db):
        dsn = PG_DSN.rsplit("/", 1)[0] + "/" + test_db
        conn = psycopg2.connect(dsn)
        conn.autocommit = True
        conn.cursor().execute("CREATE TABLE IF NOT EXISTS test_t (id SERIAL PRIMARY KEY, name TEXT)")
        conn.close()

        resp = api.post(
            f"/api/databases/{test_db}/query",
            json={"sql": "INSERT INTO test_t (name) VALUES ('new')"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 200

    def test_blocked(self, api, admin_cookies, test_db):
        resp = api.post(
            f"/api/databases/{test_db}/query",
            json={"sql": "DROP DATABASE postgres"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 403

    def test_invalid_sql(self, api, admin_cookies, test_db):
        resp = api.post(
            f"/api/databases/{test_db}/query",
            json={"sql": "SELCT * FROM nonexistent"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["error"] != ""

    def test_empty_sql(self, api, admin_cookies, test_db):
        resp = api.post(
            f"/api/databases/{test_db}/query",
            json={"sql": ""},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400


class TestSQLStatementTimeout:
    def test_slow_query_returns_within_timeout(self, api, admin_cookies, test_db):
        import time
        dsn = PG_DSN.rsplit("/", 1)[0] + "/" + test_db
        conn = psycopg2.connect(dsn)
        conn.autocommit = True
        conn.cursor().execute("CREATE TABLE IF NOT EXISTS timeout_test (id SERIAL PRIMARY KEY, data TEXT)")
        for i in range(1000):
            conn.cursor().execute("INSERT INTO timeout_test (data) VALUES (%s)", (f"row{i}",))
        conn.close()

        start = time.time()
        resp = api.post(
            f"/api/databases/{test_db}/query",
            json={"sql": "SELECT pg_sleep(15)"},
            cookies=admin_cookies,
        )
        elapsed = time.time() - start
        assert resp.status_code == 200
        assert elapsed < 15, f"Query took {elapsed}s, should have been killed by statement_timeout"


class TestSQLInvalidJsonBody:
    def test_invalid_json(self, api, admin_cookies, test_db):
        resp = api.post(
            f"/api/databases/{test_db}/query",
            data="not json",
            cookies=admin_cookies,
        )
        assert resp.status_code == 400
