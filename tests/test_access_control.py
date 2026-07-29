import os
import pytest
import psycopg2

pytestmark = [pytest.mark.access_control]

PG_DSN = os.getenv("PG_DSN", "postgresql://pgmanager:pgmanager@db:5432/pgmanager?sslmode=disable")


def setup_table(db_name, table_name="access_test"):
    dsn = PG_DSN.rsplit("/", 1)[0] + "/" + db_name
    conn = psycopg2.connect(dsn)
    conn.autocommit = True
    cur = conn.cursor()
    cur.execute(f"""
        CREATE TABLE IF NOT EXISTS {table_name} (
            id SERIAL PRIMARY KEY,
            name TEXT NOT NULL
        )
    """)
    cur.execute(f"INSERT INTO {table_name} (name) VALUES ('row1'), ('row2'), ('row3')")
    conn.close()


def connect_as_user(username, password, db_name):
    dsn = PG_DSN.rsplit("/", 1)[0] + "/" + db_name
    dsn = dsn.replace("pgmanager:pgmanager", f"{username}:{password}")
    conn = psycopg2.connect(dsn)
    conn.autocommit = True
    return conn


class TestReadAccess:
    def test_read_user_can_select(self, api, admin_cookies, test_db):
        setup_table(test_db)
        resp = api.post(
            "/api/users",
            json={"username": "readuser", "password": "readpass1234", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        assert resp.status_code == 201
        password = resp.json()["password"]

        conn = connect_as_user("readuser", password, test_db)
        cur = conn.cursor()
        cur.execute("SELECT * FROM access_test")
        rows = cur.fetchall()
        assert len(rows) == 3
        conn.close()

    def test_read_user_cannot_insert(self, api, admin_cookies, test_db):
        setup_table(test_db)
        resp = api.post(
            "/api/users",
            json={"username": "readuser2", "password": "readpass1234", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        password = resp.json()["password"]

        conn = connect_as_user("readuser2", password, test_db)
        cur = conn.cursor()
        try:
            cur.execute("INSERT INTO access_test (name) VALUES ('should_fail')")
            conn.commit()
            assert False, "should have raised an exception"
        except psycopg2.errors.InsufficientPrivilege:
            pass
        finally:
            conn.close()

    def test_read_user_cannot_update(self, api, admin_cookies, test_db):
        setup_table(test_db)
        resp = api.post(
            "/api/users",
            json={"username": "readuser3", "password": "readpass1234", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        password = resp.json()["password"]

        conn = connect_as_user("readuser3", password, test_db)
        cur = conn.cursor()
        try:
            cur.execute("UPDATE access_test SET name = 'hacked' WHERE id = 1")
            conn.commit()
            assert False, "should have raised an exception"
        except psycopg2.errors.InsufficientPrivilege:
            pass
        finally:
            conn.close()


class TestWriteAccess:
    def test_write_user_can_insert(self, api, admin_cookies, test_db):
        setup_table(test_db)
        resp = api.post(
            "/api/users",
            json={"username": "writeuser", "password": "writepass1234", "access": "write", "databases": [test_db]},
            cookies=admin_cookies,
        )
        password = resp.json()["password"]

        conn = connect_as_user("writeuser", password, test_db)
        cur = conn.cursor()
        cur.execute("INSERT INTO access_test (name) VALUES ('from_write_user')")
        conn.commit()
        cur.execute("SELECT name FROM access_test WHERE name = 'from_write_user'")
        assert cur.fetchone() is not None
        conn.close()

    def test_write_user_cannot_create_table(self, api, admin_cookies, test_db):
        setup_table(test_db)
        resp = api.post(
            "/api/users",
            json={"username": "writeuser2", "password": "writepass1234", "access": "write", "databases": [test_db]},
            cookies=admin_cookies,
        )
        password = resp.json()["password"]

        conn = connect_as_user("writeuser2", password, test_db)
        cur = conn.cursor()
        try:
            cur.execute("CREATE TABLE should_not_exist (id SERIAL)")
            conn.commit()
            assert False, "should have raised an exception"
        except psycopg2.errors.InsufficientPrivilege:
            pass
        finally:
            conn.close()


class TestDDLAccess:
    def test_ddl_user_can_create_table(self, api, admin_cookies, test_db):
        setup_table(test_db)
        resp = api.post(
            "/api/users",
            json={"username": "ddluser", "password": "ddlpass1234", "access": "ddl", "databases": [test_db]},
            cookies=admin_cookies,
        )
        password = resp.json()["password"]

        conn = connect_as_user("ddluser", password, test_db)
        cur = conn.cursor()
        cur.execute("CREATE TABLE ddl_test (id SERIAL PRIMARY KEY, val TEXT)")
        conn.commit()
        cur.execute("SELECT table_name FROM information_schema.tables WHERE table_name = 'ddl_test'")
        assert cur.fetchone() is not None
        conn.close()


class TestFullAccess:
    def test_full_user_has_all_privileges(self, api, admin_cookies, test_db):
        setup_table(test_db)
        resp = api.post(
            "/api/users",
            json={"username": "fulluser", "password": "fullpass1234", "access": "full", "databases": [test_db]},
            cookies=admin_cookies,
        )
        password = resp.json()["password"]

        conn = connect_as_user("fulluser", password, test_db)
        cur = conn.cursor()
        cur.execute("CREATE TABLE full_test (id SERIAL)")
        conn.commit()
        cur.execute("INSERT INTO full_test DEFAULT VALUES")
        conn.commit()
        cur.execute("DROP TABLE full_test")
        conn.commit()
        conn.close()


class TestWriteDeleteAccess:
    def test_write_user_can_delete(self, api, admin_cookies, test_db):
        setup_table(test_db)
        resp = api.post(
            "/api/users",
            json={"username": "writedel", "password": "writedel1234", "access": "write", "databases": [test_db]},
            cookies=admin_cookies,
        )
        password = resp.json()["password"]

        conn = connect_as_user("writedel", password, test_db)
        cur = conn.cursor()
        cur.execute("DELETE FROM access_test WHERE name = 'row1'")
        conn.commit()
        cur.execute("SELECT COUNT(*) FROM access_test")
        assert cur.fetchone()[0] == 2
        conn.close()

    def test_write_user_can_use_sequences(self, api, admin_cookies, test_db):
        setup_table(test_db)
        resp = api.post(
            "/api/users",
            json={"username": "wrseq", "password": "wrseq1234", "access": "write", "databases": [test_db]},
            cookies=admin_cookies,
        )
        password = resp.json()["password"]

        conn = connect_as_user("wrseq", password, test_db)
        cur = conn.cursor()
        cur.execute("INSERT INTO access_test (name) VALUES ('seqtest')")
        conn.commit()
        cur.execute("SELECT id FROM access_test WHERE name = 'seqtest'")
        assert cur.fetchone() is not None
        conn.close()


class TestDDLInsertAccess:
    def test_ddl_user_can_insert(self, api, admin_cookies, test_db):
        setup_table(test_db)
        resp = api.post(
            "/api/users",
            json={"username": "ddlins", "password": "ddlins1234", "access": "ddl", "databases": [test_db]},
            cookies=admin_cookies,
        )
        password = resp.json()["password"]

        conn = connect_as_user("ddlins", password, test_db)
        cur = conn.cursor()
        cur.execute("INSERT INTO access_test (name) VALUES ('ddl_insert')")
        conn.commit()
        cur.execute("SELECT name FROM access_test WHERE name = 'ddl_insert'")
        assert cur.fetchone() is not None
        conn.close()

    def test_ddl_user_can_use_sequences(self, api, admin_cookies, test_db):
        setup_table(test_db)
        resp = api.post(
            "/api/users",
            json={"username": "ddlseq", "password": "ddlseq1234", "access": "ddl", "databases": [test_db]},
            cookies=admin_cookies,
        )
        password = resp.json()["password"]

        conn = connect_as_user("ddlseq", password, test_db)
        cur = conn.cursor()
        cur.execute("INSERT INTO access_test (name) VALUES ('ddl_seq')")
        conn.commit()
        cur.execute("SELECT id FROM access_test WHERE name = 'ddl_seq'")
        assert cur.fetchone() is not None
        conn.close()


class TestReadCannotDrop:
    def test_read_user_cannot_drop_table(self, api, admin_cookies, test_db):
        setup_table(test_db)
        resp = api.post(
            "/api/users",
            json={"username": "readdrop", "password": "readdrop1234", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        password = resp.json()["password"]

        conn = connect_as_user("readdrop", password, test_db)
        cur = conn.cursor()
        try:
            cur.execute("DROP TABLE access_test")
            conn.commit()
            assert False, "should have raised an exception"
        except psycopg2.errors.InsufficientPrivilege:
            pass
        finally:
            conn.close()


class TestDefaultPrivileges:
    def test_read_user_can_select_on_new_table(self, api, admin_cookies, test_db):
        resp = api.post(
            "/api/users",
            json={"username": "defread", "password": "defread1234", "access": "read", "databases": [test_db]},
            cookies=admin_cookies,
        )
        password = resp.json()["password"]

        admin_dsn = PG_DSN.rsplit("/", 1)[0] + "/" + test_db
        admin_conn = psycopg2.connect(admin_dsn)
        admin_conn.autocommit = True
        admin_conn.cursor().execute("CREATE TABLE after_grant (id SERIAL PRIMARY KEY, val TEXT)")
        admin_conn.close()

        conn = connect_as_user("defread", password, test_db)
        cur = conn.cursor()
        cur.execute("SELECT * FROM after_grant")
        rows = cur.fetchall()
        assert len(rows) == 0
        conn.close()

    def test_write_user_can_insert_on_new_table(self, api, admin_cookies, test_db):
        resp = api.post(
            "/api/users",
            json={"username": "defwrite", "password": "defwrite1234", "access": "write", "databases": [test_db]},
            cookies=admin_cookies,
        )
        password = resp.json()["password"]

        admin_dsn = PG_DSN.rsplit("/", 1)[0] + "/" + test_db
        admin_conn = psycopg2.connect(admin_dsn)
        admin_conn.autocommit = True
        admin_conn.cursor().execute("CREATE TABLE after_grant_wr (id SERIAL PRIMARY KEY, val TEXT)")
        admin_conn.close()

        conn = connect_as_user("defwrite", password, test_db)
        cur = conn.cursor()
        cur.execute("INSERT INTO after_grant_wr (val) VALUES ('new')")
        conn.commit()
        cur.execute("SELECT val FROM after_grant_wr WHERE val = 'new'")
        assert cur.fetchone() is not None
        conn.close()
