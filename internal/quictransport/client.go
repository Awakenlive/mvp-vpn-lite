package quictransport

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"mvp-vpn-lite/internal/multipath"
	"mvp-vpn-lite/internal/packet"
	"mvp-vpn-lite/internal/tun"

	"github.com/quic-go/quic-go"
)

const defaultClientRequestTimeout = 5 * time.Second

// ClientConfig contains the remote addresses and virtual IPs used by the demo
// client.
type ClientConfig struct {
	Server0        string
	Server1        string
	VirtualIP      net.IP
	ClientIP       net.IP
	Identifier     uint16
	Count          int
	Payload        []byte
	RequestTimeout time.Duration
}

// TUNClientConfig contains the remote addresses and local TUN device used by
// the packet pump.
type TUNClientConfig struct {
	Server0    string
	Server1    string
	DeviceName string
}

// RunClient connects to the configured QUIC paths and sends demo ICMP echo
// requests without creating a TUN device.
func RunClient(ctx context.Context, cfg ClientConfig) error {
	if cfg.Server0 == "" && cfg.Server1 == "" {
		return errors.New("at least one QUIC server address is required")
	}
	virtualIPv4 := cfg.VirtualIP.To4()
	if virtualIPv4 == nil {
		return fmt.Errorf("virtual IP must be IPv4: %s", cfg.VirtualIP)
	}
	clientIPv4 := cfg.ClientIP.To4()
	if clientIPv4 == nil {
		return fmt.Errorf("client IP must be IPv4: %s", cfg.ClientIP)
	}
	if cfg.Count <= 0 {
		return fmt.Errorf("count must be positive: %d", cfg.Count)
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = defaultClientRequestTimeout
	}

	paths, err := dialClientPaths(ctx, cfg.Server0, cfg.Server1)
	if err != nil {
		return err
	}
	defer closeClientPaths(paths)

	scheduler, err := multipath.NewRoundRobin(len(paths))
	if err != nil {
		return err
	}

	payload := append([]byte(nil), cfg.Payload...)
	for i := 0; i < cfg.Count; i++ {
		path := paths[scheduler.Next()]
		sequence := uint16(i + 1)

		request, err := packet.BuildICMPEchoRequest(clientIPv4, virtualIPv4, cfg.Identifier, sequence, payload)
		if err != nil {
			return err
		}

		if err := path.stream.SetDeadline(time.Now().Add(cfg.RequestTimeout)); err != nil {
			return fmt.Errorf("path %d set deadline: %w", path.id, err)
		}
		if err := WriteFrame(path.stream, request); err != nil {
			return fmt.Errorf("path %d write request: %w", path.id, err)
		}

		reply, err := ReadFrame(path.stream)
		if err != nil {
			return fmt.Errorf("path %d read reply: %w", path.id, err)
		}
		if err := validateEchoReply(reply, virtualIPv4, clientIPv4, cfg.Identifier, sequence, payload); err != nil {
			return fmt.Errorf("path %d validate reply: %w", path.id, err)
		}
		if err := path.stream.SetDeadline(time.Time{}); err != nil {
			return fmt.Errorf("path %d clear deadline: %w", path.id, err)
		}

		log.Printf("path %d echo reply sequence=%d bytes=%d", path.id, sequence, len(reply))
	}

	return nil
}

// RunTUNClient connects a Linux TUN interface to the configured QUIC paths.
func RunTUNClient(ctx context.Context, cfg TUNClientConfig) error {
	if cfg.Server0 == "" && cfg.Server1 == "" {
		return errors.New("at least one QUIC server address is required")
	}

	device, err := tun.Open(cfg.DeviceName)
	if err != nil {
		return err
	}
	defer device.Close()
	log.Printf("opened TUN device %s", device.Name())

	paths, err := dialClientPaths(ctx, cfg.Server0, cfg.Server1)
	if err != nil {
		return err
	}
	defer closeClientPaths(paths)

	scheduler, err := multipath.NewRoundRobin(len(paths))
	if err != nil {
		return err
	}

	return pumpTUN(ctx, device, paths, scheduler)
}

type clientPath struct {
	id      int
	address string
	conn    *quic.Conn
	stream  clientStream
}

type clientStream interface {
	io.ReadWriter
	Close() error
	SetDeadline(time.Time) error
}

