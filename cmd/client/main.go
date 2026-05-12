package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"os/signal"
	"syscall"
	"time"

	"mvp-vpn-lite/internal/quictransport"
	"mvp-vpn-lite/internal/tun"
)

func main() {
	server0 := flag.String("server0", "localhost:4433", "QUIC server address for path 0")
	server1 := flag.String("server1", "localhost:4434", "QUIC server address for path 1")
	caCert := flag.String("ca-cert", "", "PEM CA/server certificate used to verify the QUIC server; empty uses demo insecure TLS")
	serverName := flag.String("server-name", "", "TLS server name override for certificate verification")
	virtualIP := flag.String("virtual-ip", "10.8.0.1", "virtual server IPv4 address")
	clientIP := flag.String("client-ip", "10.8.0.2", "client tunnel IPv4 address")
	count := flag.Int("count", 4, "number of demo ICMP echo requests to send")
	identifier := flag.Uint("identifier", 0x4d56, "ICMP identifier")
	payload := flag.String("payload", "mvp-vpn-lite", "ICMP echo payload")
	timeout := flag.Duration("timeout", 5*time.Second, "per-request timeout")
	statsInterval := flag.Duration("stats-interval", 10*time.Second, "stats log interval; 0 disables periodic stats")
	tunMode := flag.Bool("tun", false, "connect a local TUN device to the QUIC paths")
	tunName := flag.String("tun-name", tun.DefaultDeviceName, "TUN device name for -tun mode")
	reconnectMin := flag.Duration("reconnect-min", quictransport.DefaultTUNReconnectMinInterval, "minimum retry delay for reconnecting failed TUN paths")
	reconnectMax := flag.Duration("reconnect-max", quictransport.DefaultTUNReconnectMaxInterval, "maximum retry delay for reconnecting failed TUN paths")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if *tunMode {
		cfg := quictransport.TUNClientConfig{
			Server0:              *server0,
			Server1:              *server1,
			CAFile:               *caCert,
			ServerName:           *serverName,
			DeviceName:           *tunName,
			StatsInterval:        *statsInterval,
			ReconnectMinInterval: *reconnectMin,
			ReconnectMaxInterval: *reconnectMax,
		}

		log.Printf("starting QUIC TUN client: server0=%s server1=%s tun-name=%s", cfg.Server0, cfg.Server1, cfg.DeviceName)

		if err := quictransport.RunTUNClient(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatal(err)
		}
		return
	}

	if *identifier > 0xffff {
		log.Fatalf("identifier must fit in uint16: %d", *identifier)
	}

	cfg := quictransport.ClientConfig{
		Server0:        *server0,
		Server1:        *server1,
		CAFile:         *caCert,
		ServerName:     *serverName,
		VirtualIP:      net.ParseIP(*virtualIP),
		ClientIP:       net.ParseIP(*clientIP),
		Identifier:     uint16(*identifier),
		Count:          *count,
		Payload:        []byte(*payload),
		RequestTimeout: *timeout,
		StatsInterval:  *statsInterval,
	}

	log.Printf("starting QUIC demo client: server0=%s server1=%s virtual-ip=%s client-ip=%s count=%d", cfg.Server0, cfg.Server1, cfg.VirtualIP, cfg.ClientIP, cfg.Count)

	if err := quictransport.RunClient(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}
