import os
import hashlib
import io
import pytest
import psycopg2

pytestmark = [pytest.mark.backup]

PG_DSN = os.getenv("PG_DSN", "postgresql://pgmanager:pgmanager@db:5432/pgmanager?sslmode=disable")


def setup_backup_data(db_name):
    dsn = PG_DSN.rsplit("/", 1)[0] + "/" + db_name
    conn = psycopg2.connect(dsn)
    conn.autocommit = True
    cur = conn.cursor()
    cur.execute("""
        CREATE TABLE IF NOT EXISTS backup_test (
            id SERIAL PRIMARY KEY,
            data TEXT NOT NULL,
            checksum TEXT
        )
    """)
    test_data = ["alpha", "bravo", "charlie", "delta", "echo"]
    for item in test_data:
        cur.execute("INSERT INTO backup_test (data) VALUES (%s) RETURNING id", (item,))
        row_id = cur.fetchone()[0]
        checksum = hashlib.sha256(item.encode()).hexdigest()
        cur.execute("UPDATE backup_test SET checksum = %s WHERE id = %s", (checksum, row_id))
    conn.close()


def create_table_in_db(db_name, table_name, columns):
    dsn = PG_DSN.rsplit("/", 1)[0] + "/" + db_name
    conn = psycopg2.connect(dsn)
    conn.autocommit = True
    cur = conn.cursor()
    col_defs = ", ".join(f"{name} {typ}" for name, typ in columns)
    cur.execute(f"CREATE TABLE IF NOT EXISTS {table_name} ({col_defs})")
    conn.close()


def compute_checksum(db_name, table_name):
    dsn = PG_DSN.rsplit("/", 1)[0] + "/" + db_name
    conn = psycopg2.connect(dsn)
    conn.autocommit = True
    cur = conn.cursor()
    cur.execute(f"SELECT data, checksum FROM {table_name} ORDER BY id")
    rows = cur.fetchall()
    conn.close()
    return rows


class TestListBackupDatabases:
    def test_list(self, api, admin_cookies, test_db):
        resp = api.get("/api/backup/databases", cookies=admin_cookies)
        assert resp.status_code == 200
        dbs = resp.json()
        names = [d["name"] for d in dbs]
        assert "testdb" in names

    def test_hide_system(self, api, admin_cookies, test_db):
        resp = api.get("/api/backup/databases", cookies=admin_cookies)
        dbs = resp.json()
        names = [d["name"] for d in dbs]
        assert "pgmanager" not in names


class TestListBackupTables:
    def test_list(self, api, admin_cookies, test_db):
        create_table_in_db(test_db, "backup_test", [("id", "SERIAL PRIMARY KEY"), ("name", "TEXT")])
        resp = api.get(f"/api/backup/tables?db={test_db}", cookies=admin_cookies)
        assert resp.status_code == 200
        data = resp.json()
        assert data["database"] == test_db
        assert len(data["tables"]) >= 1

    def test_missing_db(self, api, admin_cookies):
        resp = api.get("/api/backup/tables", cookies=admin_cookies)
        assert resp.status_code == 400


class TestBackupAndRestore:
    def test_backup_creates_file(self, api, admin_cookies, test_db):
        setup_backup_data(test_db)
        resp = api.post(
            "/api/backup/create",
            json={"database": test_db},
            cookies=admin_cookies,
        )
        assert resp.status_code == 200
        assert len(resp.content) > 0
        assert resp.content[:5] == b"PGDMP"

    def test_backup_table_filter(self, api, admin_cookies, test_db):
        create_table_in_db(test_db, "table_a", [("id", "SERIAL PRIMARY KEY")])
        create_table_in_db(test_db, "table_b", [("id", "SERIAL PRIMARY KEY")])
        resp = api.post(
            "/api/backup/create",
            json={"database": test_db, "tables": ["table_a"]},
            cookies=admin_cookies,
        )
        assert resp.status_code == 200
        assert resp.content[:5] == b"PGDMP"

    def test_backup_protected_database(self, api, admin_cookies):
        resp = api.post(
            "/api/backup/create",
            json={"database": "postgres"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 403

    def test_backup_nonexistent_database(self, api, admin_cookies):
        resp = api.post(
            "/api/backup/create",
            json={"database": "nonexistent_db_xyz"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 500

    def test_restore_with_drop_first(self, api, admin_cookies, test_db):
        setup_backup_data(test_db)
        backup_resp = api.post(
            "/api/backup/create",
            json={"database": test_db},
            cookies=admin_cookies,
        )
        assert backup_resp.status_code == 200
        backup_data = backup_resp.content

        files = {
            "database": (None, test_db),
            "dropFirst": (None, "true"),
            "file": ("backup.dump", io.BytesIO(backup_data), "application/octet-stream"),
        }
        restore_resp = api.post(
            "/api/backup/restore",
            files=files,
            cookies=admin_cookies,
        )
        assert restore_resp.status_code == 200
        data = restore_resp.json()
        assert data["success"] is True

    def test_restore_integrity(self, api, admin_cookies, test_db):
        setup_backup_data(test_db)
        original = compute_checksum(test_db, "backup_test")

        backup_resp = api.post(
            "/api/backup/create",
            json={"database": test_db},
            cookies=admin_cookies,
        )
        backup_data = backup_resp.content

        dsn = PG_DSN.rsplit("/", 1)[0] + "/" + test_db
        conn = psycopg2.connect(dsn)
        conn.autocommit = True
        conn.cursor().execute("DROP TABLE IF EXISTS backup_test CASCADE")
        conn.close()

        files = {
            "database": (None, test_db),
            "dropFirst": (None, "true"),
            "file": ("backup.dump", io.BytesIO(backup_data), "application/octet-stream"),
        }
        restore_resp = api.post(
            "/api/backup/restore",
            files=files,
            cookies=admin_cookies,
        )
        assert restore_resp.status_code == 200

        restored = compute_checksum(test_db, "backup_test")
        assert len(restored) == len(original)
        for orig_row, rest_row in zip(original, restored):
            assert orig_row == rest_row

    def test_restore_to_protected_database(self, api, admin_cookies, test_db):
        setup_backup_data(test_db)
        backup_resp = api.post(
            "/api/backup/create",
            json={"database": test_db},
            cookies=admin_cookies,
        )
        backup_data = backup_resp.content

        files = {
            "database": (None, "postgres"),
            "dropFirst": (None, "true"),
            "file": ("backup.dump", io.BytesIO(backup_data), "application/octet-stream"),
        }
        restore_resp = api.post(
            "/api/backup/restore",
            files=files,
            cookies=admin_cookies,
        )
        assert restore_resp.status_code == 403

    def test_inspect_backup(self, api, admin_cookies, test_db):
        setup_backup_data(test_db)
        backup_resp = api.post(
            "/api/backup/create",
            json={"database": test_db},
            cookies=admin_cookies,
        )
        backup_data = backup_resp.content

        files = {"file": ("backup.dump", io.BytesIO(backup_data), "application/octet-stream")}
        resp = api.post(
            "/api/backup/inspect",
            files=files,
            cookies=admin_cookies,
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["database"] == test_db
        assert data["format"] == "custom"
        assert data["size"] > 0

    def test_inspect_invalid_file(self, api, admin_cookies):
        files = {"file": ("bad.txt", io.BytesIO(b"not a backup file"), "text/plain")}
        resp = api.post(
            "/api/backup/inspect",
            files=files,
            cookies=admin_cookies,
        )
        assert resp.status_code == 400