func dialClientPaths(ctx context.Context, server0, server1 string) ([]clientPath, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // demo server uses an ephemeral self-signed certificate
		NextProtos:         []string{mvpQUICALPN},
		MinVersion:         tls.VersionTLS13,
	}

	var paths []clientPath
	if server0 != "" {
		paths = append(paths, clientPath{id: 0, address: server0})
	}
	if server1 != "" {
		paths = append(paths, clientPath{id: 1, address: server1})
	}

	for i := range paths {
		conn, err := quic.DialAddr(ctx, paths[i].address, tlsConfig.Clone(), nil)
		if err != nil {
			closeClientPaths(paths[:i])
			return nil, fmt.Errorf("path %d dial %s: %w", paths[i].id, paths[i].address, err)
		}

		stream, err := conn.OpenStreamSync(ctx)
		if err != nil {
			_ = conn.CloseWithError(0, "open stream failed")
			closeClientPaths(paths[:i])
			return nil, fmt.Errorf("path %d open stream: %w", paths[i].id, err)
		}

		paths[i].conn = conn
		paths[i].stream = stream
		log.Printf("path %d connected to %s", paths[i].id, paths[i].address)
	}

	return paths, nil
}

type packetDevice interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Close() error
}

type pathScheduler interface {
	Next() int
}

func pumpTUN(ctx context.Context, device packetDevice, paths []clientPath, scheduler pathScheduler) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, len(paths)+1)

	go func() {
		<-ctx.Done()
		_ = device.Close()
	}()

	var deviceWriteMu sync.Mutex
	for _, path := range paths {
		go receivePathPackets(ctx, path, device, &deviceWriteMu, errCh)
	}
	go sendTUNPackets(ctx, device, paths, scheduler, errCh)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		cancel()
		return err
	}
}

func sendTUNPackets(ctx context.Context, device packetDevice, paths []clientPath, scheduler pathScheduler, errCh chan<- error) {
	buffer := make([]byte, MaxFrameSize)
	for {
		n, err := device.Read(buffer)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, os.ErrClosed) {
				return
			}
			errCh <- fmt.Errorf("read TUN packet: %w", err)
			return
		}
		if n == 0 {
			continue
		}

		path := paths[scheduler.Next()]
		packet := append([]byte(nil), buffer[:n]...)
		if err := WriteFrame(path.stream, packet); err != nil {
			errCh <- fmt.Errorf("path %d write TUN packet: %w", path.id, err)
			return
		}

		log.Printf("path %d sent TUN packet length=%d", path.id, len(packet))
	}
}

func receivePathPackets(ctx context.Context, path clientPath, device packetDevice, deviceWriteMu *sync.Mutex, errCh chan<- error) {
	for {
		rawPacket, err := ReadFrame(path.stream)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, os.ErrClosed) {
				return
			}
			errCh <- fmt.Errorf("path %d read packet: %w", path.id, err)
			return
		}

		deviceWriteMu.Lock()
		_, err = device.Write(rawPacket)
		deviceWriteMu.Unlock()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, os.ErrClosed) {
				return
			}
			errCh <- fmt.Errorf("path %d write TUN packet: %w", path.id, err)
			return
		}

		log.Printf("path %d received TUN packet length=%d", path.id, len(rawPacket))
	}
}

func closeClientPaths(paths []clientPath) {
	for _, path := range paths {
		if path.stream != nil {
			_ = path.stream.Close()
		}
		if path.conn != nil {
			_ = path.conn.CloseWithError(0, "client stopped")
		}
	}
}

func validateEchoReply(reply []byte, virtualIP, clientIP net.IP, identifier, sequence uint16, payload []byte) error {
	if len(reply) < 20 {
		return errors.New("reply is too short")
	}
	if got := packet.Checksum(reply[:20]); got != 0 {
		return fmt.Errorf("invalid IPv4 checksum %#04x", got)
	}

	ipPacket, err := packet.ParseIPv4Packet(reply)
	if err != nil {
		return err
	}
	if !ipPacket.SourceIP.Equal(virtualIP) {
		return fmt.Errorf("reply source %s does not match virtual IP %s", ipPacket.SourceIP, virtualIP)
	}
	if !ipPacket.DestinationIP.Equal(clientIP) {
		return fmt.Errorf("reply destination %s does not match client IP %s", ipPacket.DestinationIP, clientIP)
	}
	if got := packet.Checksum(ipPacket.Payload); got != 0 {
		return fmt.Errorf("invalid ICMP checksum %#04x", got)
	}

	icmpPacket, err := packet.ParseICMPPacket(ipPacket.Payload)
	if err != nil {
		return err
	}
	if icmpPacket.Type != packet.ICMPTypeEchoReply {
		return fmt.Errorf("ICMP type %d, expected echo reply", icmpPacket.Type)
	}
	if icmpPacket.Code != 0 {
		return fmt.Errorf("ICMP code %d, expected 0", icmpPacket.Code)
	}
	if icmpPacket.Identifier != identifier {
		return fmt.Errorf("ICMP identifier %#04x, expected %#04x", icmpPacket.Identifier, identifier)
	}
	if icmpPacket.Sequence != sequence {
		return fmt.Errorf("ICMP sequence %d, expected %d", icmpPacket.Sequence, sequence)
	}
	if !bytes.Equal(icmpPacket.Payload, payload) {
		return fmt.Errorf("ICMP payload %q, expected %q", icmpPacket.Payload, payload)
	}

	return nil
}
