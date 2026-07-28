import pytest

pytestmark = [pytest.mark.users]


class TestListUsers:
    def test_empty(self, api, admin_cookies, test_db):
        resp = api.get("/api/users", cookies=admin_cookies)
        assert resp.status_code == 200
        users = resp.json()
        assert len(users) == 0


class TestCreateUser:
    def test_success(self, api, admin_cookies, test_db):
        resp = api.post(
            "/api/users",
            json={"username": "testuser", "password": "testpass1234", "access": "write", "databases": [test_db]},
            cookies=admin_cookies,
        )
        assert resp.status_code == 201
        data = resp.json()
        assert data["username"] == "testuser"
        assert data["password"] != ""
        assert data["connectionString"] != ""

    def test_duplicate(self, api, admin_cookies, test_db):
        api.post(
            "/api/users",
            json={"username": "dupuser", "password": "testpass1234", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        resp = api.post(
            "/api/users",
            json={"username": "dupuser", "password": "testpass1234", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400

    def test_no_databases(self, api, admin_cookies):
        resp = api.post(
            "/api/users",
            json={"username": "testuser", "password": "testpass1234", "access": "read", "databases": []},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400

    def test_invalid_access(self, api, admin_cookies, test_db):
        resp = api.post(
            "/api/users",
            json={"username": "testuser", "password": "testpass1234", "access": "superadmin", "databases": [test_db]},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400

    def test_generate_password(self, api, admin_cookies, test_db):
        resp = api.post(
            "/api/users",
            json={"username": "genuser", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        assert resp.status_code == 201
        data = resp.json()
        assert len(data["password"]) == 16


class TestDeleteUser:
    def test_success(self, api, admin_cookies, test_db):
        api.post(
            "/api/users",
            json={"username": "deluser", "password": "testpass1234", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        resp = api.delete("/api/users/deluser", cookies=admin_cookies)
        assert resp.status_code == 200


class TestUpdateUser:
    def test_access(self, api, admin_cookies, test_db):
        api.post(
            "/api/users",
            json={"username": "upuser", "password": "testpass1234", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        resp = api.put(
            "/api/users/upuser",
            json={"access": "full"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 200


class TestAccessLevels:
    @pytest.mark.parametrize("level", ["read", "write", "ddl", "full"])
    def test_all_levels(self, api, admin_cookies, test_db, level):
        resp = api.post(
            "/api/users",
            json={"username": f"{level}user", "password": "testpass1234", "access": level, "databases": [test_db]},
            cookies=admin_cookies,
        )
        assert resp.status_code == 201
