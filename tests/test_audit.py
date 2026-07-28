import pytest

pytestmark = [pytest.mark.audit]


class TestListLogs:
    def test_empty(self, api, admin_cookies):
        resp = api.get("/api/logs", cookies=admin_cookies)
        assert resp.status_code == 200
        data = resp.json()
        assert data["total"] <= 1

    def test_with_entries(self, api, admin_cookies, test_db):
        api.post(
            "/api/auth/users",
            json={"username": "viewer1", "password": "viewer1234", "role": "viewer"},
            cookies=admin_cookies,
        )
        resp = api.get("/api/logs", cookies=admin_cookies)
        data = resp.json()
        assert data["total"] >= 1

    def test_filter_username(self, api, admin_cookies, test_db):
        api.post(
            "/api/auth/users",
            json={"username": "viewer1", "password": "viewer1234", "role": "viewer"},
            cookies=admin_cookies,
        )
        resp = api.get("/api/logs?username=admin", cookies=admin_cookies)
        data = resp.json()
        assert data["total"] >= 1

    def test_filter_action(self, api, admin_cookies, test_db):
        api.post(
            "/api/auth/users",
            json={"username": "viewer1", "password": "viewer1234", "role": "viewer"},
            cookies=admin_cookies,
        )
        resp = api.get("/api/logs?action=create_auth_user", cookies=admin_cookies)
        data = resp.json()
        assert data["total"] >= 1

    def test_pagination(self, api, admin_cookies, test_db):
        for i in range(5):
            api.post(
                "/api/auth/users",
                json={"username": f"user{i}", "password": "userpass123", "role": "viewer"},
                cookies=admin_cookies,
            )
        resp = api.get("/api/logs?limit=2&offset=0", cookies=admin_cookies)
        data = resp.json()
        assert len(data["entries"]) == 2
        assert data["total"] >= 5

    def test_order_desc(self, api, admin_cookies, test_db):
        api.post(
            "/api/auth/users",
            json={"username": "first", "password": "userpass123", "role": "viewer"},
            cookies=admin_cookies,
        )
        api.post(
            "/api/auth/users",
            json={"username": "second", "password": "userpass123", "role": "viewer"},
            cookies=admin_cookies,
        )
        resp = api.get("/api/logs", cookies=admin_cookies)
        data = resp.json()
        assert len(data["entries"]) >= 2
        assert data["entries"][0]["action"] == "create_auth_user"
