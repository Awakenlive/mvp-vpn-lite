package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"os/signal"
	"syscall"

	"mvp-vpn-lite/internal/quictransport"
)

func main() {
	listen0 := flag.String("listen0", ":4433", "QUIC listen address for path 0")
	listen1 := flag.String("listen1", ":4434", "QUIC listen address for path 1")
	virtualIP := flag.String("virtual-ip", "10.8.0.1", "virtual server IPv4 address")
	clientIP := flag.String("client-ip", "10.8.0.2", "client tunnel IPv4 address")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := quictransport.ServerConfig{
		Listen0:   *listen0,
		Listen1:   *listen1,
		VirtualIP: net.ParseIP(*virtualIP),
		ClientIP:  net.ParseIP(*clientIP),
	}

	log.Printf("starting QUIC demo server: listen0=%s listen1=%s virtual-ip=%s client-ip=%s", cfg.Listen0, cfg.Listen1, cfg.VirtualIP, cfg.ClientIP)

	if err := quictransport.RunServer(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}
