import pytest
import psycopg2
import os

pytestmark = [pytest.mark.ssl]

PG_DSN = os.getenv("PG_DSN", "postgresql://pgmanager:pgmanager@db:5432/pgmanager?sslmode=disable")


def delete_ssl_certs():
    dsn = PG_DSN
    conn = psycopg2.connect(dsn)
    conn.autocommit = True
    cur = conn.cursor()
    cur.execute("SELECT 1")
    conn.close()


class TestSSLStatus:
    def test_initial_status(self, api, admin_cookies):
        resp = api.get("/api/ssl/status", cookies=admin_cookies)
        assert resp.status_code == 200
        data = resp.json()
        assert data["enabled"] is False
        assert data["hasCerts"] is False


class TestSSLCertificates:
    def test_generate_and_status(self, api, admin_cookies):
        resp = api.post(
            "/api/ssl/generate",
            json={"commonName": "pgmanager-server", "validityDays": 365},
            cookies=admin_cookies,
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["status"] == "generated"

        resp = api.get("/api/ssl/status", cookies=admin_cookies)
        assert resp.status_code == 200
        data = resp.json()
        assert data["hasCerts"] is True
        assert data["enabled"] is True

        api.delete("/api/ssl", cookies=admin_cookies)

    def test_disable_and_enable(self, api, admin_cookies):
        api.post(
            "/api/ssl/generate",
            json={"commonName": "pgmanager-server"},
            cookies=admin_cookies,
        )

        resp = api.post("/api/ssl/disable", cookies=admin_cookies)
        assert resp.status_code == 200
        data = resp.json()
        assert data["status"] == "disabled"

        resp = api.get("/api/ssl/status", cookies=admin_cookies)
        data = resp.json()
        assert data["enabled"] is False

        resp = api.post("/api/ssl/enable", cookies=admin_cookies)
        assert resp.status_code == 200
        data = resp.json()
        assert data["status"] == "enabled"

        resp = api.get("/api/ssl/status", cookies=admin_cookies)
        data = resp.json()
        assert data["enabled"] is True

        api.delete("/api/ssl", cookies=admin_cookies)

    def test_enable_already_enabled(self, api, admin_cookies):
        api.post(
            "/api/ssl/generate",
            json={"commonName": "pgmanager-server"},
            cookies=admin_cookies,
        )
        resp = api.post("/api/ssl/enable", cookies=admin_cookies)
        assert resp.status_code == 200
        data = resp.json()
        assert "already enabled" in data["message"]

        api.delete("/api/ssl", cookies=admin_cookies)

    def test_enable_without_certs(self, api, admin_cookies):
        resp = api.get("/api/ssl/status", cookies=admin_cookies)
        data = resp.json()
        if data["hasCerts"]:
            api.delete("/api/ssl", cookies=admin_cookies)

        resp = api.post("/api/ssl/enable", cookies=admin_cookies)
        assert resp.status_code == 400

    def test_delete_certs(self, api, admin_cookies):
        api.post(
            "/api/ssl/generate",
            json={"commonName": "pgmanager-server"},
            cookies=admin_cookies,
        )
        resp = api.delete("/api/ssl", cookies=admin_cookies)
        assert resp.status_code == 200
        data = resp.json()
        assert data["status"] == "deleted"

        resp = api.get("/api/ssl/status", cookies=admin_cookies)
        data = resp.json()
        assert data["hasCerts"] is False

    def test_download_ca(self, api, admin_cookies):
        api.post(
            "/api/ssl/generate",
            json={"commonName": "pgmanager-server"},
            cookies=admin_cookies,
        )
        resp = api.get("/api/ssl/download", cookies=admin_cookies)
        assert resp.status_code == 200
        assert "attachment" in resp.headers.get("Content-Disposition", "")
        assert len(resp.content) > 0

        api.delete("/api/ssl", cookies=admin_cookies)

    def test_download_ca_not_found(self, api, admin_cookies):
        resp = api.get("/api/ssl/status", cookies=admin_cookies)
        data = resp.json()
        if data["hasCerts"]:
            api.delete("/api/ssl", cookies=admin_cookies)

        resp = api.get("/api/ssl/download", cookies=admin_cookies)
        assert resp.status_code == 404
