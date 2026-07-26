package handler

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type SSLHandler struct {
	pool    *pgxpool.Pool
	dataDir string
}

func NewSSLHandler(pool *pgxpool.Pool, dataDir string) *SSLHandler {
	return &SSLHandler{pool: pool, dataDir: dataDir}
}

type sslStatus struct {
	Enabled        bool   `json:"enabled"`
	HasCerts       bool   `json:"hasCerts"`
	Expiry         string `json:"expiry,omitempty"`
	Issuer         string `json:"issuer,omitempty"`
	SelfSigned     bool   `json:"selfSigned"`
	PgBouncerSSL   bool   `json:"pgBouncerSSL"`
	PendingRestart bool   `json:"pendingRestart"`
}

type pgbouncerSSLRequest struct {
	Enabled bool `json:"enabled"`
}

type generateRequest struct {
	CommonName   string `json:"commonName"`
	ValidityDays int    `json:"validityDays"`
}

func (sh *SSLHandler) certPath(name string) string {
	return filepath.Join(sh.dataDir, name)
}

func (sh *SSLHandler) fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// GET /api/ssl/status
func (sh *SSLHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	certPath := sh.certPath("server.crt")
	keyPath := sh.certPath("server.key")

	status := sslStatus{
		Enabled:        sh.isSSLPostgresEnabled(),
		HasCerts:       sh.fileExists(certPath) && sh.fileExists(keyPath),
		PgBouncerSSL:   sh.isPgBouncerSSLEnabled(),
		PendingRestart: sh.fileExists(filepath.Join(sh.dataDir, "pgbouncer-ssl-pending")),
	}

	if status.HasCerts {
		cert := sh.readCert(certPath)
		if cert != nil {
			status.Expiry = cert.NotAfter.Format(time.RFC3339)
			status.Issuer = cert.Issuer.CommonName
			status.SelfSigned = cert.Issuer.CommonName == cert.Subject.CommonName
		}
	}

	writeJSON(w, http.StatusOK, status)
}

// POST /api/ssl/generate
func (sh *SSLHandler) GenerateCerts(w http.ResponseWriter, r *http.Request) {
	var req generateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.CommonName == "" {
		req.CommonName = "pgmanager-server"
	}
	if req.ValidityDays <= 0 {
		req.ValidityDays = 1825 // 5 years
	}

	caKey, caCert, caCertBytes, err := sh.generateCA(req.CommonName+"-ca", req.ValidityDays*2)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate CA: "+err.Error())
		return
	}

	serverKey, serverCertBytes, err := sh.generateServerCert(caCert, caKey, req.CommonName, req.ValidityDays)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate server cert: "+err.Error())
		return
	}

	if err := sh.writeCertFiles(caCertBytes, caKey, serverCertBytes, serverKey); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to write cert files: "+err.Error())
		return
	}

	if err := sh.enableSSLPostgres(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to enable SSL in PostgreSQL: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "generated",
		"message": "SSL certificates generated and enabled",
	})
}

// POST /api/ssl/upload
func (sh *SSLHandler) UploadCerts(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse form: "+err.Error())
		return
	}

	serverCertFile, _, err := r.FormFile("server_cert")
	if err != nil {
		writeError(w, http.StatusBadRequest, "server_cert file is required")
		return
	}
	defer serverCertFile.Close()

	serverKeyFile, _, err := r.FormFile("server_key")
	if err != nil {
		writeError(w, http.StatusBadRequest, "server_key file is required")
		return
	}
	defer serverKeyFile.Close()

	serverCertPEM, err := io.ReadAll(serverCertFile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read server cert")
		return
	}
	serverKeyPEM, err := io.ReadAll(serverKeyFile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read server key")
		return
	}

	serverCertBlock, _ := pem.Decode(serverCertPEM)
	if serverCertBlock == nil {
		writeError(w, http.StatusBadRequest, "invalid server certificate PEM")
		return
	}
	serverCert, err := x509.ParseCertificate(serverCertBlock.Bytes)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid server certificate: "+err.Error())
		return
	}

	serverKeyBlock, _ := pem.Decode(serverKeyPEM)
	if serverKeyBlock == nil {
		writeError(w, http.StatusBadRequest, "invalid server key PEM")
		return
	}
	serverKey, err := x509.ParseECPrivateKey(serverKeyBlock.Bytes)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid server key (must be ECDSA): "+err.Error())
		return
	}
	_ = serverKey

	var caCertPEMBytes []byte
	caCertFile, _, err := r.FormFile("ca_cert")
	if err == nil {
		defer caCertFile.Close()
		caCertPEMBytes, _ = io.ReadAll(caCertFile)
		if caCertBlock, _ := pem.Decode(caCertPEMBytes); caCertBlock != nil {
			if caCert, parseErr := x509.ParseCertificate(caCertBlock.Bytes); parseErr == nil {
				opts := x509.VerifyOptions{
					Roots:     x509.NewCertPool(),
					KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
				}
				opts.Roots.AddCert(caCert)
				if _, verifyErr := serverCert.Verify(opts); verifyErr != nil {
					writeError(w, http.StatusBadRequest, "server certificate is not signed by the provided CA")
					return
				}
			}
		}
	}

	if err := sh.writeUploadedFiles(serverCertPEM, serverKeyPEM, caCertPEMBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to write cert files: "+err.Error())
		return
	}

	if err := sh.enableSSLPostgres(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to enable SSL in PostgreSQL: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "uploaded",
		"message": "SSL certificates uploaded and enabled",
	})
}

