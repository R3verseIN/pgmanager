import os
import pytest
import psycopg2
import requests

pytestmark = [pytest.mark.dev_isolation]

PG_DSN = os.getenv("PG_DSN", "postgresql://pgmanager:pgmanager@db:5432/pgmanager?sslmode=disable")
BASE_URL = os.getenv("PGMANAGER_URL", "http://app:8080")


def create_db_and_table(db_name):
    conn = psycopg2.connect(PG_DSN)
    conn.autocommit = True
    cur = conn.cursor()
    cur.execute(f"SELECT 1 FROM pg_database WHERE datname = '{db_name}'")
    if not cur.fetchone():
        cur.execute(f"CREATE DATABASE {db_name}")
    conn.close()

    dsn = PG_DSN.rsplit("/", 1)[0] + "/" + db_name
    conn = psycopg2.connect(dsn)
    conn.autocommit = True
    cur = conn.cursor()
    cur.execute("CREATE TABLE IF NOT EXISTS dev_test (id SERIAL PRIMARY KEY, name TEXT)")
    cur.execute("INSERT INTO dev_test (name) VALUES ('data1'), ('data2')")
    conn.close()


def login_as(username, password):
    session = requests.Session()
    session.headers.update({"Content-Type": "application/json"})
    resp = session.post(f"{BASE_URL}/api/auth/login", json={"username": username, "password": password})
    assert resp.status_code == 200, f"login failed: {resp.text}"
    return session


class TestDevUserIsolation:
    def test_dev_user_sees_only_assigned_databases(self, api, admin_cookies, test_db):
        create_db_and_table(test_db)
        create_db_and_table("other_db")

        resp = api.post(
            "/api/auth/users",
            json={"username": "devuser", "password": "devpass1234", "role": "dev", "databases": [test_db]},
            cookies=admin_cookies,
        )
        assert resp.status_code == 201
        dev_password = resp.json()["password"]

        dev_session = login_as("devuser", dev_password)

        resp = dev_session.get(f"{BASE_URL}/api/databases")
        assert resp.status_code == 200
        dbs = resp.json()
        db_names = [d["name"] for d in dbs]
        assert test_db in db_names
        assert "other_db" not in db_names

        resp = dev_session.get(f"{BASE_URL}/api/databases/{test_db}/tables")
        assert resp.status_code == 200

        resp = dev_session.get(f"{BASE_URL}/api/databases/other_db/tables")
        assert resp.status_code == 403

    def test_viewer_role_cannot_write(self, api, admin_cookies, test_db):
        create_db_and_table(test_db)

        resp = api.post(
            "/api/auth/users",
            json={"username": "vieweruser", "password": "viewerpass123", "role": "viewer"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 201

        viewer_session = login_as("vieweruser", "viewerpass123")

        resp = viewer_session.get(f"{BASE_URL}/api/databases/{test_db}/tables")
        assert resp.status_code == 200

        resp = viewer_session.post(
            f"{BASE_URL}/api/databases/{test_db}/data/dev_test",
            json={"values": {"name": "should_fail"}},
        )
        assert resp.status_code == 403

    def test_admin_can_access_all(self, api, admin_cookies, test_db):
        create_db_and_table(test_db)

        resp = api.get(f"/api/databases/{test_db}/tables", cookies=admin_cookies)
        assert resp.status_code == 200

        resp = api.post(
            f"/api/databases/{test_db}/data/dev_test",
            json={"values": {"name": "admin_row"}},
            cookies=admin_cookies,
        )
        assert resp.status_code == 201

    def test_dev_user_can_write_to_assigned_database(self, api, admin_cookies, test_db):
        create_db_and_table(test_db)

        resp = api.post(
            "/api/auth/users",
            json={"username": "devwriter", "password": "devpass1234", "role": "dev", "databases": [test_db]},
            cookies=admin_cookies,
        )
        assert resp.status_code == 201
        dev_password = resp.json()["password"]

        dev_session = login_as("devwriter", dev_password)

        resp = dev_session.post(
            f"{BASE_URL}/api/databases/{test_db}/data/dev_test",
            json={"values": {"name": "dev_inserted"}},
        )
        assert resp.status_code == 201
