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

	"mvp-vpn-lite/internal/envconfig"
	"mvp-vpn-lite/internal/quictransport"
	"mvp-vpn-lite/internal/tun"
)

func main() {
	server0 := flag.String("server0", envconfig.String("MVPVPN_CLIENT_SERVER0", "localhost:4433"), "QUIC server address for path 0 (env MVPVPN_CLIENT_SERVER0)")
	server1 := flag.String("server1", envconfig.String("MVPVPN_CLIENT_SERVER1", "localhost:4434"), "QUIC server address for path 1 (env MVPVPN_CLIENT_SERVER1)")
	caCert := flag.String("ca-cert", envconfig.String("MVPVPN_CLIENT_CA_CERT", ""), "PEM CA/server certificate used to verify the QUIC server; empty uses demo insecure TLS (env MVPVPN_CLIENT_CA_CERT)")
	serverName := flag.String("server-name", envconfig.String("MVPVPN_CLIENT_SERVER_NAME", ""), "TLS server name override for certificate verification (env MVPVPN_CLIENT_SERVER_NAME)")
	virtualIP := flag.String("virtual-ip", envconfig.String("MVPVPN_CLIENT_VIRTUAL_IP", "10.8.0.1"), "virtual server IPv4 address (env MVPVPN_CLIENT_VIRTUAL_IP)")
	clientIP := flag.String("client-ip", envconfig.String("MVPVPN_CLIENT_CLIENT_IP", "10.8.0.2"), "client tunnel IPv4 address (env MVPVPN_CLIENT_CLIENT_IP)")
	count := flag.Int("count", mustIntEnv("MVPVPN_CLIENT_COUNT", 4), "number of demo ICMP echo requests to send (env MVPVPN_CLIENT_COUNT)")
	identifier := flag.Uint("identifier", mustUintEnv("MVPVPN_CLIENT_IDENTIFIER", 0x4d56), "ICMP identifier (env MVPVPN_CLIENT_IDENTIFIER)")
	payload := flag.String("payload", envconfig.String("MVPVPN_CLIENT_PAYLOAD", "mvp-vpn-lite"), "ICMP echo payload (env MVPVPN_CLIENT_PAYLOAD)")
	timeout := flag.Duration("timeout", mustDurationEnv("MVPVPN_CLIENT_TIMEOUT", 5*time.Second), "per-request timeout (env MVPVPN_CLIENT_TIMEOUT)")
	statsInterval := flag.Duration("stats-interval", mustDurationEnv("MVPVPN_CLIENT_STATS_INTERVAL", 10*time.Second), "stats log interval; 0 disables periodic stats (env MVPVPN_CLIENT_STATS_INTERVAL)")
	tunMode := flag.Bool("tun", mustBoolEnv("MVPVPN_CLIENT_TUN", false), "connect a local TUN device to the QUIC paths (env MVPVPN_CLIENT_TUN)")
	tunName := flag.String("tun-name", envconfig.String("MVPVPN_CLIENT_TUN_NAME", tun.DefaultDeviceName), "TUN device name for -tun mode (env MVPVPN_CLIENT_TUN_NAME)")
	reconnectMin := flag.Duration("reconnect-min", mustDurationEnv("MVPVPN_CLIENT_RECONNECT_MIN", quictransport.DefaultTUNReconnectMinInterval), "minimum retry delay for reconnecting failed TUN paths (env MVPVPN_CLIENT_RECONNECT_MIN)")
	reconnectMax := flag.Duration("reconnect-max", mustDurationEnv("MVPVPN_CLIENT_RECONNECT_MAX", quictransport.DefaultTUNReconnectMaxInterval), "maximum retry delay for reconnecting failed TUN paths (env MVPVPN_CLIENT_RECONNECT_MAX)")
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

func mustBoolEnv(name string, fallback bool) bool {
	value, err := envconfig.Bool(name, fallback)
	if err != nil {
		log.Fatal(err)
	}
	return value
}

func mustDurationEnv(name string, fallback time.Duration) time.Duration {
	value, err := envconfig.Duration(name, fallback)
	if err != nil {
		log.Fatal(err)
	}
	return value
}

func mustIntEnv(name string, fallback int) int {
	value, err := envconfig.Int(name, fallback)
	if err != nil {
		log.Fatal(err)
	}
	return value
}

func mustUintEnv(name string, fallback uint) uint {
	value, err := envconfig.Uint(name, fallback)
	if err != nil {
		log.Fatal(err)
	}
	return value
}
