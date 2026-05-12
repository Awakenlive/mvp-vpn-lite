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
	"mvp-vpn-lite/internal/stats"
	"mvp-vpn-lite/internal/tun"

	"github.com/quic-go/quic-go"
)

const (
	defaultClientRequestTimeout = 5 * time.Second
	defaultClientKeepAlive      = 10 * time.Second
	defaultClientMaxIdleTimeout = 30 * time.Second

	DefaultTUNReconnectMinInterval = 1 * time.Second
	DefaultTUNReconnectMaxInterval = 30 * time.Second
)

// ClientConfig contains the remote addresses and virtual IPs used by the demo
// client.
type ClientConfig struct {
	Server0        string
	Server1        string
	CAFile         string
	ServerName     string
	VirtualIP      net.IP
	ClientIP       net.IP
	Identifier     uint16
	Count          int
	Payload        []byte
	RequestTimeout time.Duration
	StatsInterval  time.Duration
}

// TUNClientConfig contains the remote addresses and local TUN device used by
// the packet pump.
type TUNClientConfig struct {
	Server0              string
	Server1              string
	CAFile               string
	ServerName           string
	DeviceName           string
	StatsInterval        time.Duration
	ReconnectMinInterval time.Duration
	ReconnectMaxInterval time.Duration
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

	tlsConfig, err := clientTLSConfig(cfg.CAFile, cfg.ServerName)
	if err != nil {
		return err
	}

	paths, err := dialClientPaths(ctx, cfg.Server0, cfg.Server1, tlsConfig)
	if err != nil {
		return err
	}
	defer closeClientPaths(paths)

	scheduler, err := multipath.NewRoundRobin(len(paths))
	if err != nil {
		return err
	}

	var counters stats.Counters
	ctx, cancelStats := context.WithCancel(ctx)
	defer cancelStats()
	go stats.LogEvery(ctx, cfg.StatsInterval, &counters, log.Printf)
	defer func() {
		log.Printf("stats final: %s", counters.Snapshot())
	}()

	payload := append([]byte(nil), cfg.Payload...)
	for i := 0; i < cfg.Count; i++ {
		path := paths[scheduler.Next()]
		sequence := uint16(i + 1)

		request, err := packet.BuildICMPEchoRequest(clientIPv4, virtualIPv4, cfg.Identifier, sequence, payload)
		if err != nil {
			return err
		}

		if err := path.stream.SetDeadline(time.Now().Add(cfg.RequestTimeout)); err != nil {
			counters.AddError()
			return fmt.Errorf("path %d set deadline: %w", path.id, err)
		}
		if err := WriteFrame(path.stream, request); err != nil {
			counters.AddError()
			return fmt.Errorf("path %d write request: %w", path.id, err)
		}
		counters.AddTX(len(request))

		reply, err := ReadFrame(path.stream)
		if err != nil {
			counters.AddError()
			return fmt.Errorf("path %d read reply: %w", path.id, err)
		}
		counters.AddRX(len(reply))
		if err := validateEchoReply(reply, virtualIPv4, clientIPv4, cfg.Identifier, sequence, payload); err != nil {
			counters.AddError()
			return fmt.Errorf("path %d validate reply: %w", path.id, err)
		}
		if err := path.stream.SetDeadline(time.Time{}); err != nil {
			counters.AddError()
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
	reconnectMin, reconnectMax, err := normalizeTUNReconnectIntervals(cfg.ReconnectMinInterval, cfg.ReconnectMaxInterval)
	if err != nil {
		return err
	}

	device, err := tun.Open(cfg.DeviceName)
	if err != nil {
		return err
	}
	defer device.Close()
	log.Printf("opened TUN device %s", device.Name())

	tlsConfig, err := clientTLSConfig(cfg.CAFile, cfg.ServerName)
	if err != nil {
		return err
	}

	var counters stats.Counters
	ctx, cancelStats := context.WithCancel(ctx)
	defer cancelStats()
	go stats.LogEvery(ctx, cfg.StatsInterval, &counters, log.Printf)
	defer func() {
		log.Printf("stats final: %s", counters.Snapshot())
	}()

	pathSpecs := clientPathSpecs(cfg.Server0, cfg.Server1)
	paths := dialAvailableClientPaths(ctx, pathSpecs, tlsConfig, &counters)
	if len(paths) == 0 {
		log.Printf("no initial QUIC TUN paths connected; reconnect loop will keep trying")
	}

	reconnect := &clientReconnectConfig{
		pathSpecs: pathSpecs,
		tlsConfig: tlsConfig,
		minDelay:  reconnectMin,
		maxDelay:  reconnectMax,
	}
	return pumpTUN(ctx, device, paths, &counters, reconnect)
}

type clientPathSpec struct {
	id      int
	address string
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

func clientPathSpecs(server0, server1 string) []clientPathSpec {
	var specs []clientPathSpec
	if server0 != "" {
		specs = append(specs, clientPathSpec{id: 0, address: server0})
	}
	if server1 != "" {
		specs = append(specs, clientPathSpec{id: 1, address: server1})
	}
	return specs
}

func dialClientPaths(ctx context.Context, server0, server1 string, tlsConfig *tls.Config) ([]clientPath, error) {
	specs := clientPathSpecs(server0, server1)
	paths := make([]clientPath, 0, len(specs))

	for _, spec := range specs {
		path, err := dialClientPath(ctx, spec, tlsConfig)
		if err != nil {
			closeClientPaths(paths)
			return nil, err
		}
		paths = append(paths, path)
	}

	return paths, nil
}

func dialAvailableClientPaths(ctx context.Context, specs []clientPathSpec, tlsConfig *tls.Config, counters *stats.Counters) []clientPath {
	paths := make([]clientPath, 0, len(specs))
	for _, spec := range specs {
		path, err := dialClientPath(ctx, spec, tlsConfig)
		if err != nil {
			if ctx.Err() != nil {
				return paths
			}
			counters.AddError()
			log.Printf("path %d initial dial to %s failed: %v", spec.id, spec.address, err)
			continue
		}
		paths = append(paths, path)
	}
	return paths
}

func dialClientPath(ctx context.Context, spec clientPathSpec, tlsConfig *tls.Config) (clientPath, error) {
	conn, err := quic.DialAddr(ctx, spec.address, tlsConfig.Clone(), clientQUICConfig())
	if err != nil {
		return clientPath{}, fmt.Errorf("path %d dial %s: %w", spec.id, spec.address, err)
	}

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		_ = conn.CloseWithError(0, "open stream failed")
		return clientPath{}, fmt.Errorf("path %d open stream: %w", spec.id, err)
	}

	log.Printf("path %d connected to %s", spec.id, spec.address)
	return clientPath{
		id:      spec.id,
		address: spec.address,
		conn:    conn,
		stream:  stream,
	}, nil
}

func clientQUICConfig() *quic.Config {
	return &quic.Config{
		KeepAlivePeriod: defaultClientKeepAlive,
		MaxIdleTimeout:  defaultClientMaxIdleTimeout,
	}
}

type packetDevice interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Close() error
}

type clientPathSet struct {
	mu    sync.Mutex
	paths []clientPath
	next  int
}

func newClientPathSet(paths []clientPath) *clientPathSet {
	return &clientPathSet{
		paths: append([]clientPath(nil), paths...),
	}
}

func (s *clientPathSet) add(path clientPath) (clientPath, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, current := range s.paths {
		if current.id == path.id {
			s.paths[i] = path
			return current, true
		}
	}

	s.paths = append(s.paths, path)
	return clientPath{}, false
}

func (s *clientPathSet) nextPath() (clientPath, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.paths) == 0 {
		return clientPath{}, errors.New("no active QUIC paths")
	}

	path := s.paths[s.next%len(s.paths)]
	s.next = (s.next + 1) % len(s.paths)
	return path, nil
}

