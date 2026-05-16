package quictransport

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
)

func serverTLSConfig(certFile, keyFile, clientCAFile string) (*tls.Config, error) {
	var cfg *tls.Config
	if certFile == "" && keyFile == "" {
		generated, err := generateSelfSignedTLSConfig()
		if err != nil {
			return nil, err
		}
		cfg = generated
	} else if certFile == "" || keyFile == "" {
		return nil, errors.New("both TLS cert and key files are required")
	} else {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("load TLS key pair: %w", err)
		}

		cfg = &tls.Config{
			Certificates: []tls.Certificate{cert},
			NextProtos:   []string{mvpQUICALPN},
			MinVersion:   tls.VersionTLS13,
		}
	}

	if clientCAFile != "" {
		clientCAs, err := loadCertPool(clientCAFile, "client CA")
		if err != nil {
			return nil, err
		}
		cfg.ClientCAs = clientCAs
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return cfg, nil
}

func clientTLSConfig(caFile, serverName, certFile, keyFile string) (*tls.Config, error) {
	cfg := &tls.Config{
		NextProtos: []string{mvpQUICALPN},
		MinVersion: tls.VersionTLS13,
		ServerName: serverName,
	}
	if caFile == "" {
		cfg.InsecureSkipVerify = true // demo mode for the ephemeral self-signed server cert
	} else {
		rootCAs, err := loadCertPool(caFile, "CA")
		if err != nil {
			return nil, err
		}
		cfg.RootCAs = rootCAs
	}

	if certFile == "" && keyFile == "" {
		return cfg, nil
	}
	if certFile == "" || keyFile == "" {
		return nil, errors.New("both client TLS cert and key files are required")
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load client TLS key pair: %w", err)
	}
	cfg.Certificates = []tls.Certificate{cert}

	return cfg, nil
}

func loadCertPool(path, label string) (*x509.CertPool, error) {
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s certificate: %w", label, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		return nil, fmt.Errorf("%s certificate file %s does not contain PEM certificates", label, path)
	}
	return pool, nil
}
