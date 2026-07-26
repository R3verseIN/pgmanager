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
	"strings"
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
	Enabled    bool   `json:"enabled"`
	HasCerts   bool   `json:"hasCerts"`
	Expiry     string `json:"expiry,omitempty"`
	Issuer     string `json:"issuer,omitempty"`
	SelfSigned bool   `json:"selfSigned"`
	PgBouncerSSL bool `json:"pgBouncerSSL"`
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

func (sh *SSLHandler) sslPrefPath() string {
	return filepath.Join(sh.dataDir, "pgmanager-ssl-enabled")
}

func (sh *SSLHandler) pgbouncerSSLPrefPath() string {
	return filepath.Join(sh.dataDir, "pgmanager-pgbouncer-ssl")
}

func (sh *SSLHandler) pgbouncerRestartSignalPath() string {
	return "/etc/pgbouncer/shared/pgbouncer-restart-signal"
}

// GET /api/ssl/status
func (sh *SSLHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	certPath := sh.certPath("server.crt")
	keyPath := sh.certPath("server.key")

	status := sslStatus{
		Enabled:      sh.isSSLPostgresEnabled(),
		HasCerts:     sh.fileExists(certPath) && sh.fileExists(keyPath),
		PgBouncerSSL: sh.isPgBouncerSSLEnabled(),
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
		"message": "SSL certificates generated and enabled. New certificates will take effect on next PostgreSQL restart.",
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
		"message": "SSL certificates uploaded and enabled. New certificates will take effect on next PostgreSQL restart.",
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
		"message": "SSL certificates removed and SSL disabled. Change will take effect on next PostgreSQL restart.",
	})
}

// isSSLPostgresEnabled checks if SSL is configured by reading the preference file
// and verifying certs exist. More reliable than SHOW ssl which shows the configured
// value, not the actual runtime state.
func (sh *SSLHandler) isSSLPostgresEnabled() bool {
	prefPath := sh.sslPrefPath()
	if sh.fileExists(prefPath) {
		data, err := os.ReadFile(prefPath)
		if err == nil && string(data) == "off" {
			return false
		}
	}
	// Default: check if certs exist (first boot defaults to on)
	certPath := sh.certPath("server.crt")
	keyPath := sh.certPath("server.key")
	return sh.fileExists(certPath) && sh.fileExists(keyPath)
}

func (sh *SSLHandler) enableSSLPostgres() error {
	certPath := sh.certPath("server.crt")
	keyPath := sh.certPath("server.key")
	caPath := sh.certPath("root.crt")

	// Write SSL preference file
	if err := os.WriteFile(sh.sslPrefPath(), []byte("on"), 0644); err != nil {
		return fmt.Errorf("write ssl preference: %w", err)
	}

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

	// Update HBA rules to require SSL for external connections
	sh.updateHBAForSSL(true)

	return nil
}

func (sh *SSLHandler) disableSSLPostgres() error {
	// Write SSL preference file
	if err := os.WriteFile(sh.sslPrefPath(), []byte("off"), 0644); err != nil {
		return fmt.Errorf("write ssl preference: %w", err)
	}

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

	// Update HBA rules to allow non-SSL for external connections
	sh.updateHBAForSSL(false)

	return nil
}

func (sh *SSLHandler) updateHBAForSSL(requireSSL bool) {
	hbaPath := filepath.Join(sh.dataDir, "pg_hba.conf")
	data, err := os.ReadFile(hbaPath)
	if err != nil {
		return
	}
	content := string(data)

	if requireSSL {
		content = replaceExternalHBA(content, "hostssl")
	} else {
		content = replaceExternalHBA(content, "host")
	}

	os.WriteFile(hbaPath, []byte(content), 0644)
}

func replaceExternalHBA(content, newType string) string {
	lines := splitLines(content)
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed[0] == '#' {
			result = append(result, line)
			continue
		}
		// Match external rules: host/hostssl all all 0.0.0.0/0 or ::0/0
		if (strings.HasPrefix(trimmed, "host ") || strings.HasPrefix(trimmed, "hostssl ")) &&
			(strings.Contains(trimmed, " all 0.0.0.0/0 ") || strings.Contains(trimmed, " all ::0/0 ")) &&
			!strings.Contains(trimmed, "172.16.") && !strings.Contains(trimmed, "192.168.") && !strings.Contains(trimmed, "10.") {
			// Replace the type prefix
			idx := strings.Index(trimmed, " ")
			line = newType + trimmed[idx:]
		}
		result = append(result, line)
	}
	return joinLines(result)
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i+1])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func joinLines(lines []string) string {
	result := ""
	for _, line := range lines {
		result += line
	}
	return result
}

func (sh *SSLHandler) isPgBouncerSSLEnabled() bool {
	prefPath := sh.pgbouncerSSLPrefPath()
	if sh.fileExists(prefPath) {
		data, err := os.ReadFile(prefPath)
		if err == nil {
			return string(data) != "off"
		}
	}
	// Default: enabled if certs exist
	return sh.fileExists(sh.certPath("server.crt")) && sh.fileExists(sh.certPath("server.key"))
}

// POST /api/ssl/pgbouncer — toggle PgBouncer client TLS
func (sh *SSLHandler) TogglePgBouncerSSL(w http.ResponseWriter, r *http.Request) {
	var req pgbouncerSSLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Enabled && !sh.fileExists(sh.certPath("server.crt")) {
		writeError(w, http.StatusBadRequest, "SSL certificates must be generated or uploaded before enabling PgBouncer SSL")
		return
	}

	// Write preference file
	val := "on"
	if !req.Enabled {
		val = "off"
	}
	if err := os.WriteFile(sh.pgbouncerSSLPrefPath(), []byte(val), 0644); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save PgBouncer SSL preference: "+err.Error())
		return
	}

	// Write restart signal file — PgBouncer watcher will pick it up
	if err := os.WriteFile(sh.pgbouncerRestartSignalPath(), []byte("1"), 0644); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to write restart signal: "+err.Error())
		return
	}

	state := "disabled"
	if req.Enabled {
		state = "enabled"
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  state,
		"message": fmt.Sprintf("PgBouncer SSL %s. Restarting...", state),
	})
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