func (s *clientPathSet) remove(stream clientStream) (clientPath, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, path := range s.paths {
		if path.stream == stream {
			s.paths = append(s.paths[:i], s.paths[i+1:]...)
			if len(s.paths) == 0 {
				s.next = 0
			} else if s.next >= len(s.paths) {
				s.next %= len(s.paths)
			}
			return path, true
		}
	}

	return clientPath{}, false
}

func (s *clientPathSet) hasPathID(pathID int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, path := range s.paths {
		if path.id == pathID {
			return true
		}
	}
	return false
}

func (s *clientPathSet) len() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.paths)
}

func (s *clientPathSet) snapshot() []clientPath {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]clientPath(nil), s.paths...)
}

type clientReconnectConfig struct {
	pathSpecs []clientPathSpec
	tlsConfig *tls.Config
	minDelay  time.Duration
	maxDelay  time.Duration
}

func pumpTUN(ctx context.Context, device packetDevice, paths []clientPath, counters *stats.Counters, reconnect *clientReconnectConfig) error {
	ctx, cancel := context.WithCancel(ctx)

	errCh := make(chan error, maxTUNClientErrors(paths, reconnect)+1)
	pathSet := newClientPathSet(paths)
	defer func() {
		cancel()
		closeClientPaths(pathSet.snapshot())
	}()

	go func() {
		<-ctx.Done()
		_ = device.Close()
	}()

	var deviceWriteMu sync.Mutex
	for _, path := range paths {
		go receivePathPackets(ctx, path, device, &deviceWriteMu, pathSet, counters, errCh)
	}
	if reconnect != nil {
		for _, spec := range reconnect.pathSpecs {
			go reconnectClientPath(ctx, spec, reconnect.tlsConfig, reconnect.minDelay, reconnect.maxDelay, device, &deviceWriteMu, pathSet, counters, errCh)
		}
	}
	go sendTUNPackets(ctx, device, pathSet, counters, errCh)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		cancel()
		return err
	}
}

func maxTUNClientErrors(paths []clientPath, reconnect *clientReconnectConfig) int {
	if reconnect != nil && len(reconnect.pathSpecs) > len(paths) {
		return len(reconnect.pathSpecs)
	}
	return len(paths)
}

