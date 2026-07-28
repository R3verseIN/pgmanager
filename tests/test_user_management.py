import os
import pytest
import psycopg2

pytestmark = [pytest.mark.user_management]

PG_DSN = os.getenv("PG_DSN", "postgresql://pgmanager:pgmanager@db:5432/pgmanager?sslmode=disable")


def table_exists(db_name, table_name):
    dsn = PG_DSN.rsplit("/", 1)[0] + "/" + db_name
    conn = psycopg2.connect(dsn)
    conn.autocommit = True
    cur = conn.cursor()
    cur.execute(
        "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = %s)",
        (table_name,),
    )
    result = cur.fetchone()[0]
    conn.close()
    return result


def role_exists(username):
    conn = psycopg2.connect(PG_DSN)
    conn.autocommit = True
    cur = conn.cursor()
    cur.execute("SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = %s)", (username,))
    result = cur.fetchone()[0]
    conn.close()
    return result


class TestAddUserDatabase:
    def test_success(self, api, admin_cookies, test_db):
        api.post(
            "/api/users",
            json={"username": "addbuser", "password": "addbpass1234", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        api.post("/api/databases", json={"name": "adddb2"}, cookies=admin_cookies)
        resp = api.post(
            "/api/users/addbuser/databases",
            json={"database": "adddb2"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 201

        resp = api.get("/api/users", cookies=admin_cookies)
        users = resp.json()
        user = [u for u in users if u["username"] == "addbuser"][0]
        assert "adddb2" in user["databases"]

    def test_nonexistent_user(self, api, admin_cookies, test_db):
        resp = api.post(
            "/api/users/ghost/databases",
            json={"database": test_db},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400

    def test_nonexistent_database(self, api, admin_cookies, test_db):
        api.post(
            "/api/users",
            json={"username": "ghostdb", "password": "ghostdbpass1234", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        resp = api.post(
            "/api/users/ghostdb/databases",
            json={"database": "nonexistent_xyz"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400

    def test_duplicate_database(self, api, admin_cookies, test_db):
        api.post(
            "/api/users",
            json={"username": "dupdb", "password": "dupdbpass1234", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        resp = api.post(
            "/api/users/dupdb/databases",
            json={"database": test_db},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400

    def test_protected_database(self, api, admin_cookies, test_db):
        api.post(
            "/api/users",
            json={"username": "protdb", "password": "protdbpass1234", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        resp = api.post(
            "/api/users/protdb/databases",
            json={"database": "postgres"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400


class TestRemoveUserDatabase:
    def test_success(self, api, admin_cookies, test_db):
        api.post("/api/databases", json={"name": "rmdrb"}, cookies=admin_cookies)
        api.post(
            "/api/users",
            json={"username": "rmdruser", "password": "rmdrpass1234", "access": "read", "databases": [test_db, "rmdrb"]},
            cookies=admin_cookies,
        )
        resp = api.delete(
            "/api/users/rmdruser/databases/rmdrb",
            cookies=admin_cookies,
        )
        assert resp.status_code == 200

        resp = api.get("/api/users", cookies=admin_cookies)
        users = resp.json()
        user = [u for u in users if u["username"] == "rmdruser"][0]
        assert "rmdrb" not in user["databases"]

    def test_last_database_drops_role(self, api, admin_cookies, test_db):
        api.post(
            "/api/users",
            json={"username": "lastdb", "password": "lastdbpass1234", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        assert role_exists("lastdb")

        resp = api.delete(
            f"/api/users/lastdb/databases/{test_db}",
            cookies=admin_cookies,
        )
        assert resp.status_code == 200

        resp = api.get("/api/users", cookies=admin_cookies)
        users = resp.json()
        usernames = [u["username"] for u in users]
        assert "lastdb" not in usernames


class TestUpdateUserAccess:
    def test_change_access_level(self, api, admin_cookies, test_db):
        api.post(
            "/api/users",
            json={"username": "uplevel", "password": "uplevelpass1234", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        resp = api.put(
            "/api/users/uplevel",
            json={"access": "write"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 200

        resp = api.get("/api/users", cookies=admin_cookies)
        users = resp.json()
        user = [u for u in users if u["username"] == "uplevel"][0]
        assert user["access"] == "write"

    def test_change_databases(self, api, admin_cookies, test_db):
        api.post("/api/databases", json={"name": "newdbaccess"}, cookies=admin_cookies)
        api.post(
            "/api/users",
            json={"username": "updb", "password": "updbpass1234", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        resp = api.put(
            "/api/users/updb",
            json={"databases": ["newdbaccess"]},
            cookies=admin_cookies,
        )
        assert resp.status_code == 200

        resp = api.get("/api/users", cookies=admin_cookies)
        users = resp.json()
        user = [u for u in users if u["username"] == "updb"][0]
        assert "newdbaccess" in user["databases"]
        assert test_db not in user["databases"]

    def test_generate_password(self, api, admin_cookies, test_db):
        api.post(
            "/api/users",
            json={"username": "genpw", "password": "genpwpass1234", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        resp = api.put(
            "/api/users/genpw",
            json={"generatePassword": True},
            cookies=admin_cookies,
        )
        assert resp.status_code == 200
        data = resp.json()
        assert "password" in data
        assert len(data["password"]) == 16

    def test_nonexistent_user(self, api, admin_cookies, test_db):
        resp = api.put(
            "/api/users/ghostuser",
            json={"access": "full"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400


class TestDeleteUser:
    def test_deletes_pg_role(self, api, admin_cookies, test_db):
        api.post(
            "/api/users",
            json={"username": "delrole", "password": "delpass1234", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        assert role_exists("delrole")

        resp = api.delete("/api/users/delrole", cookies=admin_cookies)
        assert resp.status_code == 200
        assert not role_exists("delrole")

    def test_removes_managed_metadata(self, api, admin_cookies, test_db):
        api.post(
            "/api/users",
            json={"username": "delmeta", "password": "delpass1234", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        resp = api.delete("/api/users/delmeta", cookies=admin_cookies)
        assert resp.status_code == 200

        resp = api.get("/api/users", cookies=admin_cookies)
        users = resp.json()
        usernames = [u["username"] for u in users]
        assert "delmeta" not in usernames