// GET /api/ssl/download
func (sh *SSLHandler) DownloadCA(w http.ResponseWriter, r *http.Request) {
	caPath := sh.certPath("root.crt")
	if !sh.fileExists(caPath) {
		writeError(w, http.StatusNotFound, "CA certificate not found")
		return
	}

	w.Header().Set("Content-Disposition", "attachment; filename=root.crt")
	http.ServeFile(w, r, caPath)
}

// DELETE /api/ssl
func (sh *SSLHandler) DeleteCerts(w http.ResponseWriter, r *http.Request) {
	if err := sh.disableSSLPostgres(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to disable SSL in PostgreSQL: "+err.Error())
		return
	}

	for _, name := range []string{"server.crt", "server.key", "root.crt", "root.key"} {
		os.Remove(sh.certPath(name))
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "deleted",
		"message": "SSL certificates removed and SSL disabled",
	})
}

func (sh *SSLHandler) isSSLPostgresEnabled() bool {
	var setting string
	err := sh.pool.QueryRow(context.Background(), "SHOW ssl").Scan(&setting)
	if err != nil {
		return false
	}
	return setting == "on"
}

func (sh *SSLHandler) enableSSLPostgres() error {
	certPath := sh.certPath("server.crt")
	keyPath := sh.certPath("server.key")
	caPath := sh.certPath("root.crt")

	commands := []string{
		"ALTER SYSTEM SET ssl = 'on';",
		fmt.Sprintf("ALTER SYSTEM SET ssl_cert_file = '%s';", certPath),
		fmt.Sprintf("ALTER SYSTEM SET ssl_key_file = '%s';", keyPath),
	}
	if sh.fileExists(caPath) {
		commands = append(commands, fmt.Sprintf("ALTER SYSTEM SET ssl_ca_file = '%s';", caPath))
	}
	commands = append(commands, "SELECT pg_reload_conf();")

	for _, cmd := range commands {
		if _, err := sh.pool.Exec(context.Background(), cmd); err != nil {
			return fmt.Errorf("exec %q: %w", cmd, err)
		}
	}
	return nil
}

func (sh *SSLHandler) disableSSLPostgres() error {
	commands := []string{
		"ALTER SYSTEM SET ssl = 'off';",
		"ALTER SYSTEM RESET ssl_cert_file;",
		"ALTER SYSTEM RESET ssl_key_file;",
		"ALTER SYSTEM RESET ssl_ca_file;",
		"SELECT pg_reload_conf();",
	}
	for _, cmd := range commands {
		if _, err := sh.pool.Exec(context.Background(), cmd); err != nil {
			return fmt.Errorf("exec %q: %w", cmd, err)
		}
	}
	return nil
}

func (sh *SSLHandler) generateCA(cn string, validityDays int) (*ecdsa.PrivateKey, *x509.Certificate, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, nil, err
	}

	tmpl := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   cn,
			Organization: []string{"pgmanager"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Duration(validityDays) * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, nil, err
	}

	return key, tmpl, certBytes, nil
}

