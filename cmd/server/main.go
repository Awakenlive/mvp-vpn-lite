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
	listen0 := flag.String("listen0", ":4433", "QUIC listen address for path 0")
	listen1 := flag.String("listen1", ":4434", "QUIC listen address for path 1")
	tlsCert := flag.String("tls-cert", "", "PEM TLS certificate file; empty generates an ephemeral demo certificate")
	tlsKey := flag.String("tls-key", "", "PEM TLS private key file; required with -tls-cert")
	virtualIP := flag.String("virtual-ip", "10.8.0.1", "virtual server IPv4 address")
	clientIP := flag.String("client-ip", "10.8.0.2", "client tunnel IPv4 address")
	statsInterval := flag.Duration("stats-interval", 10*time.Second, "stats log interval; 0 disables periodic stats")
	tunMode := flag.Bool("tun", false, "connect a local TUN device to the QUIC paths")
	tunName := flag.String("tun-name", tun.DefaultServerDeviceName, "TUN device name for -tun mode")
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
