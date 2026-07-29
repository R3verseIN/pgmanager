import pytest
import io
import os
import psycopg2

pytestmark = [pytest.mark.ssl]

PG_DSN = os.getenv("PG_DSN", "postgresql://pgmanager:pgmanager@db:5432/pgmanager?sslmode=disable")


def generate_self_signed_cert():
    from cryptography import x509
    from cryptography.x509.oid import NameOID
    from cryptography.hazmat.primitives import hashes, serialization
    from cryptography.hazmat.primitives.asymmetric import ec
    import datetime

    key = ec.generate_private_key(ec.SECP256R1())
    subject = issuer = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, "test-server")])
    cert = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(issuer)
        .public_key(key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(datetime.datetime.now(datetime.timezone.utc))
        .not_valid_after(datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(days=365))
        .add_extension(x509.BasicConstraints(ca=False, path_length=None), critical=True)
        .sign(key, hashes.SHA256())
    )
    cert_pem = cert.public_bytes(serialization.Encoding.PEM)
    key_pem = key.private_bytes(
        serialization.Encoding.PEM,
        serialization.PrivateFormat.TraditionalOpenSSL,
        serialization.NoEncryption(),
    )
    return cert_pem, key_pem


def generate_ca_cert(cn="test-ca"):
    from cryptography import x509
    from cryptography.x509.oid import NameOID
    from cryptography.hazmat.primitives import hashes, serialization
    from cryptography.hazmat.primitives.asymmetric import ec
    import datetime

    key = ec.generate_private_key(ec.SECP256R1())
    subject = issuer = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, cn)])
    cert = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(issuer)
        .public_key(key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(datetime.datetime.now(datetime.timezone.utc))
        .not_valid_after(datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(days=3650))
        .add_extension(x509.BasicConstraints(ca=True, path_length=1), critical=True)
        .add_extension(
            x509.KeyUsage(
                digital_signature=False, key_encipherment=False, content_commitment=False,
                data_encipherment=False, key_agreement=False, key_cert_sign=True,
                crl_sign=True, encipher_only=False, decipher_only=False,
            ),
            critical=True,
        )
        .sign(key, hashes.SHA256())
    )
    cert_pem = cert.public_bytes(serialization.Encoding.PEM)
    key_pem = key.private_bytes(
        serialization.Encoding.PEM,
        serialization.PrivateFormat.TraditionalOpenSSL,
        serialization.NoEncryption(),
    )
    return cert_pem, key_pem, key, cert


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


