import pytest

pytestmark = [pytest.mark.auth_management]


class TestResetPassword:
    def test_success(self, api, admin_cookies):
        api.post(
            "/api/auth/users",
            json={"username": "resetuser", "password": "oldpass1234", "role": "viewer"},
            cookies=admin_cookies,
        )
        resp = api.post(
            "/api/auth/users/resetuser/reset-password",
            json={"password": "newpass1234"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["password"] == "newpass1234"

        resp = api.post("/api/auth/login", json={"username": "resetuser", "password": "newpass1234"})
        assert resp.status_code == 200

    def test_auto_generate(self, api, admin_cookies):
        api.post(
            "/api/auth/users",
            json={"username": "autogen", "password": "oldpass1234", "role": "viewer"},
            cookies=admin_cookies,
        )
        resp = api.post(
            "/api/auth/users/autogen/reset-password",
            json={},
            cookies=admin_cookies,
        )
        assert resp.status_code == 200
        data = resp.json()
        assert len(data["password"]) == 16

    def test_nonexistent_user(self, api, admin_cookies):
        resp = api.post(
            "/api/auth/users/nobody/reset-password",
            json={"password": "newpass1234"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 404

    def test_short_password(self, api, admin_cookies):
        api.post(
            "/api/auth/users",
            json={"username": "shortpw", "password": "oldpass1234", "role": "viewer"},
            cookies=admin_cookies,
        )
        resp = api.post(
            "/api/auth/users/shortpw/reset-password",
            json={"password": "short"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400


class TestUpdateAuthUser:
    def test_change_role(self, api, admin_cookies, test_db):
        api.post(
            "/api/auth/users",
            json={"username": "promote", "password": "prompass1234", "role": "viewer"},
            cookies=admin_cookies,
        )
        resp = api.put(
            "/api/auth/users/promote",
            json={"role": "dev", "databases": [test_db]},
            cookies=admin_cookies,
        )
        assert resp.status_code == 200

        resp = api.get("/api/auth/users", cookies=admin_cookies)
        users = resp.json()
        promote_user = [u for u in users if u["username"] == "promote"]
        assert len(promote_user) == 1
        assert promote_user[0]["role"] == "dev"

    def test_cannot_change_own_role(self, api, admin_cookies):
        resp = api.put(
            "/api/auth/users/admin",
            json={"role": "viewer"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400

    def test_cannot_delete_own_account(self, api, admin_cookies):
        resp = api.delete("/api/auth/users/admin", cookies=admin_cookies)
        assert resp.status_code == 400

    def test_cannot_change_last_admin_role(self, api, admin_cookies):
        resp = api.put(
            "/api/auth/users/admin",
            json={"role": "viewer"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400

    def test_dev_requires_databases(self, api, admin_cookies):
        api.post(
            "/api/auth/users",
            json={"username": "devnodev", "password": "devpass1234", "role": "viewer"},
            cookies=admin_cookies,
        )
        resp = api.put(
            "/api/auth/users/devnodev",
            json={"role": "dev"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400

    def test_nonexistent_user(self, api, admin_cookies):
        resp = api.put(
            "/api/auth/users/ghost",
            json={"role": "admin"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 404
