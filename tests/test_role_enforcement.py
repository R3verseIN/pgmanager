import os
import pytest
import psycopg2

pytestmark = [pytest.mark.access_control]

PG_DSN = os.getenv("PG_DSN", "postgresql://pgmanager:pgmanager@db:5432/pgmanager?sslmode=disable")


def setup_table(db_name):
    dsn = PG_DSN.rsplit("/", 1)[0] + "/" + db_name
    conn = psycopg2.connect(dsn)
    conn.autocommit = True
    cur = conn.cursor()
    cur.execute("""
        CREATE TABLE IF NOT EXISTS role_test (
            id SERIAL PRIMARY KEY,
            name TEXT NOT NULL
        )
    """)
    cur.execute("INSERT INTO role_test (name) VALUES ('a'), ('b'), ('c')")
    conn.close()


def create_viewer_session(api, admin_cookies, username, password):
    resp = api.post(
        "/api/auth/users",
        json={"username": username, "password": password, "role": "viewer"},
        cookies=admin_cookies,
    )
    assert resp.status_code == 201
    resp = api.post("/api/auth/login", json={"username": username, "password": password})
    return {"session_id": resp.cookies.get("session_id")}


def create_dev_session(api, admin_cookies, username, password, databases):
    resp = api.post(
        "/api/auth/users",
        json={"username": username, "password": password, "role": "dev", "databases": databases},
        cookies=admin_cookies,
    )
    assert resp.status_code == 201
    resp = api.post("/api/auth/login", json={"username": username, "password": password})
    return {"session_id": resp.cookies.get("session_id")}


class TestViewerAdminEndpointEnforcement:
    def test_viewer_cannot_create_database(self, api, admin_cookies, test_db):
        cookies = create_viewer_session(api, admin_cookies, "viewdb1", "viewdb11234")
        resp = api.post("/api/databases", json={"name": "shouldfail"}, cookies=cookies)
        assert resp.status_code == 403

    def test_viewer_cannot_delete_database(self, api, admin_cookies, test_db):
        cookies = create_viewer_session(api, admin_cookies, "viewdb2", "viewdb21234")
        resp = api.delete("/api/databases/testdb", cookies=cookies)
        assert resp.status_code == 403

    def test_viewer_cannot_create_user(self, api, admin_cookies, test_db):
        cookies = create_viewer_session(api, admin_cookies, "viewusr1", "viewusr11234")
        resp = api.post(
            "/api/users",
            json={"username": "failuser", "password": "failpass1234", "access": "read", "databases": [test_db]},
            cookies=cookies,
        )
        assert resp.status_code == 403

    def test_viewer_cannot_update_settings(self, api, admin_cookies, test_db):
        cookies = create_viewer_session(api, admin_cookies, "viewset1", "viewset11234")
        resp = api.put(
            "/api/settings",
            json={"pgbouncer_pool_mode": "session"},
            cookies=cookies,
        )
        assert resp.status_code == 403

    def test_viewer_cannot_manage_pgbouncer(self, api, admin_cookies, test_db):
        cookies = create_viewer_session(api, admin_cookies, "viewpgb1", "viewpgb11234")
        resp = api.get("/api/pgbouncer/databases", cookies=cookies)
        assert resp.status_code == 403

    def test_viewer_cannot_manage_auth_users(self, api, admin_cookies, test_db):
        cookies = create_viewer_session(api, admin_cookies, "viewauth1", "viewauth11234")
        resp = api.get("/api/auth/users", cookies=cookies)
        assert resp.status_code == 403


class TestDevAdminEndpointEnforcement:
    def test_dev_cannot_create_database(self, api, admin_cookies, test_db):
        cookies = create_dev_session(api, admin_cookies, "devdb1", "devdb11234", [test_db])
        resp = api.post("/api/databases", json={"name": "shouldfail"}, cookies=cookies)
        assert resp.status_code == 403

    def test_dev_cannot_delete_database(self, api, admin_cookies, test_db):
        cookies = create_dev_session(api, admin_cookies, "devdb2", "devdb21234", [test_db])
        resp = api.delete("/api/databases/testdb", cookies=cookies)
        assert resp.status_code == 403

    def test_dev_cannot_create_user(self, api, admin_cookies, test_db):
        cookies = create_dev_session(api, admin_cookies, "devusr1", "devusr11234", [test_db])
        resp = api.post(
            "/api/users",
            json={"username": "failuser", "password": "failpass1234", "access": "read", "databases": [test_db]},
            cookies=cookies,
        )
        assert resp.status_code == 403

    def test_dev_cannot_update_settings(self, api, admin_cookies, test_db):
        cookies = create_dev_session(api, admin_cookies, "devset1", "devset11234", [test_db])
        resp = api.put(
            "/api/settings",
            json={"pgbouncer_pool_mode": "session"},
            cookies=cookies,
        )
        assert resp.status_code == 403

    def test_dev_cannot_manage_pgbouncer(self, api, admin_cookies, test_db):
        cookies = create_dev_session(api, admin_cookies, "devpb1", "devpb11234", [test_db])
        resp = api.get("/api/pgbouncer/databases", cookies=cookies)
        assert resp.status_code == 403

    def test_dev_can_read_data_on_assigned_db(self, api, admin_cookies, test_db):
        dsn = PG_DSN.rsplit("/", 1)[0] + "/" + test_db
        conn = psycopg2.connect(dsn)
        conn.autocommit = True
        conn.cursor().execute("CREATE TABLE IF NOT EXISTS dev_test (id SERIAL PRIMARY KEY)")
        conn.close()

        cookies = create_dev_session(api, admin_cookies, "devrd1", "devrd11234", [test_db])
        resp = api.get(f"/api/databases/{test_db}/data/dev_test", cookies=cookies)
        assert resp.status_code == 200

    def test_dev_can_execute_sql_on_assigned_db(self, api, admin_cookies, test_db):
        dsn = PG_DSN.rsplit("/", 1)[0] + "/" + test_db
        conn = psycopg2.connect(dsn)
        conn.autocommit = True
        conn.cursor().execute("CREATE TABLE IF NOT EXISTS dev_sql_test (id SERIAL PRIMARY KEY)")
        conn.close()

        cookies = create_dev_session(api, admin_cookies, "devsql1", "devsql11234", [test_db])
        resp = api.post(
            f"/api/databases/{test_db}/query",
            json={"sql": "SELECT * FROM dev_sql_test"},
            cookies=cookies,
        )
        assert resp.status_code == 200

    def test_dev_cannot_access_unassigned_db(self, api, admin_cookies, test_db):
        api.post("/api/databases", json={"name": "otherdb"}, cookies=admin_cookies)
        cookies = create_dev_session(api, admin_cookies, "devnoa1", "devnoa11234", [test_db])
        resp = api.get("/api/databases/otherdb/data/nonexistent", cookies=cookies)
        assert resp.status_code == 403
