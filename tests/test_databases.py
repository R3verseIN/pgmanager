import pytest

pytestmark = [pytest.mark.databases]


class TestListDatabases:
    def test_list(self, api, admin_cookies, test_db):
        resp = api.get("/api/databases", cookies=admin_cookies)
        assert resp.status_code == 200
        dbs = resp.json()
        names = [d["name"] for d in dbs]
        assert "testdb" in names

    def test_hide_system(self, api, admin_cookies, test_db):
        resp = api.get("/api/databases", cookies=admin_cookies)
        dbs = resp.json()
        names = [d["name"] for d in dbs]
        assert "postgres" not in names
        assert "template0" not in names

    def test_show_system(self, api, admin_cookies, test_db):
        resp = api.get("/api/databases?showSystem=true", cookies=admin_cookies)
        dbs = resp.json()
        names = [d["name"] for d in dbs]
        assert "postgres" in names


class TestCreateDatabase:
    def test_success(self, api, admin_cookies):
        resp = api.post("/api/databases", json={"name": "newdb"}, cookies=admin_cookies)
        assert resp.status_code == 201

    def test_protected(self, api, admin_cookies):
        resp = api.post("/api/databases", json={"name": "postgres"}, cookies=admin_cookies)
        assert resp.status_code == 403

    def test_duplicate(self, api, admin_cookies):
        api.post("/api/databases", json={"name": "dupdb"}, cookies=admin_cookies)
        resp = api.post("/api/databases", json={"name": "dupdb"}, cookies=admin_cookies)
        assert resp.status_code == 409


class TestDeleteDatabase:
    def test_success(self, api, admin_cookies):
        api.post("/api/databases", json={"name": "deldb"}, cookies=admin_cookies)
        resp = api.delete("/api/databases/deldb", cookies=admin_cookies)
        assert resp.status_code == 204

    def test_protected(self, api, admin_cookies):
        resp = api.delete("/api/databases/postgres", cookies=admin_cookies)
        assert resp.status_code == 403
