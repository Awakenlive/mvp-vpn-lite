package quictransport

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClientTLSConfigDefaultsToDemoInsecureMode(t *testing.T) {
	t.Parallel()

	cfg, err := clientTLSConfig("", "")
	if err != nil {
		t.Fatalf("clientTLSConfig() error = %v", err)
	}
	if !cfg.InsecureSkipVerify {
		t.Fatal("InsecureSkipVerify = false, want true for demo mode")
	}
	if len(cfg.NextProtos) != 1 || cfg.NextProtos[0] != mvpQUICALPN {
		t.Fatalf("NextProtos = %v, want %s", cfg.NextProtos, mvpQUICALPN)
	}
}

func TestClientTLSConfigLoadsCAFile(t *testing.T) {
	t.Parallel()

	certPEM, _ := generateTestCertificatePEM(t)
	certFile := filepath.Join(t.TempDir(), "server.crt")
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatalf("WriteFile(cert) error = %v", err)
	}

	cfg, err := clientTLSConfig(certFile, "localhost")
	if err != nil {
		t.Fatalf("clientTLSConfig() error = %v", err)
	}
	if cfg.InsecureSkipVerify {
		t.Fatal("InsecureSkipVerify = true, want false with CA file")
	}
	if cfg.RootCAs == nil {
		t.Fatal("RootCAs = nil, want loaded pool")
	}
	if cfg.ServerName != "localhost" {
		t.Fatalf("ServerName = %q, want localhost", cfg.ServerName)
	}
}

func TestServerTLSConfigRejectsPartialFiles(t *testing.T) {
	t.Parallel()

	if _, err := serverTLSConfig("server.crt", ""); err == nil {
		t.Fatal("serverTLSConfig(cert only) error = nil, want error")
	}
	if _, err := serverTLSConfig("", "server.key"); err == nil {
		t.Fatal("serverTLSConfig(key only) error = nil, want error")
	}
}

func TestServerTLSConfigLoadsKeyPair(t *testing.T) {
	t.Parallel()

	certPEM, keyPEM := generateTestCertificatePEM(t)
	dir := t.TempDir()
	certFile := filepath.Join(dir, "server.crt")
	keyFile := filepath.Join(dir, "server.key")
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatalf("WriteFile(cert) error = %v", err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatalf("WriteFile(key) error = %v", err)
	}

	cfg, err := serverTLSConfig(certFile, keyFile)
	if err != nil {
		t.Fatalf("serverTLSConfig() error = %v", err)
	}
	if len(cfg.Certificates) != 1 {
		t.Fatalf("Certificates = %d, want 1", len(cfg.Certificates))
	}
	if len(cfg.NextProtos) != 1 || cfg.NextProtos[0] != mvpQUICALPN {
		t.Fatalf("NextProtos = %v, want %s", cfg.NextProtos, mvpQUICALPN)
	}
}

func generateTestCertificatePEM(t *testing.T) ([]byte, []byte) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certPEM, keyPEM
}
