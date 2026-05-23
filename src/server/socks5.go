package main

import (
	"io"
	"net"
	"strconv"
	"time"
)

// socks5Server is a minimal SOCKS5 proxy (RFC 1928) supporting CONNECT.
type socks5Server struct {
	addr    string
	running bool
}

// startSOCKS5 starts the built-in SOCKS5 proxy on the configured address.
// Default: server_ip:1744
func startSOCKS5(addr string) {
	s := &socks5Server{addr: addr, running: true}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		structuredLog.Error("socks5_start", "Failed to start SOCKS5 proxy", map[string]interface{}{
			"addr":  addr,
			"error": err.Error(),
		})
		return
	}

	structuredLog.Info("socks5_start", "SOCKS5 proxy started", map[string]interface{}{
		"addr": addr,
	})

	for s.running {
		conn, err := listener.Accept()
		if err != nil {
			if s.running {
				structuredLog.Warn("socks5_accept", "SOCKS5 accept error", map[string]interface{}{
					"error": err.Error(),
				})
			}
			continue
		}
		go s.handle(conn)
	}
}

func (s *socks5Server) handle(client net.Conn) {
	defer client.Close()

	// Set deadline for the handshake
	client.SetDeadline(time.Now().Add(30 * time.Second))

	// Step 1: Auth negotiation (RFC 1928 §3)
	// Read version + nmethods
	buf := make([]byte, 263)
	n, err := io.ReadFull(client, buf[:2])
	if err != nil || n < 2 || buf[0] != 0x05 {
		return
	}
	nmethods := int(buf[1])
	if nmethods > 0 {
		io.ReadFull(client, buf[:nmethods])
	}

	// Reply: no authentication required (0x00)
	client.Write([]byte{0x05, 0x00})

	// Step 2: Request (RFC 1928 §4)
	n, err = io.ReadFull(client, buf[:4])
	if err != nil || n < 4 || buf[0] != 0x05 {
		return
	}
	cmd := buf[1]
	if cmd != 0x01 { // Only CONNECT is supported
		client.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	// Parse destination address
	var dstAddr string
	atyp := buf[3]
	switch atyp {
	case 0x01: // IPv4
		n, err = io.ReadFull(client, buf[:4])
		if err != nil || n < 4 {
			return
		}
		dstAddr = net.IP(buf[:4]).String()
	case 0x03: // Domain name
		n, err = io.ReadFull(client, buf[:1])
		if err != nil || n < 1 {
			return
		}
		nameLen := int(buf[0])
		n, err = io.ReadFull(client, buf[:nameLen])
		if err != nil || n < nameLen {
			return
		}
		dstAddr = string(buf[:nameLen])
	case 0x04: // IPv6 — not supported
		client.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	default:
		client.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	// Read port (2 bytes, big-endian)
	n, err = io.ReadFull(client, buf[:2])
	if err != nil || n < 2 {
		return
	}
	port := int(buf[0])<<8 | int(buf[1])
	dstAddr = net.JoinHostPort(dstAddr, strconv.Itoa(port))

	// Connect to destination
	dst, err := net.DialTimeout("tcp", dstAddr, 15*time.Second)
	if err != nil {
		client.Write([]byte{0x05, 0x04, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer dst.Close()

	// Reply success with bound address
	client.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})

	// Clear deadline — relay
	client.SetDeadline(time.Time{})
	dst.SetDeadline(time.Time{})

	// Bidirectional relay
	done := make(chan struct{}, 2)
	go func() {
		io.Copy(dst, client)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(client, dst)
		done <- struct{}{}
	}()

	// Wait for either direction to finish, then close
	<-done
	if tcpConn, ok := client.(*net.TCPConn); ok {
		tcpConn.SetLinger(0)
	}
}
