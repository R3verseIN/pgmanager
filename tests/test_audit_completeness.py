import pytest

pytestmark = [pytest.mark.audit_completeness]


class TestAuditLogEntryFields:
    def test_create_user_creates_audit_entry(self, api, admin_cookies, test_db):
        api.post(
            "/api/users",
            json={"username": "audituser", "password": "auditpass1234", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        resp = api.get("/api/logs?action=create_user", cookies=admin_cookies)
        data = resp.json()
        assert data["total"] >= 1
        entry = data["entries"][0]
        assert entry["action"] == "create_user"
        assert entry["username"] == "admin"
        assert entry["ipAddress"] is not None

    def test_create_database_creates_audit_entry(self, api, admin_cookies):
        api.post("/api/databases", json={"name": "auditdb"}, cookies=admin_cookies)
        resp = api.get("/api/logs?action=create_database", cookies=admin_cookies)
        data = resp.json()
        assert data["total"] >= 1
        entry = data["entries"][0]
        assert entry["action"] == "create_database"
        assert entry["database"] == "auditdb"

    def test_delete_database_creates_audit_entry(self, api, admin_cookies):
        api.post("/api/databases", json={"name": "delauditdb"}, cookies=admin_cookies)
        api.delete("/api/databases/delauditdb", cookies=admin_cookies)
        resp = api.get("/api/logs?action=delete_database", cookies=admin_cookies)
        data = resp.json()
        assert data["total"] >= 1

    def test_raw_query_creates_audit_entry(self, api, admin_cookies, test_db):
        dsn = f"postgresql://pgmanager:pgmanager@db:5432/{test_db}?sslmode=disable"
        import psycopg2
        conn = psycopg2.connect(dsn)
        conn.autocommit = True
        conn.cursor().execute("CREATE TABLE IF NOT EXISTS audit_qtest (id SERIAL PRIMARY KEY)")
        conn.close()

        api.post(
            f"/api/databases/{test_db}/query",
            json={"sql": "SELECT 1"},
            cookies=admin_cookies,
        )
        resp = api.get("/api/logs?action=raw_query", cookies=admin_cookies)
        data = resp.json()
        assert data["total"] >= 1
        entry = data["entries"][0]
        assert entry["action"] == "raw_query"
        assert entry["database"] == test_db
        assert entry["detail"] is not None


class TestAuditLogFiltering:
    def test_filter_by_database(self, api, admin_cookies, test_db):
        api.post("/api/databases", json={"name": "filterdb"}, cookies=admin_cookies)
        resp = api.get("/api/logs?database=filterdb", cookies=admin_cookies)
        data = resp.json()
        assert data["total"] >= 1
        for entry in data["entries"]:
            assert entry["database"] == "filterdb"

    def test_filter_by_date_from(self, api, admin_cookies, test_db):
        api.post("/api/databases", json={"name": "datedb"}, cookies=admin_cookies)
        resp = api.get("/api/logs?from=2000-01-01T00:00:00Z", cookies=admin_cookies)
        data = resp.json()
        assert data["total"] >= 1

    def test_filter_by_date_to(self, api, admin_cookies, test_db):
        api.post("/api/databases", json={"name": "toonlydb"}, cookies=admin_cookies)
        resp = api.get("/api/logs?to=2099-12-31T23:59:59Z", cookies=admin_cookies)
        data = resp.json()
        assert data["total"] >= 1

    def test_filter_by_date_range(self, api, admin_cookies, test_db):
        api.post("/api/databases", json={"name": "rangedb"}, cookies=admin_cookies)
        resp = api.get(
            "/api/logs?from=2000-01-01T00:00:00Z&to=2099-12-31T23:59:59Z",
            cookies=admin_cookies,
        )
        data = resp.json()
        assert data["total"] >= 1

    def test_combined_filters(self, api, admin_cookies, test_db):
        api.post(
            "/api/auth/users",
            json={"username": "combo", "password": "combopass123", "role": "viewer"},
            cookies=admin_cookies,
        )
        resp = api.get(
            "/api/logs?action=create_auth_user&username=admin",
            cookies=admin_cookies,
        )
        data = resp.json()
        assert data["total"] >= 1
        for entry in data["entries"]:
            assert entry["action"] == "create_auth_user"
            assert entry["username"] == "admin"


class TestAdditionalAuditActions:
    def test_update_user_audit_entry(self, api, admin_cookies, test_db):
        api.post(
            "/api/users",
            json={"username": "auditupd", "password": "auditupd1234", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        api.put(
            "/api/users/auditupd",
            json={"access": "write"},
            cookies=admin_cookies,
        )
        resp = api.get("/api/logs?action=update_user", cookies=admin_cookies)
        data = resp.json()
        assert data["total"] >= 1

    def test_delete_user_audit_entry(self, api, admin_cookies, test_db):
        api.post(
            "/api/users",
            json={"username": "auditdel", "password": "auditdel1234", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        api.delete("/api/users/auditdel", cookies=admin_cookies)
        resp = api.get("/api/logs?action=delete_user", cookies=admin_cookies)
        data = resp.json()
        assert data["total"] >= 1

    def test_remove_user_database_audit_entry(self, api, admin_cookies, test_db):
        api.post("/api/databases", json={"name": "auditrmd"}, cookies=admin_cookies)
        api.post(
            "/api/users",
            json={"username": "auditrmdusr", "password": "auditrmdpass1234", "access": "read", "databases": [test_db, "auditrmd"]},
            cookies=admin_cookies,
        )
        api.delete("/api/users/auditrmdusr/databases/auditrmd", cookies=admin_cookies)
        resp = api.get("/api/logs?action=remove_user_database", cookies=admin_cookies)
        data = resp.json()
        assert data["total"] >= 1

    def test_list_tables_audit_entry(self, api, admin_cookies, test_db):
        dsn = f"postgresql://pgmanager:pgmanager@db:5432/{test_db}?sslmode=disable"
        import psycopg2
        conn = psycopg2.connect(dsn)
        conn.autocommit = True
        conn.cursor().execute("CREATE TABLE IF NOT EXISTS audit_tbl (id SERIAL PRIMARY KEY)")
        conn.close()

        api.get(f"/api/databases/{test_db}/tables", cookies=admin_cookies)
        resp = api.get("/api/logs?action=list_tables", cookies=admin_cookies)
        data = resp.json()
        assert data["total"] >= 1

    def test_view_data_audit_entry(self, api, admin_cookies, test_db):
        dsn = f"postgresql://pgmanager:pgmanager@db:5432/{test_db}?sslmode=disable"
        import psycopg2
        conn = psycopg2.connect(dsn)
        conn.autocommit = True
        conn.cursor().execute("CREATE TABLE IF NOT EXISTS audit_vd (id SERIAL PRIMARY KEY)")
        conn.close()

        api.get(f"/api/databases/{test_db}/data/audit_vd", cookies=admin_cookies)
        resp = api.get("/api/logs?action=view_data", cookies=admin_cookies)
        data = resp.json()
        assert data["total"] >= 1

    def test_insert_row_audit_entry(self, api, admin_cookies, test_db):
        dsn = f"postgresql://pgmanager:pgmanager@db:5432/{test_db}?sslmode=disable"
        import psycopg2
        conn = psycopg2.connect(dsn)
        conn.autocommit = True
        conn.cursor().execute("CREATE TABLE IF NOT EXISTS audit_ir (id SERIAL PRIMARY KEY, name TEXT)")
        conn.close()

        api.post(
            f"/api/databases/{test_db}/data/audit_ir",
            json={"values": {"name": "auditrow"}},
            cookies=admin_cookies,
        )
        resp = api.get("/api/logs?action=insert_row", cookies=admin_cookies)
        data = resp.json()
        assert data["total"] >= 1

    def test_list_pgbouncer_databases_audit_entry(self, api, admin_cookies):
        api.get("/api/pgbouncer/databases", cookies=admin_cookies)
        resp = api.get("/api/logs?action=list_pgbouncer_databases", cookies=admin_cookies)
        data = resp.json()
        assert data["total"] >= 1
