import os
import time
import requests
import psycopg2
import pytest


BASE_URL = os.getenv("PGMANAGER_URL", "http://app:8080")
PG_DSN = os.getenv("PG_DSN", "postgresql://pgmanager:pgmanager@db:5432/pgmanager?sslmode=disable")


class APIClient:
    def __init__(self, session, base_url):
        self.session = session
        self.base_url = base_url.rstrip("/")

    def _url(self, path):
        if path.startswith("http://") or path.startswith("https://"):
            return path
        return f"{self.base_url}{path}"

    def _set_json_content_type(self, kwargs):
        if "files" not in kwargs:
            headers = kwargs.get("headers", {})
            headers.setdefault("Content-Type", "application/json")
            kwargs["headers"] = headers
        return kwargs

    def get(self, url, **kwargs):
        return self.session.get(self._url(url), **kwargs)

    def post(self, url, **kwargs):
        return self.session.post(self._url(url), **self._set_json_content_type(kwargs))

    def put(self, url, **kwargs):
        return self.session.put(self._url(url), **self._set_json_content_type(kwargs))

    def delete(self, url, **kwargs):
        return self.session.delete(self._url(url), **self._set_json_content_type(kwargs))

    @property
    def cookies(self):
        return self.session.cookies


@pytest.fixture(scope="session")
def base_url():
    return BASE_URL


@pytest.fixture(scope="session")
def api(base_url):
    session = requests.Session()
    return APIClient(session, base_url)


@pytest.fixture(scope="session")
def pg_conn():
    conn = psycopg2.connect(PG_DSN)
    conn.autocommit = True
    yield conn
    conn.close()


@pytest.fixture(scope="session")
def pg_cursor(pg_conn):
    cur = pg_conn.cursor()
    yield cur
    cur.close()


@pytest.fixture(autouse=True)
def clean_slate(pg_cursor):
    pg_cursor.execute("DELETE FROM sessions")
    pg_cursor.execute("DELETE FROM dev_databases")
    pg_cursor.execute("DELETE FROM managed_users")
    pg_cursor.execute("DELETE FROM audit_log")
    pg_cursor.execute("DELETE FROM auth_users")
    pg_cursor.execute("DELETE FROM system_config")
    pg_cursor.execute("""
        INSERT INTO system_config (key, value) VALUES
            ('pgbouncer_pool_mode', 'transaction'),
            ('pgbouncer_default_pool_size', '20'),
            ('pgbouncer_max_client_conn', '100')
        ON CONFLICT (key) DO NOTHING
    """)
    pg_cursor.execute("""
        INSERT INTO pgbouncer_databases (database_name, allowed)
        VALUES ('pgmanager', false), ('postgres', false), ('template0', false), ('template1', false)
        ON CONFLICT (database_name) DO NOTHING
    """)
    managed_roles = [
        "readuser", "readuser2", "readuser3", "writeuser", "writeuser2",
        "ddluser", "fulluser", "lvlreaduser", "lvlwriteuser", "lvlddluser", "lvlfulluser",
        "addbuser", "rmdruser", "uplevel", "updb", "genpw", "delrole", "delmeta",
        "testuser", "genuser", "dupuser", "deluser", "upuser",
        "audituser", "ghost", "ghostdb", "protdb", "dupdb",
        "combo",
        "writedel", "wrseq", "ddlins", "ddlseq", "readdrop",
        "defread", "defwrite",
        "viewdb1", "viewdb2", "viewusr1", "viewset1", "viewpgb1", "viewauth1",
        "devdb1", "devdb2", "devusr1", "devset1", "devpb1", "devrd1", "devsql1", "devnoa1",
        "shortpw", "longpw", "protdbusr", "nodbusr", "badaccess", "nodbs",
    ]
    for role in managed_roles:
        try:
            pg_cursor.execute(f"DROP OWNED BY {role} CASCADE")
        except Exception:
            pass
        try:
            pg_cursor.execute(f"DROP ROLE IF EXISTS {role}")
        except Exception:
            pass
    for db_name in ["newdb", "dupdb", "deldb", "adddb2", "rmdrb", "filterdb", "datedb",
                     "toonlydb", "rangedb", "delauditdb", "auditdb", "newdbaccess",
                     "auditudb", "auditrmd", "otherdb", "shouldfail"]:
        try:
            pg_cursor.execute(f"DROP DATABASE IF EXISTS {db_name} WITH (FORCE)")
        except Exception:
            pass


@pytest.fixture
def admin_session(api, pg_cursor):
    pg_cursor.execute(
        "INSERT INTO auth_users (username, password_hash, role) VALUES ($1, crypt('admin1234', gen_salt('bf')), 'admin') RETURNING id",
        ("admin",),
    )
    session = requests.Session()
    yield APIClient(session, BASE_URL)


@pytest.fixture
def admin_token(api, pg_cursor):
    resp = api.post("/api/auth/setup", json={"username": "admin", "password": "admin1234"})
    assert resp.status_code == 201, f"setup failed: {resp.text}"
    resp = api.post("/api/auth/login", json={"username": "admin", "password": "admin1234"})
    assert resp.status_code == 200, f"login failed: {resp.text}"
    return resp.cookies.get("session_id")


@pytest.fixture
def admin_cookies(admin_token):
    return {"session_id": admin_token}


@pytest.fixture
def test_db(pg_cursor):
    pg_cursor.execute("DROP DATABASE IF EXISTS testdb WITH (FORCE)")
    pg_cursor.execute("CREATE DATABASE testdb")
    pg_cursor.execute("INSERT INTO pgbouncer_databases (database_name, allowed) VALUES ('testdb', true) ON CONFLICT (database_name) DO NOTHING")
    yield "testdb"
    pg_cursor.execute("DROP DATABASE IF EXISTS testdb WITH (FORCE)")
    pg_cursor.execute("DELETE FROM pgbouncer_databases WHERE database_name = 'testdb'")
