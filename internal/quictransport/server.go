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
	"os"
	"sync"
	"time"

	"mvp-vpn-lite/internal/packet"
	"mvp-vpn-lite/internal/stats"
	"mvp-vpn-lite/internal/tun"

	"github.com/quic-go/quic-go"
)

const mvpQUICALPN = "mvp-vpn-lite"

// ServerConfig contains the addresses and virtual IPs used by the demo server.
type ServerConfig struct {
	Listen0       string
	Listen1       string
	TLSCertFile   string
	TLSKeyFile    string
	VirtualIP     net.IP
	ClientIP      net.IP
	StatsInterval time.Duration
	UseTUN        bool
	DeviceName    string
}

// RunServer starts one QUIC listener per path and blocks until ctx is canceled
// or a listener fails.
func RunServer(ctx context.Context, cfg ServerConfig) error {
	if cfg.Listen0 == "" && cfg.Listen1 == "" {
		return errors.New("at least one QUIC listen address is required")
	}
	if !cfg.UseTUN {
		if cfg.VirtualIP.To4() == nil {
			return fmt.Errorf("virtual IP must be IPv4: %s", cfg.VirtualIP)
		}
		if cfg.ClientIP.To4() == nil {
			return fmt.Errorf("client IP must be IPv4: %s", cfg.ClientIP)
		}
	}

	tlsConfig, err := serverTLSConfig(cfg.TLSCertFile, cfg.TLSKeyFile)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var counters stats.Counters
	go stats.LogEvery(ctx, cfg.StatsInterval, &counters, log.Printf)
	defer func() {
		log.Printf("stats final: %s", counters.Snapshot())
	}()

	if cfg.UseTUN {
		return runTUNServer(ctx, cfg, tlsConfig, &counters)
	}

	errCh := make(chan error, 2)
	started := 0
	if cfg.Listen0 != "" {
		started++
		go serveQUICPath(ctx, 0, cfg.Listen0, tlsConfig.Clone(), cfg.VirtualIP, cfg.ClientIP, &counters, errCh)
	}
	if cfg.Listen1 != "" {
		started++
		go serveQUICPath(ctx, 1, cfg.Listen1, tlsConfig.Clone(), cfg.VirtualIP, cfg.ClientIP, &counters, errCh)
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

func runTUNServer(ctx context.Context, cfg ServerConfig, tlsConfig *tls.Config, counters *stats.Counters) error {
	device, err := tun.Open(cfg.DeviceName)
	if err != nil {
		return err
	}
	defer device.Close()
	log.Printf("opened TUN device %s", device.Name())

	go func() {
		<-ctx.Done()
		_ = device.Close()
	}()

	session := newServerTUNSession(device, counters)
	errCh := make(chan error, 3)
	go session.forwardDevicePackets(ctx, errCh)

	if cfg.Listen0 != "" {
		go serveTUNQUICPath(ctx, 0, cfg.Listen0, tlsConfig.Clone(), session, counters, errCh)
	}
	if cfg.Listen1 != "" {
		go serveTUNQUICPath(ctx, 1, cfg.Listen1, tlsConfig.Clone(), session, counters, errCh)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			if err != nil {
				return err
			}
		}
	}
}

func serveQUICPath(ctx context.Context, pathID int, listenAddr string, tlsConfig *tls.Config, virtualIP, clientIP net.IP, counters *stats.Counters, errCh chan<- error) {
	listener, err := quic.ListenAddr(listenAddr, tlsConfig, nil)
	if err != nil {
		counters.AddError()
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
			counters.AddError()
			errCh <- fmt.Errorf("path %d accept connection: %w", pathID, err)
			return
		}

		log.Printf("path %d accepted connection from %s", pathID, conn.RemoteAddr())
		go handleQUICConnection(ctx, pathID, conn, virtualIP, clientIP, counters)
	}
}

func serveTUNQUICPath(ctx context.Context, pathID int, listenAddr string, tlsConfig *tls.Config, session *serverTUNSession, counters *stats.Counters, errCh chan<- error) {
	listener, err := quic.ListenAddr(listenAddr, tlsConfig, nil)
	if err != nil {
		counters.AddError()
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
				return
			}
			counters.AddError()
			errCh <- fmt.Errorf("path %d accept connection: %w", pathID, err)
			return
		}

		log.Printf("path %d accepted connection from %s", pathID, conn.RemoteAddr())
		go handleTUNQUICConnection(ctx, pathID, conn, session, counters)
	}
}

func handleQUICConnection(ctx context.Context, pathID int, conn *quic.Conn, virtualIP, clientIP net.IP, counters *stats.Counters) {
	defer conn.CloseWithError(0, "server handler stopped")

	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		if ctx.Err() == nil {
			counters.AddError()
			log.Printf("path %d accept stream: %v", pathID, err)
		}
		return
	}
	defer stream.Close()

	handleQUICStream(pathID, stream, virtualIP, clientIP, counters)
}

