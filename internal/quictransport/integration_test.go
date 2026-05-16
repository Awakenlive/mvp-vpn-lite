package quictransport

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunClientWithMutualTLS(t *testing.T) {
	t.Parallel()

	certPEM, keyPEM := generateTestCertificatePEM(t)
	dir := t.TempDir()
	certFile := filepath.Join(dir, "peer.crt")
	keyFile := filepath.Join(dir, "peer.key")
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatalf("WriteFile(cert) error = %v", err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatalf("WriteFile(key) error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr := freeUDPAddr(t)
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- RunServer(ctx, ServerConfig{
			Listen0:        addr,
			TLSCertFile:    certFile,
			TLSKeyFile:     keyFile,
			ClientCAFile:   certFile,
			VirtualIP:      net.IPv4(10, 8, 0, 1),
			ClientIP:       net.IPv4(10, 8, 0, 2),
			StatsInterval:  0,
			TUNAllowedCIDR: "10.8.0.0/24",
		})
	}()
	clientCfg := ClientConfig{
		Server0:        addr,
		CAFile:         certFile,
		ServerName:     "localhost",
		ClientCertFile: certFile,
		ClientKeyFile:  keyFile,
		VirtualIP:      net.IPv4(10, 8, 0, 1),
		ClientIP:       net.IPv4(10, 8, 0, 2),
		Identifier:     0x4d56,
		Count:          1,
		Payload:        []byte("mtls"),
		RequestTimeout: 2 * time.Second,
		StatsInterval:  0,
	}
	var err error
	deadline := time.Now().Add(2 * time.Second)
	for {
		err = RunClient(ctx, clientCfg)
		if err == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("RunClient(mTLS) error = %v", err)
	}

	cancel()
	select {
	case err := <-serverErrCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("RunServer() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunServer() did not stop after context cancellation")
	}
}

func freeUDPAddr(t *testing.T) string {
	t.Helper()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket() error = %v", err)
	}
	defer conn.Close()
	return conn.LocalAddr().String()
}