func (sh *SSLHandler) generateServerCert(caCert *x509.Certificate, caKey *ecdsa.PrivateKey, cn string, validityDays int) (*ecdsa.PrivateKey, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}

	tmpl := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   cn,
			Organization: []string{"pgmanager"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Duration(validityDays) * 24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}

	return key, certBytes, nil
}

func (sh *SSLHandler) writeCertFiles(caCertBytes []byte, caKey *ecdsa.PrivateKey, serverCertBytes []byte, serverKey *ecdsa.PrivateKey) error {
	caCertFile, err := os.Create(sh.certPath("root.crt"))
	if err != nil {
		return err
	}
	defer caCertFile.Close()
	pem.Encode(caCertFile, &pem.Block{Type: "CERTIFICATE", Bytes: caCertBytes})

	caKeyFile, err := os.OpenFile(sh.certPath("root.key"), os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer caKeyFile.Close()
	caKeyBytes, _ := x509.MarshalECPrivateKey(caKey)
	pem.Encode(caKeyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: caKeyBytes})

	serverCertFile, err := os.Create(sh.certPath("server.crt"))
	if err != nil {
		return err
	}
	defer serverCertFile.Close()
	pem.Encode(serverCertFile, &pem.Block{Type: "CERTIFICATE", Bytes: serverCertBytes})

	serverKeyFile, err := os.OpenFile(sh.certPath("server.key"), os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer serverKeyFile.Close()
	serverKeyBytes, _ := x509.MarshalECPrivateKey(serverKey)
	pem.Encode(serverKeyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyBytes})

	return nil
}

func (sh *SSLHandler) writeUploadedFiles(serverCertPEM, serverKeyPEM, caCertPEM []byte) error {
	if err := os.WriteFile(sh.certPath("server.crt"), serverCertPEM, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(sh.certPath("server.key"), serverKeyPEM, 0600); err != nil {
		return err
	}
	if len(caCertPEM) > 0 {
		if err := os.WriteFile(sh.certPath("root.crt"), caCertPEM, 0644); err != nil {
			return err
		}
	}
	return nil
}

func (sh *SSLHandler) readCert(path string) *x509.Certificate {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil
	}
	return cert
}

// pgbouncerSSLConfigPath is where PgBouncer SSL toggle state is stored
var pgbouncerSSLConfigPath = "/etc/pgbouncer/shared/pgbouncer-ssl.conf"

func (sh *SSLHandler) isPgBouncerSSLEnabled() bool {
	data, err := os.ReadFile(pgbouncerSSLConfigPath)
	if err != nil {
		return false
	}
	return string(data) == "on"
}

func (sh *SSLHandler) setPgBouncerSSL(enabled bool) error {
	val := "off"
	if enabled {
		val = "on"
	}
	if err := os.WriteFile(pgbouncerSSLConfigPath, []byte(val), 0644); err != nil {
		return err
	}

	// Write pending restart flag so UI can show restart required
	if err := os.WriteFile(filepath.Join(sh.dataDir, "pgbouncer-ssl-pending"), []byte("1"), 0644); err != nil {
		return err
	}

	return nil
}

func (sh *SSLHandler) clearPendingRestart() {
	os.Remove(filepath.Join(sh.dataDir, "pgbouncer-ssl-pending"))
}

// POST /api/ssl/pgbouncer — toggle PgBouncer client TLS
func (sh *SSLHandler) TogglePgBouncerSSL(w http.ResponseWriter, r *http.Request) {
	var req pgbouncerSSLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// PgBouncer client_tls_sslmode is a startup-only parameter — requires restart
	if req.Enabled && !sh.fileExists(sh.certPath("server.crt")) {
		writeError(w, http.StatusBadRequest, "SSL certificates must be generated or uploaded before enabling PgBouncer SSL")
		return
	}

	if err := sh.setPgBouncerSSL(req.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save PgBouncer SSL config: "+err.Error())
		return
	}

	state := "disabled"
	if req.Enabled {
		state = "enabled"
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  state,
		"message": fmt.Sprintf("PgBouncer SSL %s. Restart the pgbouncer container to apply.", state),
	})
}

// POST /api/ssl/pgbouncer/apply — called after pgbouncer container restart to clear pending flag
func (sh *SSLHandler) ApplyPgBouncerSSL(w http.ResponseWriter, r *http.Request) {
	sh.clearPendingRestart()
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "applied",
		"message": "PgBouncer SSL config applied",
	})
}
