import pytest

pytestmark = [pytest.mark.auth]


class TestSetupCheck:
    def test_needs_setup(self, api):
        resp = api.get("/api/auth/setup-check")
        assert resp.status_code == 200
        data = resp.json()
        assert data["needsSetup"] is True

    def test_after_setup(self, api, admin_cookies):
        resp = api.get("/api/auth/setup-check", cookies=admin_cookies)
        assert resp.status_code == 200
        data = resp.json()
        assert data["needsSetup"] is False


class TestSetup:
    def test_create_admin(self, api):
        resp = api.post("/api/auth/setup", json={"username": "admin", "password": "admin1234"})
        assert resp.status_code == 201

    def test_duplicate_setup(self, api, admin_cookies):
        resp = api.post(
            "/api/auth/setup",
            json={"username": "admin2", "password": "admin1234"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 409

    def test_short_username(self, api):
        resp = api.post("/api/auth/setup", json={"username": "ab", "password": "admin1234"})
        assert resp.status_code == 400

    def test_short_password(self, api):
        resp = api.post("/api/auth/setup", json={"username": "admin", "password": "short"})
        assert resp.status_code == 400


class TestLogin:
    def test_success(self, api):
        api.post("/api/auth/setup", json={"username": "admin", "password": "admin1234"})
        resp = api.post("/api/auth/login", json={"username": "admin", "password": "admin1234"})
        assert resp.status_code == 200
        assert "session_id" in resp.cookies

    def test_bad_password(self, api):
        api.post("/api/auth/setup", json={"username": "admin", "password": "admin1234"})
        resp = api.post("/api/auth/login", json={"username": "admin", "password": "wrong"})
        assert resp.status_code == 401

    def test_nonexistent_user(self, api):
        api.post("/api/auth/setup", json={"username": "admin", "password": "admin1234"})
        resp = api.post("/api/auth/login", json={"username": "nobody", "password": "password1234"})
        assert resp.status_code == 401


class TestGetMe:
    def test_authenticated(self, api, admin_cookies):
        resp = api.get("/api/auth/me", cookies=admin_cookies)
        assert resp.status_code == 200
        data = resp.json()
        assert data["username"] == "admin"
        assert data["role"] == "admin"

    def test_unauthenticated(self, api):
        resp = api.get("/api/auth/me")
        assert resp.status_code == 401


class TestChangePassword:
    def test_success(self, api, admin_cookies):
        resp = api.put(
            "/api/auth/password",
            json={"current_password": "admin1234", "new_password": "newpass1234"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 200
        resp = api.post("/api/auth/login", json={"username": "admin", "password": "newpass1234"})
        assert resp.status_code == 200

    def test_wrong_current(self, api, admin_cookies):
        resp = api.put(
            "/api/auth/password",
            json={"current_password": "wrong", "new_password": "newpass1234"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 401


class TestLogout:
    def test_success(self, api, admin_cookies):
        resp = api.post("/api/auth/logout", cookies=admin_cookies)
        assert resp.status_code == 200


class TestCreateAuthUser:
    def test_success(self, api, admin_cookies):
        resp = api.post(
            "/api/auth/users",
            json={"username": "viewer1", "password": "viewer1234", "role": "viewer"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 201

    def test_duplicate(self, api, admin_cookies):
        api.post(
            "/api/auth/users",
            json={"username": "viewer1", "password": "viewer1234", "role": "viewer"},
            cookies=admin_cookies,
        )
        resp = api.post(
            "/api/auth/users",
            json={"username": "viewer1", "password": "viewer1234", "role": "viewer"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 409

    def test_invalid_role(self, api, admin_cookies):
        resp = api.post(
            "/api/auth/users",
            json={"username": "user1", "password": "user12345", "role": "superadmin"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400


class TestListAuthUsers:
    def test_list(self, api, admin_cookies, test_db):
        api.post(
            "/api/auth/users",
            json={"username": "viewer1", "password": "viewer1234", "role": "viewer"},
            cookies=admin_cookies,
        )
        resp = api.get("/api/auth/users", cookies=admin_cookies)
        assert resp.status_code == 200
        users = resp.json()
        assert len(users) >= 2


class TestDeleteAuthUser:
    def test_success(self, api, admin_cookies):
        api.post(
            "/api/auth/users",
            json={"username": "viewer1", "password": "viewer1234", "role": "viewer"},
            cookies=admin_cookies,
        )
        resp = api.delete("/api/auth/users/viewer1", cookies=admin_cookies)
        assert resp.status_code == 200

    def test_last_admin(self, api, admin_cookies):
        resp = api.delete("/api/auth/users/admin", cookies=admin_cookies)
        assert resp.status_code == 400