func handleTUNQUICConnection(ctx context.Context, pathID int, conn *quic.Conn, session *serverTUNSession, counters *stats.Counters) {
	defer conn.CloseWithError(0, "server TUN handler stopped")

	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		if ctx.Err() == nil {
			counters.AddError()
			log.Printf("path %d accept stream: %v", pathID, err)
		}
		return
	}
	defer stream.Close()

	session.addPath(pathID, stream)
	defer session.removePath(stream)

	for {
		rawPacket, err := ReadFrame(stream)
		if err != nil {
			if ctx.Err() != nil || isGracefulStreamEnd(err) {
				return
			}
			counters.AddError()
			log.Printf("path %d read packet: %v", pathID, err)
			return
		}

		if err := session.writeDevice(rawPacket); err != nil {
			if ctx.Err() != nil || errors.Is(err, os.ErrClosed) {
				return
			}
			counters.AddError()
			log.Printf("path %d write TUN packet: %v", pathID, err)
			return
		}
		counters.AddRX(len(rawPacket))

		log.Printf("path %d wrote TUN packet length=%d", pathID, len(rawPacket))
	}
}

func handleQUICStream(pathID int, stream io.ReadWriter, virtualIP, clientIP net.IP, counters *stats.Counters) {
	for {
		rawPacket, err := ReadFrame(stream)
		if err != nil {
			if !isGracefulStreamEnd(err) {
				counters.AddError()
				log.Printf("path %d read frame: %v", pathID, err)
			}
			return
		}
		counters.AddRX(len(rawPacket))

		log.Printf("path %d received packet length=%d", pathID, len(rawPacket))

		reply, err := packet.BuildICMPEchoReply(rawPacket, virtualIP, clientIP)
		if err != nil {
			counters.AddDrop()
			log.Printf("path %d ignored packet: %v", pathID, err)
			continue
		}

		if err := WriteFrame(stream, reply); err != nil {
			counters.AddError()
			log.Printf("path %d write reply: %v", pathID, err)
			return
		}
		counters.AddTX(len(reply))

		log.Printf("path %d sent reply length=%d", pathID, len(reply))
	}
}

type serverStream interface {
	io.ReadWriter
	Close() error
}

type serverPath struct {
	id     int
	stream serverStream
}

type serverTUNSession struct {
	device        packetDevice
	counters      *stats.Counters
	deviceWriteMu sync.Mutex
	pathsMu       sync.Mutex
	paths         []serverPath
	nextPath      int
}

func newServerTUNSession(device packetDevice, counters *stats.Counters) *serverTUNSession {
	return &serverTUNSession{
		device:   device,
		counters: counters,
	}
}

func (s *serverTUNSession) addPath(pathID int, stream serverStream) {
	s.pathsMu.Lock()
	defer s.pathsMu.Unlock()

	s.paths = append(s.paths, serverPath{id: pathID, stream: stream})
	log.Printf("path %d joined TUN session; active paths=%d", pathID, len(s.paths))
}

func (s *serverTUNSession) removePath(stream serverStream) {
	s.pathsMu.Lock()
	defer s.pathsMu.Unlock()

	for i, path := range s.paths {
		if path.stream == stream {
			s.paths = append(s.paths[:i], s.paths[i+1:]...)
			if len(s.paths) == 0 {
				s.nextPath = 0
			} else if s.nextPath >= len(s.paths) {
				s.nextPath %= len(s.paths)
			}
			log.Printf("path %d left TUN session; active paths=%d", path.id, len(s.paths))
			return
		}
	}
}

func (s *serverTUNSession) writeDevice(packet []byte) error {
	s.deviceWriteMu.Lock()
	defer s.deviceWriteMu.Unlock()

	_, err := s.device.Write(packet)
	return err
}

func (s *serverTUNSession) forwardDevicePackets(ctx context.Context, errCh chan<- error) {
	buffer := make([]byte, MaxFrameSize)
	retryableReadErrors := 0
	for {
		n, err := s.device.Read(buffer)
		if err != nil {
			if isClosedPacketDeviceError(ctx, err) {
				return
			}
			if isRetryablePacketDeviceReadError(err) && retryableReadErrors < maxRetryablePacketDeviceReads {
				retryableReadErrors++
				s.counters.AddError()
				if shouldLogRetryablePacketDeviceRead(retryableReadErrors) {
					log.Printf("retrying TUN read after transient error: %v", err)
				}
				if !sleepContext(ctx, retryablePacketDeviceReadDelay) {
					return
				}
				continue
			}
			s.counters.AddError()
			errCh <- fmt.Errorf("read TUN packet: %w", err)
			return
		}
		retryableReadErrors = 0
		if n == 0 {
			continue
		}

		packet := append([]byte(nil), buffer[:n]...)
		if err := s.writeNextPath(packet); err != nil {
			s.counters.AddDrop()
			log.Printf("dropped TUN packet length=%d: %v", len(packet), err)
			continue
		}
		s.counters.AddTX(len(packet))
	}
}

func (s *serverTUNSession) writeNextPath(packet []byte) error {
	s.pathsMu.Lock()
	if len(s.paths) == 0 {
		s.pathsMu.Unlock()
		return errors.New("no active QUIC paths")
	}
	path := s.paths[s.nextPath%len(s.paths)]
	s.nextPath = (s.nextPath + 1) % len(s.paths)
	s.pathsMu.Unlock()

	if err := WriteFrame(path.stream, packet); err != nil {
		s.removePath(path.stream)
		s.counters.AddError()
		return fmt.Errorf("path %d write packet: %w", path.id, err)
	}

	log.Printf("path %d sent TUN packet length=%d", path.id, len(packet))
	return nil
}

func isGracefulStreamEnd(err error) bool {
	if errors.Is(err, io.EOF) {
		return true
	}

	var appErr *quic.ApplicationError
	return errors.As(err, &appErr) && appErr.ErrorCode == quic.ApplicationErrorCode(0)
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