class TestUploadCerts:
    def test_upload_valid_certs(self, api, admin_cookies):
        cert_pem, key_pem = generate_self_signed_cert()
        files = {
            "server_cert": ("server.crt", io.BytesIO(cert_pem), "application/x-pem-file"),
            "server_key": ("server.key", io.BytesIO(key_pem), "application/x-pem-file"),
        }
        resp = api.post("/api/ssl/upload", files=files, cookies=admin_cookies)
        assert resp.status_code == 200
        data = resp.json()
        assert data["status"] == "uploaded"

        resp = api.get("/api/ssl/status", cookies=admin_cookies)
        data = resp.json()
        assert data["hasCerts"] is True
        assert data["enabled"] is True

        api.delete("/api/ssl", cookies=admin_cookies)

    def test_upload_missing_server_cert(self, api, admin_cookies):
        _, key_pem = generate_self_signed_cert()
        files = {
            "server_key": ("server.key", io.BytesIO(key_pem), "application/x-pem-file"),
        }
        resp = api.post("/api/ssl/upload", files=files, cookies=admin_cookies)
        assert resp.status_code == 400
        assert "server_cert" in resp.json()["error"]

    def test_upload_missing_server_key(self, api, admin_cookies):
        cert_pem, _ = generate_self_signed_cert()
        files = {
            "server_cert": ("server.crt", io.BytesIO(cert_pem), "application/x-pem-file"),
        }
        resp = api.post("/api/ssl/upload", files=files, cookies=admin_cookies)
        assert resp.status_code == 400
        assert "server_key" in resp.json()["error"]

    def test_upload_invalid_cert_pem(self, api, admin_cookies):
        files = {
            "server_cert": ("server.crt", io.BytesIO(b"not a cert"), "application/x-pem-file"),
            "server_key": ("server.key", io.BytesIO(b"not a key"), "application/x-pem-file"),
        }
        resp = api.post("/api/ssl/upload", files=files, cookies=admin_cookies)
        assert resp.status_code == 400
        assert "invalid" in resp.json()["error"].lower()

    def test_upload_invalid_key_pem(self, api, admin_cookies):
        cert_pem, _ = generate_self_signed_cert()
        files = {
            "server_cert": ("server.crt", io.BytesIO(cert_pem), "application/x-pem-file"),
            "server_key": ("server.key", io.BytesIO(b"not a key"), "application/x-pem-file"),
        }
        resp = api.post("/api/ssl/upload", files=files, cookies=admin_cookies)
        assert resp.status_code == 400
        assert "invalid" in resp.json()["error"].lower()

    def test_upload_with_ca_cert(self, api, admin_cookies):
        ca_cert_pem, ca_key_pem, ca_key, ca_cert = generate_ca_cert()

        from cryptography import x509
        from cryptography.x509.oid import NameOID
        from cryptography.hazmat.primitives import hashes, serialization
        from cryptography.hazmat.primitives.asymmetric import ec
        import datetime

        server_key = ec.generate_private_key(ec.SECP256R1())
        server_subject = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, "test-server")])
        server_cert = (
            x509.CertificateBuilder()
            .subject_name(server_subject)
            .issuer_name(ca_cert.subject)
            .public_key(server_key.public_key())
            .serial_number(x509.random_serial_number())
            .not_valid_before(datetime.datetime.now(datetime.timezone.utc))
            .not_valid_after(datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(days=365))
            .add_extension(x509.BasicConstraints(ca=False, path_length=None), critical=True)
            .sign(ca_key, hashes.SHA256())
        )
        server_cert_pem = server_cert.public_bytes(serialization.Encoding.PEM)
        server_key_pem = server_key.private_bytes(
            serialization.Encoding.PEM, serialization.PrivateFormat.TraditionalOpenSSL, serialization.NoEncryption(),
        )

        files = {
            "server_cert": ("server.crt", io.BytesIO(server_cert_pem), "application/x-pem-file"),
            "server_key": ("server.key", io.BytesIO(server_key_pem), "application/x-pem-file"),
            "ca_cert": ("root.crt", io.BytesIO(ca_cert_pem), "application/x-pem-file"),
        }
        resp = api.post("/api/ssl/upload", files=files, cookies=admin_cookies)
        assert resp.status_code == 200

        api.delete("/api/ssl", cookies=admin_cookies)

    def test_upload_ca_mismatch(self, api, admin_cookies):
        cert_pem, key_pem = generate_self_signed_cert()
        other_ca_pem, _, _, _ = generate_ca_cert("other-ca")

        files = {
            "server_cert": ("server.crt", io.BytesIO(cert_pem), "application/x-pem-file"),
            "server_key": ("server.key", io.BytesIO(key_pem), "application/x-pem-file"),
            "ca_cert": ("root.crt", io.BytesIO(other_ca_pem), "application/x-pem-file"),
        }
        resp = api.post("/api/ssl/upload", files=files, cookies=admin_cookies)
        assert resp.status_code == 400
        assert "not signed" in resp.json()["error"].lower()


class TestSSLEdgeCases:
    def test_generate_invalid_json(self, api, admin_cookies):
        resp = api.post("/api/ssl/generate", data="not json", cookies=admin_cookies)
        assert resp.status_code == 400

    def test_generate_default_validity(self, api, admin_cookies):
        resp = api.post(
            "/api/ssl/generate",
            json={"commonName": "test-server"},
            cookies=admin_cookies,
        )
        assert resp.status_code == 200

        resp = api.get("/api/ssl/status", cookies=admin_cookies)
        data = resp.json()
        assert data["hasCerts"] is True
        assert data["expiry"] != ""
        assert data["issuer"] != ""

        api.delete("/api/ssl", cookies=admin_cookies)

    def test_delete_when_enabled(self, api, admin_cookies):
        api.post(
            "/api/ssl/generate",
            json={"commonName": "pgmanager-server"},
            cookies=admin_cookies,
        )
        resp = api.get("/api/ssl/status", cookies=admin_cookies)
        assert resp.json()["enabled"] is True

        resp = api.delete("/api/ssl", cookies=admin_cookies)
        assert resp.status_code == 200

        resp = api.get("/api/ssl/status", cookies=admin_cookies)
        data = resp.json()
        assert data["hasCerts"] is False
        assert data["enabled"] is False
