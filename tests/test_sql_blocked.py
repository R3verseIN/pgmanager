import os
import pytest
import psycopg2

pytestmark = [pytest.mark.sql]

PG_DSN = os.getenv("PG_DSN", "postgresql://pgmanager:pgmanager@db:5432/pgmanager?sslmode=disable")


def setup_table(db_name):
    dsn = PG_DSN.rsplit("/", 1)[0] + "/" + db_name
    conn = psycopg2.connect(dsn)
    conn.autocommit = True
    cur = conn.cursor()
    cur.execute("CREATE TABLE IF NOT EXISTS block_test (id SERIAL PRIMARY KEY, name TEXT)")
    conn.close()


class TestBlockedSQL:
    def test_drop_database(self, api, admin_cookies, test_db):
        resp = api.post(
            f"/api/databases/{test_db}/query",
            json={"sql": "DROP DATABASE postgres"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 403
        assert "not allowed" in resp.json()["error"]

    def test_drop_owned_by(self, api, admin_cookies, test_db):
        resp = api.post(
            f"/api/databases/{test_db}/query",
            json={"sql": "DROP OWNED BY someuser CASCADE"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 403

    def test_alter_role(self, api, admin_cookies, test_db):
        resp = api.post(
            f"/api/databases/{test_db}/query",
            json={"sql": "ALTER ROLE someuser WITH SUPERUSER"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 403

    def test_create_role(self, api, admin_cookies, test_db):
        resp = api.post(
            f"/api/databases/{test_db}/query",
            json={"sql": "CREATE ROLE eviluser WITH LOGIN"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 403

    def test_drop_role(self, api, admin_cookies, test_db):
        resp = api.post(
            f"/api/databases/{test_db}/query",
            json={"sql": "DROP ROLE someuser"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 403

    def test_grant(self, api, admin_cookies, test_db):
        resp = api.post(
            f"/api/databases/{test_db}/query",
            json={"sql": "GRANT ALL ON DATABASE testdb TO someuser"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 403

    def test_revoke(self, api, admin_cookies, test_db):
        resp = api.post(
            f"/api/databases/{test_db}/query",
            json={"sql": "REVOKE ALL ON DATABASE testdb FROM someuser"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 403

    def test_truncate(self, api, admin_cookies, test_db):
        setup_table(test_db)
        resp = api.post(
            f"/api/databases/{test_db}/query",
            json={"sql": "TRUNCATE block_test"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 403

    def test_comment_on_database(self, api, admin_cookies, test_db):
        resp = api.post(
            f"/api/databases/{test_db}/query",
            json={"sql": "COMMENT ON DATABASE testdb IS 'evil'"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 403

    def test_blocked_case_insensitive(self, api, admin_cookies, test_db):
        resp = api.post(
            f"/api/databases/{test_db}/query",
            json={"sql": "drop database postgres"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 403

    def test_blocked_with_leading_whitespace(self, api, admin_cookies, test_db):
        resp = api.post(
            f"/api/databases/{test_db}/query",
            json={"sql": "   \n  DROP DATABASE postgres"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 403


class TestQueryTypes:
    def test_with_query(self, api, admin_cookies, test_db):
        setup_table(test_db)
        resp = api.post(
            f"/api/databases/{test_db}/query",
            json={"sql": "WITH cte AS (SELECT * FROM block_test) SELECT * FROM cte"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 200
        data = resp.json()
        assert "columns" in data

    def test_explain_query(self, api, admin_cookies, test_db):
        setup_table(test_db)
        resp = api.post(
            f"/api/databases/{test_db}/query",
            json={"sql": "EXPLAIN SELECT * FROM block_test"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["rowCount"] >= 1

    def test_show_query(self, api, admin_cookies, test_db):
        resp = api.post(
            f"/api/databases/{test_db}/query",
            json={"sql": "SHOW server_version"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["rowCount"] == 1

    def test_insert_returns_rows_affected(self, api, admin_cookies, test_db):
        setup_table(test_db)
        resp = api.post(
            f"/api/databases/{test_db}/query",
            json={"sql": "INSERT INTO block_test (name) VALUES ('test1'), ('test2')"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["columns"] == ["rows_affected"]
        assert data["rows"][0][0] == 2

    def test_invalid_json_body(self, api, admin_cookies, test_db):
        resp = api.post(
            f"/api/databases/{test_db}/query",
            data="not json",
            cookies=admin_cookies,
        )
        assert resp.status_code == 400
