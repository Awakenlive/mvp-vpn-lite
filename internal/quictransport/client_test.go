package quictransport

import (
	"net"
	"testing"

	"mvp-vpn-lite/internal/packet"
)

func TestValidateEchoReply(t *testing.T) {
	t.Parallel()

	virtualIP := net.IPv4(10, 8, 0, 1)
	clientIP := net.IPv4(10, 8, 0, 2)
	payload := []byte("demo")

	request, err := packet.BuildICMPEchoRequest(clientIP, virtualIP, 0x4d56, 9, payload)
	if err != nil {
		t.Fatalf("BuildICMPEchoRequest() error = %v", err)
	}
	reply, err := packet.BuildICMPEchoReply(request, virtualIP, clientIP)
	if err != nil {
		t.Fatalf("BuildICMPEchoReply() error = %v", err)
	}

	if err := validateEchoReply(reply, virtualIP, clientIP, 0x4d56, 9, payload); err != nil {
		t.Fatalf("validateEchoReply() error = %v", err)
	}
}

func TestValidateEchoReplyRejectsUnexpectedSequence(t *testing.T) {
	t.Parallel()

	virtualIP := net.IPv4(10, 8, 0, 1)
	clientIP := net.IPv4(10, 8, 0, 2)
	payload := []byte("demo")

	request, err := packet.BuildICMPEchoRequest(clientIP, virtualIP, 0x4d56, 9, payload)
	if err != nil {
		t.Fatalf("BuildICMPEchoRequest() error = %v", err)
	}
	reply, err := packet.BuildICMPEchoReply(request, virtualIP, clientIP)
	if err != nil {
		t.Fatalf("BuildICMPEchoReply() error = %v", err)
	}

	if err := validateEchoReply(reply, virtualIP, clientIP, 0x4d56, 10, payload); err == nil {
		t.Fatal("validateEchoReply() error = nil, want error")
	}
}
