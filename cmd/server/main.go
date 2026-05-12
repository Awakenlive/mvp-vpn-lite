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
	listen0 := flag.String("listen0", envconfig.String("MVPVPN_SERVER_LISTEN0", ":4433"), "QUIC listen address for path 0 (env MVPVPN_SERVER_LISTEN0)")
	listen1 := flag.String("listen1", envconfig.String("MVPVPN_SERVER_LISTEN1", ":4434"), "QUIC listen address for path 1 (env MVPVPN_SERVER_LISTEN1)")
	tlsCert := flag.String("tls-cert", envconfig.String("MVPVPN_SERVER_TLS_CERT", ""), "PEM TLS certificate file; empty generates an ephemeral demo certificate (env MVPVPN_SERVER_TLS_CERT)")
	tlsKey := flag.String("tls-key", envconfig.String("MVPVPN_SERVER_TLS_KEY", ""), "PEM TLS private key file; required with -tls-cert (env MVPVPN_SERVER_TLS_KEY)")
	virtualIP := flag.String("virtual-ip", envconfig.String("MVPVPN_SERVER_VIRTUAL_IP", "10.8.0.1"), "virtual server IPv4 address (env MVPVPN_SERVER_VIRTUAL_IP)")
	clientIP := flag.String("client-ip", envconfig.String("MVPVPN_SERVER_CLIENT_IP", "10.8.0.2"), "client tunnel IPv4 address (env MVPVPN_SERVER_CLIENT_IP)")
	statsInterval := flag.Duration("stats-interval", mustDurationEnv("MVPVPN_SERVER_STATS_INTERVAL", 10*time.Second), "stats log interval; 0 disables periodic stats (env MVPVPN_SERVER_STATS_INTERVAL)")
	tunMode := flag.Bool("tun", mustBoolEnv("MVPVPN_SERVER_TUN", false), "connect a local TUN device to the QUIC paths (env MVPVPN_SERVER_TUN)")
	tunName := flag.String("tun-name", envconfig.String("MVPVPN_SERVER_TUN_NAME", tun.DefaultServerDeviceName), "TUN device name for -tun mode (env MVPVPN_SERVER_TUN_NAME)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := quictransport.ServerConfig{
		Listen0:       *listen0,
		Listen1:       *listen1,
		TLSCertFile:   *tlsCert,
		TLSKeyFile:    *tlsKey,
		VirtualIP:     net.ParseIP(*virtualIP),
		ClientIP:      net.ParseIP(*clientIP),
		StatsInterval: *statsInterval,
		UseTUN:        *tunMode,
		DeviceName:    *tunName,
	}

	if cfg.UseTUN {
		log.Printf("starting QUIC TUN server: listen0=%s listen1=%s tun-name=%s", cfg.Listen0, cfg.Listen1, cfg.DeviceName)
	} else {
		log.Printf("starting QUIC demo server: listen0=%s listen1=%s virtual-ip=%s client-ip=%s", cfg.Listen0, cfg.Listen1, cfg.VirtualIP, cfg.ClientIP)
	}

	if err := quictransport.RunServer(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
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
