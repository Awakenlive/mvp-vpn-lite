package quictransport

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"time"

	"mvp-vpn-lite/internal/packet"

	"github.com/quic-go/quic-go"
)

const mvpQUICALPN = "mvp-vpn-lite"

// ServerConfig contains the addresses and virtual IPs used by the demo server.
type ServerConfig struct {
	Listen0   string
	Listen1   string
	VirtualIP net.IP
	ClientIP  net.IP
}

// RunServer starts one QUIC listener per path and blocks until ctx is canceled
// or a listener fails.
func RunServer(ctx context.Context, cfg ServerConfig) error {
	if cfg.Listen0 == "" && cfg.Listen1 == "" {
		return errors.New("at least one QUIC listen address is required")
	}
	if cfg.VirtualIP.To4() == nil {
		return fmt.Errorf("virtual IP must be IPv4: %s", cfg.VirtualIP)
	}
	if cfg.ClientIP.To4() == nil {
		return fmt.Errorf("client IP must be IPv4: %s", cfg.ClientIP)
	}

	tlsConfig, err := generateSelfSignedTLSConfig()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 2)
	started := 0
	if cfg.Listen0 != "" {
		started++
		go serveQUICPath(ctx, 0, cfg.Listen0, tlsConfig.Clone(), cfg.VirtualIP, cfg.ClientIP, errCh)
	}
	if cfg.Listen1 != "" {
		started++
		go serveQUICPath(ctx, 1, cfg.Listen1, tlsConfig.Clone(), cfg.VirtualIP, cfg.ClientIP, errCh)
	}

	for i := 0; i < started; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			if err != nil {
				cancel()
				return err
			}
		}
	}

	return nil
}

func serveQUICPath(ctx context.Context, pathID int, listenAddr string, tlsConfig *tls.Config, virtualIP, clientIP net.IP, errCh chan<- error) {
	listener, err := quic.ListenAddr(listenAddr, tlsConfig, nil)
	if err != nil {
		errCh <- fmt.Errorf("path %d listen %s: %w", pathID, listenAddr, err)
		return
	}
	defer listener.Close()

	log.Printf("path %d listening on %s", pathID, listener.Addr())

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		conn, err := listener.Accept(ctx)
		if err != nil {
			if ctx.Err() != nil {
				errCh <- ctx.Err()
				return
			}
			errCh <- fmt.Errorf("path %d accept connection: %w", pathID, err)
			return
		}

		log.Printf("path %d accepted connection from %s", pathID, conn.RemoteAddr())
		go handleQUICConnection(ctx, pathID, conn, virtualIP, clientIP)
	}
}

func handleQUICConnection(ctx context.Context, pathID int, conn *quic.Conn, virtualIP, clientIP net.IP) {
	defer conn.CloseWithError(0, "server handler stopped")

	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("path %d accept stream: %v", pathID, err)
		}
		return
	}
	defer stream.Close()

	handleQUICStream(pathID, stream, virtualIP, clientIP)
}

func handleQUICStream(pathID int, stream io.ReadWriter, virtualIP, clientIP net.IP) {
	for {
		rawPacket, err := ReadFrame(stream)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Printf("path %d read frame: %v", pathID, err)
			}
			return
		}

		log.Printf("path %d received packet length=%d", pathID, len(rawPacket))

		reply, err := packet.BuildICMPEchoReply(rawPacket, virtualIP, clientIP)
		if err != nil {
			log.Printf("path %d ignored packet: %v", pathID, err)
			continue
		}

		if err := WriteFrame(stream, reply); err != nil {
			log.Printf("path %d write reply: %v", pathID, err)
			return
		}

		log.Printf("path %d sent reply length=%d", pathID, len(reply))
	}
}

func generateSelfSignedTLSConfig() (*tls.Config, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate TLS key: %w", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, fmt.Errorf("generate TLS serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: "mvp-vpn-lite",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create TLS certificate: %w", err)
	}

	keyDER := x509.MarshalPKCS1PrivateKey(key)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyDER})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("load TLS key pair: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{mvpQUICALPN},
		MinVersion:   tls.VersionTLS13,
	}, nil
}