func sendTUNPackets(ctx context.Context, device packetDevice, pathSet *clientPathSet, counters *stats.Counters, errCh chan<- error) {
	buffer := make([]byte, MaxFrameSize)
	for {
		n, err := device.Read(buffer)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, os.ErrClosed) {
				return
			}
			counters.AddError()
			errCh <- fmt.Errorf("read TUN packet: %w", err)
			return
		}
		if n == 0 {
			continue
		}

		packet := append([]byte(nil), buffer[:n]...)
		for {
			path, err := pathSet.nextPath()
			if err != nil {
				counters.AddDrop()
				log.Printf("dropped TUN packet length=%d: %v", len(packet), err)
				break
			}

			if err := WriteFrame(path.stream, packet); err != nil {
				counters.AddError()
				if removed, ok := pathSet.remove(path.stream); ok {
					closeClientPath(removed)
					log.Printf("path %d removed after write error: %v; active paths=%d", removed.id, err, pathSet.len())
				}
				continue
			}
			counters.AddTX(len(packet))
			log.Printf("path %d sent TUN packet length=%d", path.id, len(packet))
			break
		}
	}
}

func receivePathPackets(ctx context.Context, path clientPath, device packetDevice, deviceWriteMu *sync.Mutex, pathSet *clientPathSet, counters *stats.Counters, errCh chan<- error) {
	defer func() {
		if removed, ok := pathSet.remove(path.stream); ok {
			closeClientPath(removed)
			log.Printf("path %d receive loop stopped; active paths=%d", removed.id, pathSet.len())
		}
	}()

	for {
		rawPacket, err := ReadFrame(path.stream)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, os.ErrClosed) || isGracefulStreamEnd(err) {
				return
			}
			counters.AddError()
			log.Printf("path %d read packet: %v", path.id, err)
			return
		}

		deviceWriteMu.Lock()
		_, err = device.Write(rawPacket)
		deviceWriteMu.Unlock()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, os.ErrClosed) {
				return
			}
			counters.AddError()
			errCh <- fmt.Errorf("path %d write TUN packet: %w", path.id, err)
			return
		}
		counters.AddRX(len(rawPacket))

		log.Printf("path %d received TUN packet length=%d", path.id, len(rawPacket))
	}
}

func reconnectClientPath(
	ctx context.Context,
	spec clientPathSpec,
	tlsConfig *tls.Config,
	minDelay time.Duration,
	maxDelay time.Duration,
	device packetDevice,
	deviceWriteMu *sync.Mutex,
	pathSet *clientPathSet,
	counters *stats.Counters,
	errCh chan<- error,
) {
	backoff := newReconnectBackoff(minDelay, maxDelay)

	for {
		if ctx.Err() != nil {
			return
		}
		if pathSet.hasPathID(spec.id) {
			if !sleepContext(ctx, minDelay) {
				return
			}
			continue
		}

		path, err := dialClientPath(ctx, spec, tlsConfig)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			counters.AddError()
			delay := backoff.next()
			log.Printf("path %d reconnect to %s failed: %v; retrying in %s", spec.id, spec.address, err, delay)
			if !sleepContext(ctx, delay) {
				return
			}
			continue
		}

		backoff.reset()
		if old, replaced := pathSet.add(path); replaced {
			closeClientPath(old)
		}
		log.Printf("path %d reconnected to %s; active paths=%d", path.id, path.address, pathSet.len())
		go receivePathPackets(ctx, path, device, deviceWriteMu, pathSet, counters, errCh)
	}
}

type reconnectBackoff struct {
	min     time.Duration
	max     time.Duration
	current time.Duration
}

func newReconnectBackoff(minDelay, maxDelay time.Duration) *reconnectBackoff {
	return &reconnectBackoff{
		min:     minDelay,
		max:     maxDelay,
		current: minDelay,
	}
}

func (b *reconnectBackoff) next() time.Duration {
	delay := b.current
	if b.current < b.max {
		b.current *= 2
		if b.current > b.max {
			b.current = b.max
		}
	}
	return delay
}

func (b *reconnectBackoff) reset() {
	b.current = b.min
}

func sleepContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func normalizeTUNReconnectIntervals(minDelay, maxDelay time.Duration) (time.Duration, time.Duration, error) {
	if minDelay == 0 {
		minDelay = DefaultTUNReconnectMinInterval
	}
	if maxDelay == 0 {
		maxDelay = DefaultTUNReconnectMaxInterval
	}
	if minDelay < 0 {
		return 0, 0, fmt.Errorf("reconnect min interval must not be negative: %s", minDelay)
	}
	if maxDelay < 0 {
		return 0, 0, fmt.Errorf("reconnect max interval must not be negative: %s", maxDelay)
	}
	if minDelay > maxDelay {
		return 0, 0, fmt.Errorf("reconnect min interval %s exceeds max interval %s", minDelay, maxDelay)
	}
	return minDelay, maxDelay, nil
}

func closeClientPaths(paths []clientPath) {
	for _, path := range paths {
		closeClientPath(path)
	}
}

func closeClientPath(path clientPath) {
	if path.stream != nil {
		_ = path.stream.Close()
	}
	if path.conn != nil {
		_ = path.conn.CloseWithError(0, "client stopped")
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
