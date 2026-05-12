package quictransport

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
)

func serverTLSConfig(certFile, keyFile string) (*tls.Config, error) {
	if certFile == "" && keyFile == "" {
		return generateSelfSignedTLSConfig()
	}
	if certFile == "" || keyFile == "" {
		return nil, errors.New("both TLS cert and key files are required")
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load TLS key pair: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{mvpQUICALPN},
		MinVersion:   tls.VersionTLS13,
	}, nil
}

func clientTLSConfig(caFile, serverName string) (*tls.Config, error) {
	cfg := &tls.Config{
		NextProtos: []string{mvpQUICALPN},
		MinVersion: tls.VersionTLS13,
		ServerName: serverName,
	}
	if caFile == "" {
		cfg.InsecureSkipVerify = true // demo mode for the ephemeral self-signed server cert
		return cfg, nil
	}

	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read CA certificate: %w", err)
	}
	rootCAs := x509.NewCertPool()
	if !rootCAs.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("CA certificate file %s does not contain PEM certificates", caFile)
	}

	cfg.RootCAs = rootCAs
	return cfg, nil
}
