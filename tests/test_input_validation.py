import pytest

pytestmark = [pytest.mark.databases]


class TestDatabaseNameValidation:
    def test_empty_name(self, api, admin_cookies):
        resp = api.post("/api/databases", json={"name": ""}, cookies=admin_cookies)
        assert resp.status_code == 400

    def test_name_too_long(self, api, admin_cookies):
        resp = api.post("/api/databases", json={"name": "a" * 64}, cookies=admin_cookies)
        assert resp.status_code == 400

    def test_name_starts_with_digit(self, api, admin_cookies):
        resp = api.post("/api/databases", json={"name": "1badname"}, cookies=admin_cookies)
        assert resp.status_code == 400

    def test_name_with_special_chars(self, api, admin_cookies):
        resp = api.post("/api/databases", json={"name": "bad!name"}, cookies=admin_cookies)
        assert resp.status_code == 400

    def test_name_with_spaces(self, api, admin_cookies):
        resp = api.post("/api/databases", json={"name": "bad name"}, cookies=admin_cookies)
        assert resp.status_code == 400

    def test_invalid_json_body(self, api, admin_cookies):
        resp = api.post("/api/databases", data="not json", cookies=admin_cookies)
        assert resp.status_code == 400


class TestUserValidation:
    def test_empty_username(self, api, admin_cookies, test_db):
        resp = api.post(
            "/api/users",
            json={"username": "", "password": "testpass1234", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400

    def test_username_too_long(self, api, admin_cookies, test_db):
        resp = api.post(
            "/api/users",
            json={"username": "u" * 64, "password": "testpass1234", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400

    def test_username_starts_with_digit(self, api, admin_cookies, test_db):
        resp = api.post(
            "/api/users",
            json={"username": "1baduser", "password": "testpass1234", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400

    def test_password_too_short(self, api, admin_cookies, test_db):
        resp = api.post(
            "/api/users",
            json={"username": "shortpw", "password": "short", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400

    def test_password_too_long(self, api, admin_cookies, test_db):
        resp = api.post(
            "/api/users",
            json={"username": "longpw", "password": "x" * 129, "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400

    def test_protected_database_in_list(self, api, admin_cookies, test_db):
        resp = api.post(
            "/api/users",
            json={"username": "protdbusr", "password": "testpass1234", "access": "read", "databases": ["postgres"]},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400

    def test_nonexistent_database_in_list(self, api, admin_cookies, test_db):
        resp = api.post(
            "/api/users",
            json={"username": "nodbusr", "password": "testpass1234", "access": "read", "databases": ["nonexistent_xyz"]},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400

    def test_invalid_access_level(self, api, admin_cookies, test_db):
        resp = api.post(
            "/api/users",
            json={"username": "badaccess", "password": "testpass1234", "access": "superadmin", "databases": [test_db]},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400

    def test_invalid_json_body(self, api, admin_cookies):
        resp = api.post("/api/users", data="not json", cookies=admin_cookies)
        assert resp.status_code == 400

    def test_no_databases(self, api, admin_cookies):
        resp = api.post(
            "/api/users",
            json={"username": "nodbs", "password": "testpass1234", "access": "read", "databases": []},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400


class TestAuthValidation:
    def test_setup_invalid_json(self, api):
        resp = api.post("/api/auth/setup", data="not json")
        assert resp.status_code == 400

    def test_login_invalid_json(self, api):
        resp = api.post("/api/auth/login", data="not json")
        assert resp.status_code == 400

    def test_login_empty_username(self, api):
        resp = api.post("/api/auth/login", json={"username": "", "password": "admin1234"})
        assert resp.status_code in (400, 401)

    def test_login_empty_password(self, api):
        resp = api.post("/api/auth/login", json={"username": "admin", "password": ""})
        assert resp.status_code in (400, 401)

    def test_change_password_invalid_json(self, api, admin_cookies):
        resp = api.put("/api/auth/password", data="not json", cookies=admin_cookies)
        assert resp.status_code == 400

    def test_change_password_short_new(self, api, admin_cookies):
        resp = api.put(
            "/api/auth/password",
            json={"current_password": "admin1234", "new_password": "short"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400

    def test_change_password_without_session(self, api):
        resp = api.put(
            "/api/auth/password",
            json={"current_password": "admin1234", "new_password": "newpassword1234"},
        )
        assert resp.status_code in (401, 403)
