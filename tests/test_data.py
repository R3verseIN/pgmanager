import os
import pytest
import psycopg2

pytestmark = [pytest.mark.data]

PG_DSN = os.getenv("PG_DSN", "postgresql://pgmanager:pgmanager@db:5432/pgmanager?sslmode=disable")


def setup_data(db_name):
    dsn = PG_DSN.rsplit("/", 1)[0] + "/" + db_name
    conn = psycopg2.connect(dsn)
    conn.autocommit = True
    cur = conn.cursor()
    cur.execute("""
        CREATE TABLE IF NOT EXISTS items (
            id SERIAL PRIMARY KEY,
            name TEXT NOT NULL,
            value INTEGER DEFAULT 0,
            active BOOLEAN DEFAULT true
        )
    """)
    for i in range(5):
        cur.execute("INSERT INTO items (name, value, active) VALUES (%s, %s, %s)",
                     (f"item{i}", i * 10, i % 2 == 0))
    conn.close()


class TestListData:
    def test_list(self, api, admin_cookies, test_db):
        setup_data(test_db)
        resp = api.get(f"/api/databases/{test_db}/data/items", cookies=admin_cookies)
        assert resp.status_code == 200
        data = resp.json()
        assert data["total"] == 5
        assert len(data["columns"]) == 4

    def test_pagination(self, api, admin_cookies, test_db):
        setup_data(test_db)
        resp = api.get(f"/api/databases/{test_db}/data/items?limit=2&offset=0", cookies=admin_cookies)
        data = resp.json()
        assert len(data["rows"]) == 2
        assert data["total"] == 5

    def test_sort(self, api, admin_cookies, test_db):
        setup_data(test_db)
        resp = api.get(f"/api/databases/{test_db}/data/items?sort=value&order=DESC", cookies=admin_cookies)
        data = resp.json()
        assert data["rows"][0][2] > data["rows"][1][2]


class TestInsertRow:
    def test_success(self, api, admin_cookies, test_db):
        setup_data(test_db)
        resp = api.post(
            f"/api/databases/{test_db}/data/items",
            json={"values": {"name": "newitem", "value": 99, "active": False}},
            cookies=admin_cookies,
        )
        assert resp.status_code == 201


class TestUpdateRow:
    def test_success(self, api, admin_cookies, test_db):
        setup_data(test_db)
        resp = api.put(
            f"/api/databases/{test_db}/data/items",
            json={
                "values": {"value": 999},
                "where": [{"column": "name", "operator": "=", "value": "item0"}],
            },
            cookies=admin_cookies,
        )
        assert resp.status_code == 200


class TestDeleteRow:
    def test_success(self, api, admin_cookies, test_db):
        setup_data(test_db)
        resp = api.delete(
            f"/api/databases/{test_db}/data/items",
            json={"where": [{"column": "name", "operator": "=", "value": "item0"}]},
            cookies=admin_cookies,
        )
        assert resp.status_code == 200


class TestDataBoundary:
    def test_limit_defaults_to_100(self, api, admin_cookies, test_db):
        setup_data(test_db)
        resp = api.get(f"/api/databases/{test_db}/data/items?limit=0", cookies=admin_cookies)
        assert resp.status_code == 200
        data = resp.json()
        assert len(data["rows"]) <= 100

    def test_negative_offset_defaults_to_0(self, api, admin_cookies, test_db):
        setup_data(test_db)
        resp = api.get(f"/api/databases/{test_db}/data/items?offset=-5", cookies=admin_cookies)
        assert resp.status_code == 200
        data = resp.json()
        assert data["total"] == 5

    def test_insert_empty_values(self, api, admin_cookies, test_db):
        setup_data(test_db)
        resp = api.post(
            f"/api/databases/{test_db}/data/items",
            json={"values": {}},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400

    def test_update_empty_values(self, api, admin_cookies, test_db):
        setup_data(test_db)
        resp = api.put(
            f"/api/databases/{test_db}/data/items",
            json={
                "values": {},
                "where": [{"column": "name", "operator": "=", "value": "item0"}],
            },
            cookies=admin_cookies,
        )
        assert resp.status_code == 400

    def test_update_missing_where(self, api, admin_cookies, test_db):
        setup_data(test_db)
        resp = api.put(
            f"/api/databases/{test_db}/data/items",
            json={"values": {"value": 999}},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400

    def test_delete_missing_where(self, api, admin_cookies, test_db):
        setup_data(test_db)
        resp = api.delete(
            f"/api/databases/{test_db}/data/items",
            json={},
            cookies=admin_cookies,
        )
        assert resp.status_code == 400

    def test_update_rows_affected(self, api, admin_cookies, test_db):
        setup_data(test_db)
        resp = api.put(
            f"/api/databases/{test_db}/data/items",
            json={
                "values": {"value": 888},
                "where": [{"column": "active", "operator": "=", "value": True}],
            },
            cookies=admin_cookies,
        )
        assert resp.status_code == 200
        data = resp.json()
        assert "rowsAffected" in data

    def test_delete_rows_affected(self, api, admin_cookies, test_db):
        setup_data(test_db)
        resp = api.delete(
            f"/api/databases/{test_db}/data/items",
            json={"where": [{"column": "active", "operator": "=", "value": False}]},
            cookies=admin_cookies,
        )
        assert resp.status_code == 200
        data = resp.json()
        assert "rowsAffected" in data
