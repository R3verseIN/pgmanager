import pytest

pytestmark = [pytest.mark.settings]


class TestGetSettings:
    def test_default_settings(self, api, admin_cookies):
        resp = api.get("/api/settings", cookies=admin_cookies)
        assert resp.status_code == 200
        settings = resp.json()
        assert "pgbouncer_pool_mode" in settings
        assert "pgbouncer_default_pool_size" in settings
        assert "pgbouncer_max_client_conn" in settings

    def test_values(self, api, admin_cookies):
        resp = api.get("/api/settings", cookies=admin_cookies)
        settings = resp.json()
        assert settings["pgbouncer_pool_mode"] == "transaction"
        assert settings["pgbouncer_default_pool_size"] == "20"
        assert settings["pgbouncer_max_client_conn"] == "100"


class TestUpdateSettings:
    def test_update(self, api, admin_cookies):
        resp = api.put(
            "/api/settings",
            json={"pgbouncer_pool_mode": "session", "pgbouncer_default_pool_size": "50"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 200

        resp = api.get("/api/settings", cookies=admin_cookies)
        settings = resp.json()
        assert settings["pgbouncer_pool_mode"] == "session"
        assert settings["pgbouncer_default_pool_size"] == "50"

    def test_new_key(self, api, admin_cookies):
        resp = api.put(
            "/api/settings",
            json={"custom_setting": "custom_value"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 200

        resp = api.get("/api/settings", cookies=admin_cookies)
        settings = resp.json()
        assert settings["custom_setting"] == "custom_value"

    def test_invalid_json(self, api, admin_cookies):
        resp = api.put(
            "/api/settings",
            data="not json",
            cookies=admin_cookies,
        )
        assert resp.status_code == 400
