import pytest

pytestmark = [pytest.mark.pgbouncer]


class TestPgBouncerDatabases:
    def test_list(self, api, admin_cookies, test_db):
        resp = api.get("/api/pgbouncer/databases", cookies=admin_cookies)
        assert resp.status_code == 200
        dbs = resp.json()
        assert len(dbs) >= 1
        names = [d["databaseName"] for d in dbs]
        assert "pgmanager" in names

    def test_toggle_allowed(self, api, admin_cookies, test_db):
        resp = api.put(
            f"/api/pgbouncer/databases/{test_db}",
            json={"allowed": True},
            cookies=admin_cookies,
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["databaseName"] == test_db
        assert data["allowed"] is True

        resp = api.get("/api/pgbouncer/databases", cookies=admin_cookies)
        dbs = resp.json()
        test_entry = [d for d in dbs if d["databaseName"] == test_db]
        assert len(test_entry) == 1
        assert test_entry[0]["allowed"] is True

    def test_toggle_not_found(self, api, admin_cookies):
        resp = api.put(
            "/api/pgbouncer/databases/nonexistent_xyz",
            json={"allowed": True},
            cookies=admin_cookies,
        )
        assert resp.status_code == 404


class TestPgBouncerConfig:
    def test_get_default(self, api, admin_cookies):
        resp = api.get("/api/pgbouncer/config", cookies=admin_cookies)
        assert resp.status_code == 200
        config = resp.json()
        assert config["poolMode"] == "transaction"
        assert config["defaultPoolSize"] == 20
        assert config["maxClientConn"] == 100

    def test_update_config(self, api, admin_cookies):
        resp = api.put(
            "/api/pgbouncer/config",
            json={"poolMode": "session", "defaultPoolSize": 50, "maxClientConn": 200},
            cookies=admin_cookies,
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["poolMode"] == "session"
        assert data["defaultPoolSize"] == 50

        resp = api.get("/api/pgbouncer/config", cookies=admin_cookies)
        config = resp.json()
        assert config["poolMode"] == "session"
        assert config["defaultPoolSize"] == 50
        assert config["maxClientConn"] == 200

    def test_invalid_pool_mode(self, api, admin_cookies):
        resp = api.put(
            "/api/pgbouncer/config",
            json={"poolMode": "invalid", "defaultPoolSize": 20, "maxClientConn": 100},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400

    def test_invalid_pool_size(self, api, admin_cookies):
        resp = api.put(
            "/api/pgbouncer/config",
            json={"poolMode": "transaction", "defaultPoolSize": 0, "maxClientConn": 100},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400

    def test_invalid_max_client_conn(self, api, admin_cookies):
        resp = api.put(
            "/api/pgbouncer/config",
            json={"poolMode": "transaction", "defaultPoolSize": 20, "maxClientConn": 0},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400

    def test_invalid_json(self, api, admin_cookies):
        resp = api.put(
            "/api/pgbouncer/config",
            data="not json",
            cookies=admin_cookies,
        )
        assert resp.status_code == 400
