//go:build windows
// +build windows

package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.zx2c4.com/wintun"
	"github.com/gorilla/websocket"
	"github.com/quic-go/quic-go"
	utls "github.com/refraction-networking/utls"
	"wsvpn/obfuscation"
)

// ---------------------------------------------------
// Client / Config
// ---------------------------------------------------

type Config struct {
	Name              string `json:"name"`
	ClientIP          string `json:"client_ip"`
	ServerURL         string `json:"server_url"`
	UUID              string `json:"uuid"`
	Reconnect         bool   `json:"reconnect"`
	LogLevel          string `json:"log_level"`
	Obfuscation    string `json:"obfuscation"`    // "on" or "off" (default: "on")
	Transport      string `json:"transport"`
	DNS            string `json:"dns"`
	QUICSNI        string `json:"quic_sni"`        // Optional: override QUIC SNI
	TLSFingerprint string `json:"tls_fingerprint"` // Browser TLS fingerprint: chrome, firefox, ios, edge, random
	TrafficShape   string `json:"traffic_shape"`   // Traffic shaping mode: off, jitter, browse, adaptive
	FrontDomain        string   `json:"front_domain"`        // CDN front domain for domain fronting: SNI=front, Host=real
	DispersionPeers    []string `json:"dispersion_peers"`    // Additional server URLs for traffic dispersion
	TrafficInduction   bool     `json:"traffic_induction"`   // Generate fake browsing noise during idle
	InductionDomains   []string `json:"induction_domains"`   // Domains to use for traffic induction
}

type Client struct {
	cfg        *Config
	tun        *wintun.Adapter
	tunSession wintun.Session
	conn       *websocket.Conn
	quicConn   *quic.Conn
	quicStream *quic.Stream
	running    bool
	runningMu  sync.RWMutex
	stopCh     chan struct{}
	errCh      chan error
	serverIP   string
	wsWriteMu  sync.Mutex               // Protect WebSocket writes (CRITICAL FIX #1)
	shape       *obfuscation.ShaperState // Traffic shaper
	peerConns   []*websocket.Conn        // Extra connections for traffic dispersion
	sendIdx     uint32                    // Atomic counter for round-robin sending
	inductionCh chan struct{}             // Close to stop traffic induction
}

// ---------------------------------------------------
// Wintun DLL Loading
// ---------------------------------------------------

var (
	wintunDLL  *syscall.DLL
	wintunOnce sync.Once
	wintunErr  error
)

func getArchitecture() string {
	switch runtime.GOARCH {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	default:
		return "amd64"
	}
}

func findWintunDLL() (string, error) {
	arch := getArchitecture()

	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("get executable path: %w", err)
	}

	exeDir := filepath.Dir(exePath)

	dllPath := filepath.Join(exeDir, "drivers", "wintun", arch, "wintun.dll")
	if _, err := os.Stat(dllPath); err == nil {
		return dllPath, nil
	}

	dllPath = filepath.Join(exeDir, "wintun.dll")
	if _, err := os.Stat(dllPath); err == nil {
		return dllPath, nil
	}

	return "", fmt.Errorf("wintun.dll not found (arch: %s)", arch)
}

func loadWintunDriver() error {
	wintunOnce.Do(func() {
		dllPath, err := findWintunDLL()
		if err != nil {
			wintunErr = err
			return
		}

		wintunDLL, wintunErr = syscall.LoadDLL(dllPath)
		if wintunErr != nil {
			wintunErr = fmt.Errorf("failed to load wintun.dll: %w", wintunErr)
			return
		}

		log.Printf("Wintun driver loaded")
	})

	return wintunErr
}

// ---------------------------------------------------
// Main & CLI parsing
// ---------------------------------------------------

const Version = "v1.2"

func showHelp() {
	fmt.Println("WSVPN Windows Client " + Version)
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  wsvpn-client.exe              Start VPN (default)")
	fmt.Println("  wsvpn-client.exe -config F    Start with config file")
	fmt.Println("  wsvpn-client.exe disconnect   Stop VPN")
	fmt.Println("  wsvpn-client.exe -version     Show version")
	fmt.Println("  wsvpn-client.exe -help        Show this help")
}

func main() {
	cmd := "connect"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	// Global flags
	if cmd == "-h" || cmd == "--help" || cmd == "-help" || cmd == "help" {
		showHelp()
		os.Exit(0)
	}
	if cmd == "-version" || cmd == "--version" || cmd == "version" {
		versionCmd()
		return
	}

	switch cmd {
	case "connect", "-config":
		connectCmd()
	case "disconnect":
		disconnectCmd()
	default:
		// Treat unknown arg as config file path
		os.Setenv("WSVPN_CONFIG", os.Args[1])
		connectCmd()
	}
}

var client *Client

func findConfig() string {
	// Try common paths
	paths := []string{"client.json", "client-windows.json"}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	// Try next to exe
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for _, p := range paths {
			fp := filepath.Join(dir, p)
			if _, err := os.Stat(fp); err == nil {
				return fp
			}
		}
	}
	return "client.json"
}

func connectCmd() {
	cfgPath := findConfig()

	// Parse -config flag if provided
	for i, a := range os.Args {
		if (a == "-config" || a == "--config") && i+1 < len(os.Args) {
			cfgPath = os.Args[i+1]
		}
	}
	}

	// Load Wintun driver first
	if err := loadWintunDriver(); err != nil {
		log.Fatalf("Failed to load wintun driver: %v", err)
	}
	defer wintunDLL.Release()

	// Load configuration
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		log.Fatalf("Config error: %v", err)
	}

	// Validate and normalize obfuscation setting (default: "on")
	if cfg.Obfuscation == "" {
		cfg.Obfuscation = "on"
	}
	obfuscationEnabled := (cfg.Obfuscation == "on" || cfg.Obfuscation == "true" || cfg.Obfuscation == "1")
	log.Printf("Obfuscation: %s", cfg.Obfuscation)

	// Initialize obfuscation based on config
	if obfuscationEnabled {
		obfuscation.InitObfuscation()
		log.Println("Obfuscation enabled")
	} else {
		log.Println("Obfuscation disabled")
	}

	// Create client
	client = &Client{cfg: cfg}

	// Main connection loop (supports reconnect)
	for {
		if err := client.runConnection(); err != nil {
			if !cfg.Reconnect {
				log.Printf("Reconnect disabled, exiting: %v", err)
				break
			}
			log.Printf("Connection lost: %v", err)
			log.Println("Reconnecting in 5 seconds...")
			time.Sleep(5 * time.Second)
			// Clean up and retry
			client.stop()
			client = &Client{cfg: cfg}
		} else {
			// Clean exit
			break
		}
	}

	client.stop()
	fmt.Println("VPN disconnected")
}

func (c *Client) runConnection() error {
	// Resolve server IP (for route exclusion)
	serverIP, err := resolveServerIP(c.cfg.ServerURL)
	if err != nil {
		return fmt.Errorf("resolve server IP: %w", err)
	}
	c.serverIP = serverIP
	log.Printf("Server resolved")

	// Connect to get assigned IP
	assignedIP, err := c.connect()
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	log.Printf("Server assigned IP: %s", assignedIP)

	// Initialize TUN with server-assigned IP
	if err := c.initTUN(assignedIP); err != nil {
		return fmt.Errorf("init TUN: %w", err)
	}

	// Start forwarding
	if err := c.start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	log.Println("VPN connection established successfully")

	// Connect to dispersion peers for traffic distribution
	for _, peerURL := range c.cfg.DispersionPeers {
		savedURL := c.cfg.ServerURL
		c.cfg.ServerURL = peerURL
		peerIP, err := c.connect()
		if err != nil {
			log.Printf("Dispersion peer failed: %s: %v", peerURL, err)
			c.cfg.ServerURL = savedURL
			continue
		}
		c.peerConns = append(c.peerConns, c.conn)
		log.Printf("Dispersion peer connected: %s (IP: %s)", peerURL, peerIP)
		c.cfg.ServerURL = savedURL
	}

	// Wait for errors or stop signal
	return c.waitForStop()
}

func disconnectCmd() {
	if client == nil || !client.isRunning() {
		fmt.Println("VPN not running")
		return
	}
	client.stop()
	fmt.Println("VPN stopped")
}

func statusCmd() {
	if client != nil && client.isRunning() {
		fmt.Println("VPN Status: Connected")
		fmt.Printf("  Server: %s\n", client.cfg.ServerURL)
		fmt.Printf("  IP: %s\n", client.cfg.ClientIP)
		fmt.Printf("  Transport: %s\n", client.cfg.Transport)
	} else {
		fmt.Println("VPN Status: Disconnected")
	}
}

func versionCmd() {
	fmt.Printf("WSVPN Client %s (Windows)\n", Version)
	fmt.Printf("  Architecture: %s\n", runtime.GOARCH)
	fmt.Printf("  Go Version: %s\n", runtime.Version())
}

// ---------------------------------------------------
// Server IP Resolution
// ---------------------------------------------------

func resolveServerIP(serverURL string) (string, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return "", fmt.Errorf("parse server URL: %w", err)
	}

	host := strings.Split(u.Host, ":")[0]

	addrs, err := net.LookupIP(host)
	if err != nil {
		return "", fmt.Errorf("DNS lookup failed: %w", err)
	}

	for _, ip := range addrs {
		if ip.To4() != nil {
			return ip.String(), nil
		}
	}

	return "", fmt.Errorf("no IPv4 address found for %s", host)
}

// ---------------------------------------------------
// TUN init (native Wintun)
// ---------------------------------------------------

func (c *Client) initTUN(assignedIP string) error {
	if c.cfg.Name == "" {
		c.cfg.Name = "WSVPN"
	}

	if err := validateInterfaceName(c.cfg.Name); err != nil {
		return fmt.Errorf("invalid interface name: %w", err)
	}

	if assignedIP == "" {
		return fmt.Errorf("no IP assigned from server")
	}
	if net.ParseIP(assignedIP) == nil || net.ParseIP(assignedIP).To4() == nil {
		return fmt.Errorf("invalid IPv4 address from server: %s", assignedIP)
	}

	// Get executable directory to locate wintun.dll
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}
	exeDir := filepath.Dir(exePath)

	// Construct expected wintun.dll path
	dllPath := filepath.Join(exeDir, "wintun.dll")
	
	// Try drivers/wintun/arch/wintun.dll as fallback
	if _, err := os.Stat(dllPath); os.IsNotExist(err) {
		arch := getArchitecture()
		dllPath = filepath.Join(exeDir, "drivers", "wintun", arch, "wintun.dll")
	}

	log.Printf("Wintun DLL path: %s", dllPath)
	// Note: wintun.CreateAdapter() will auto-load wintun.dll from executable directory
	// The DLL should be placed in the same directory as the executable

	// Create/open Wintun adapter using pure Wintun API (no TAP dependencies)
	adapter, err := wintun.OpenAdapter(c.cfg.Name)
	if err != nil {
		log.Printf("Adapter %s not found, creating...", c.cfg.Name)
		// Create adapter with proper description that doesn't reference TAP
		adapter, err = wintun.CreateAdapter(c.cfg.Name, "WSVPN", nil)
		if err != nil {
			return fmt.Errorf("create Wintun adapter: %w", err)
		}
	}
	c.tun = adapter

	// Set IP address using netsh
	if err := runNetsh("interface", "ipv4", "set", "address", "name="+c.cfg.Name, "static", assignedIP, "255.255.255.0"); err != nil {
		return fmt.Errorf("netsh set address: %w", err)
	}

	// Start Wintun session with 1MB ring buffer
	session, err := adapter.StartSession(0x100000) // 1MB
	if err != nil {
		return fmt.Errorf("start Wintun session: %w", err)
	}
	c.tunSession = session

	log.Printf("Wintun interface %s created with IP %s", c.cfg.Name, assignedIP)
	return nil
}

// getPhysicalRouteInfo gets the physical interface and gateway for reaching serverIP
func getPhysicalRouteInfo(serverIP string) (string, string, error) {
	// Use route print to find the best route to serverIP
	cmd := exec.Command("route", "print", serverIP)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("route print: %w", err)
	}

	// Parse output to find interface and gateway
	// Example output:
	// Route Template: 0.0.0.0 -> 10.1.0.6
	// Network Destination  Netmask          Gateway       Interface  Metric
	// 10.1.0.6       255.255.255.255  10.1.0.1      10.1.0.100     25
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 5 && fields[0] == serverIP {
			gateway := fields[2]
			iface := fields[3]
			if gateway != "" && iface != "" {
				return iface, gateway, nil
			}
		}
	}

	return "", "", fmt.Errorf("no route found to %s", serverIP)
}

// runRouteAdd adds a TEMPORARY route using the route command (cleared on reboot)
func runRouteAdd(destination, mask, gateway, interface_ string) error {
	// route add <dest> mask <mask> <gateway> metric <metric> if <interface>
	// Note: NO -p flag to avoid permanent route pollution
	cmd := exec.Command("route", "add", destination, "mask", mask, gateway, "metric", "1", "if", interface_)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("route add: %s", string(out))
	}
	return nil
}

// runRouteDelete deletes a route (used for cleanup on disconnect)
func runRouteDelete(destination, mask string) error {
	cmd := exec.Command("route", "delete", destination, "mask", mask)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Route may not exist, log but don't fail
		log.Printf("Route delete (%s/%s) result: %s", destination, mask, string(out))
		return nil // Don't treat as error
	}
	log.Printf("Route deleted: %s/%s", destination, mask)
	return nil
}

// ---------------------------------------------------
// Input Validation
// ---------------------------------------------------

func validateInterfaceName(name string) error {
	if len(name) == 0 || len(name) > 32 {
		return fmt.Errorf("interface name must be 1-32 characters")
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			return fmt.Errorf("interface name contains invalid characters")
		}
	}
	return nil
}

func runNetsh(args ...string) error {
	allowedSubcommands := map[string]bool{
		"interface": true,
	}

	if len(args) < 2 {
		return fmt.Errorf("invalid netsh command")
	}

	if !allowedSubcommands[args[0]] {
		return fmt.Errorf("netsh subcommand not allowed")
	}

	c := exec.Command("netsh", args...)
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("netsh command failed: %s", string(out))
	}
	return nil
}

// ---------------------------------------------------
// Client lifecycle
// ---------------------------------------------------

func (c *Client) isRunning() bool {
	c.runningMu.RLock()
	defer c.runningMu.RUnlock()
	return c.running
}

func (c *Client) setRunning(running bool) {
	c.runningMu.Lock()
	defer c.runningMu.Unlock()
	c.running = running
}

func (c *Client) start() error {
	c.setRunning(true)
	c.stopCh = make(chan struct{})
	c.errCh = make(chan error, 3) // CRITICAL FIX #3: Buffer = 3 (one for each goroutine)

	// Initialize traffic shaper
	c.shape = obfuscation.NewShaperState(c.cfg.TrafficShape)

	go c.forwardToServer()
	go c.forwardFromServer()

	if c.cfg.Transport == "websocket" {
		go c.irregularHeartbeat()
	}

	// Connection lifetime rotation

	// Traffic induction
	if c.cfg.TrafficInduction {
		c.inductionCh = make(chan struct{})
		go c.trafficInductionLoop()
	}

	return nil
}

func (c *Client) stop() {
	if !c.isRunning() {
		return
	}
	c.setRunning(false)

	close(c.stopCh)
	if c.inductionCh != nil {
		close(c.inductionCh)
	}

	// Cleanup routes — only remove the server-specific host route, never touch default routes
	if c.serverIP != "" {
		runRouteDelete(c.serverIP, "255.255.255.255")
	}

	if c.conn != nil {
		c.conn.Close()
	}
	for _, pc := range c.peerConns {
		pc.Close()
	}
	if c.quicStream != nil {
		c.quicStream.Close()
	}
	if c.quicConn != nil {
		c.quicConn.CloseWithError(0, "client stopped")
	}
	
	// Cleanup Wintun
	if c.tunSession != (wintun.Session{}) {
		c.tunSession.End()
		c.tunSession = wintun.Session{}
	}
	if c.tun != nil {
		c.tun.Close()
		c.tun = nil
	}

	log.Println("Client stopped")
}

func (c *Client) waitForStop() error {
	for {
		select {
		case err := <-c.errCh:
			return err // Return first error, trigger reconnect in connectCmd
		case <-c.stopCh:
			return nil
		}
	}
}

// ---------------------------------------------------
// Transport
// ---------------------------------------------------

func (c *Client) connect() (string, error) {
	if c.cfg.Transport == "" {
		c.cfg.Transport = "websocket"
	}

	switch c.cfg.Transport {
	case "quic":
		return c.connectQUIC()
	case "websocket":
		return c.connectWebSocket()
	default:
		return "", fmt.Errorf("unknown transport: %s", c.cfg.Transport)
	}
}

// getUTLSClientHelloID maps config fingerprint name to uTLS ClientHelloID
func getUTLSClientHelloID(fingerprint string) utls.ClientHelloID {
	switch fingerprint {
	case obfuscation.TLSFingerprintFirefox:
		return utls.HelloFirefox_Auto
	case obfuscation.TLSFingerprintIOS:
		return utls.HelloIOS_Auto
	case obfuscation.TLSFingerprintEdge:
		return utls.HelloEdge_Auto
	case obfuscation.TLSFingerprintRandom:
		ids := []utls.ClientHelloID{
			utls.HelloChrome_Auto,
			utls.HelloFirefox_Auto,
			utls.HelloIOS_Auto,
			utls.HelloEdge_Auto,
		}
		return ids[time.Now().UnixNano()%int64(len(ids))]
	case obfuscation.TLSFingerprintChrome:
		fallthrough
	default:
		return utls.HelloChrome_Auto
	}
}

func (c *Client) connectWebSocket() (string, error) {
	if !strings.HasPrefix(c.cfg.ServerURL, "wss://") && !strings.HasPrefix(c.cfg.ServerURL, "ws://") {
		return "", fmt.Errorf("invalid server URL (must start with wss:// or ws://)")
	}

	if len(c.cfg.UUID) < 8 {
		return "", fmt.Errorf("invalid UUID (too short)")
	}

	u, err := url.Parse(c.cfg.ServerURL)
	if err != nil {
		return "", fmt.Errorf("invalid server URL: %w", err)
	}

	realHost := u.Hostname()
	connectHost := realHost
	sniHost := realHost

	// Domain fronting: TCP+TLS to CDN front, Host header to real server
	if c.cfg.FrontDomain != "" {
		fu, err := url.Parse(c.cfg.FrontDomain)
		if err == nil {
			connectHost = fu.Hostname()
		} else {
			connectHost = c.cfg.FrontDomain
		}
		sniHost = connectHost
	}

	port := u.Port()
	if port == "" {
		port = "443"
	}
	connectAddr := net.JoinHostPort(connectHost, port)
	realAddr := net.JoinHostPort(realHost, port)

	// Use ws:// scheme so gorilla doesn't do TLS; uTLS does it via NetDial.
	// URL uses the REAL host for Host header; TCP+TLS goes to the front domain.
	wsURL := fmt.Sprintf("ws://%s/ws/%s", realAddr, c.cfg.UUID)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		NetDial: func(network, addr string) (net.Conn, error) {
			// Connect to front domain (or real server if no fronting)
			tcpConn, err := net.DialTimeout(network, connectAddr, 10*time.Second)
			if err != nil {
				return nil, err
			}

			helloID := getUTLSClientHelloID(c.cfg.TLSFingerprint)
			utlsConn := utls.UClient(tcpConn, &utls.Config{
				ServerName: sniHost,
			}, helloID)
			if err := utlsConn.Handshake(); err != nil {
				tcpConn.Close()
				return nil, fmt.Errorf("uTLS handshake failed: %w", err)
			}

			return utlsConn, nil
		},
	}

	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return "", fmt.Errorf("dial WebSocket failed: %w", err)
	}
	c.conn = conn

	_, msg, err := c.conn.ReadMessage()
	if err != nil {
		return "", fmt.Errorf("read IP assignment failed: %w", err)
	}
	assignedIP := strings.TrimSpace(string(msg))

	if net.ParseIP(assignedIP) == nil {
		return "", fmt.Errorf("invalid IP from server: %s", assignedIP)
	}

	log.Printf("Connection established (TLS fingerprint: %s)", c.cfg.TLSFingerprint)
	return assignedIP, nil
}

func (c *Client) connectQUIC() (string, error) {
	if !strings.HasPrefix(c.cfg.ServerURL, "quic://") {
		return "", fmt.Errorf("invalid QUIC URL (must start with quic://)")
	}

	addr := strings.TrimPrefix(c.cfg.ServerURL, "quic://")
	if i := strings.Index(addr, "/"); i != -1 {
		addr = addr[:i]
	}

	// Use configured SNI or default
	sni := c.cfg.QUICSNI
	if sni == "" {
		sni = "app.glidesky.org"
	}

	tlsCfg := &tls.Config{
		ServerName: sni,
		NextProtos: []string{"h3"},
	}

	quicCfg := &quic.Config{
		MaxIdleTimeout:  30 * time.Second,
		EnableDatagrams: true,
		KeepAlivePeriod: 10 * time.Second,
	}

	conn, err := quic.DialAddr(context.Background(), addr, tlsCfg, quicCfg)
	if err != nil {
		return "", fmt.Errorf("dial QUIC failed: %w", err)
	}
	c.quicConn = conn

	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		conn.CloseWithError(0, "stream open failed")
		return "", fmt.Errorf("open stream failed: %w", err)
	}
	c.quicStream = stream

	if _, err := stream.Write([]byte(c.cfg.UUID)); err != nil {
		stream.Close()
		conn.CloseWithError(0, "UUID send failed")
		return "", fmt.Errorf("authentication failed: %w", err)
	}

	buf := make([]byte, 1024)
	n, err := stream.Read(buf)
	if err != nil {
		stream.Close()
		conn.CloseWithError(0, "IP read failed")
		return "", fmt.Errorf("read response failed: %w", err)
	}

	ip := string(buf[:n])
	if ip == "UNAUTHORIZED" {
		stream.Close()
		conn.CloseWithError(0, "unauthorized")
		return "", fmt.Errorf("authentication rejected")
	}

	assignedIP := strings.TrimSpace(ip)
	if net.ParseIP(assignedIP) == nil {
		return "", fmt.Errorf("invalid IP from server: %s", assignedIP)
	}

	log.Printf("Connection established")
	return assignedIP, nil
}

// ---------------------------------------------------
// Forwarding
// ---------------------------------------------------

func (c *Client) sendPacket(data []byte) error {
	if c.cfg.Transport == "quic" {
		if _, err := c.quicStream.Write(data); err != nil {
			return fmt.Errorf("QUIC write: %w", err)
		}
		return nil
	}

	// Traffic dispersion: round-robin across all connections
	idx := int(atomic.AddUint32(&c.sendIdx, 1))
	allConns := append([]*websocket.Conn{c.conn}, c.peerConns...)
	conn := allConns[idx%len(allConns)]

	c.wsWriteMu.Lock()
	err := conn.WriteMessage(websocket.BinaryMessage, data)
	c.wsWriteMu.Unlock()
	if err != nil {
		return fmt.Errorf("WebSocket write: %w", err)
	}
	return nil
}

func (c *Client) forwardToServer() {
	log.Printf("forwardToServer started (traffic shape: %s)", c.cfg.TrafficShape)
	packetCount := 0

	for {
		if !c.isRunning() {
			log.Printf("forwardToServer: client not running, exiting")
			return
		}

		// Read packet from Wintun session
		pkt, err := c.tunSession.ReceivePacket()
		if err != nil {
			time.Sleep(time.Millisecond)
			continue
		}

		packetCount++
		if packetCount <= 10 || packetCount%100 == 0 {
			log.Printf("TUN read: %d bytes (packet #%d)", len(pkt), packetCount)
		}

		// Check obfuscation setting (string: "on"/"off")
		obfuscationEnabled := (c.cfg.Obfuscation == "" || c.cfg.Obfuscation == "on" || c.cfg.Obfuscation == "true" || c.cfg.Obfuscation == "1")

		var send []byte
		if obfuscationEnabled {
			send = obfuscation.SimulateHTTPSPattern(pkt)
		} else {
			send = pkt
		}

		c.tunSession.ReleaseReceivePacket(pkt)

		// Apply traffic shaping
		delay, shouldBuffer := c.shape.NextDelay()
		if shouldBuffer {
			dataCopy := make([]byte, len(send))
			copy(dataCopy, send)
			go func(data []byte, d time.Duration) {
				select {
				case <-time.After(d):
					_ = c.sendPacket(data)
				case <-c.stopCh:
				}
			}(dataCopy, delay)
			continue
		} else if delay > 0 {
			time.Sleep(delay)
		}

		if err := c.sendPacket(send); err != nil {
			c.sendError(err)
			return
		}
	}
}

func (c *Client) forwardFromServer() {
	log.Printf("forwardFromServer started")
	buf := make([]byte, 2048)
	packetCount := 0

	if c.cfg.Transport == "quic" {
		for {
			if !c.isRunning() {
				return
			}

			n, err := c.quicStream.Read(buf)
			if err != nil {
				c.sendError(fmt.Errorf("QUIC read: %w", err))
				return
			}

			pkt := buf[:n]
			// Check obfuscation setting (string: "on"/"off")
			obfuscationEnabled := (c.cfg.Obfuscation == "" || c.cfg.Obfuscation == "on" || c.cfg.Obfuscation == "true" || c.cfg.Obfuscation == "1")
			if obfuscationEnabled {
				if pb, err := obfuscation.RemovePadding(pkt); err == nil {
					pkt = pb
				}
			}

			// Write packet to Wintun
			if err := c.writeWintunPacket(pkt); err != nil {
				c.sendError(fmt.Errorf("Wintun write: %w", err))
				return
			}
		}
	} else {
		for {
			if !c.isRunning() {
				log.Printf("forwardFromServer: client not running, exiting")
				return
			}

			_, data, err := c.conn.ReadMessage()
			if err != nil {
				c.sendError(fmt.Errorf("WebSocket read: %w", err))
				return
			}

			packetCount++
			if packetCount <= 10 || packetCount % 100 == 0 {
				log.Printf("WebSocket read: %d bytes (packet #%d)", len(data), packetCount)
			}

			// Check obfuscation setting (string: "on"/"off")
			obfuscationEnabled := (c.cfg.Obfuscation == "" || c.cfg.Obfuscation == "on" || c.cfg.Obfuscation == "true" || c.cfg.Obfuscation == "1")
			if obfuscationEnabled {
				if pb, err := obfuscation.RemovePadding(data); err == nil {
					data = pb
				}
			}

			// Write packet to Wintun
			if err := c.writeWintunPacket(data); err != nil {
				c.sendError(fmt.Errorf("Wintun write: %w", err))
				return
			}
		}
	}
}

// writeWintunPacket writes a packet to the Wintun device
func (c *Client) writeWintunPacket(data []byte) error {
	packet, err := c.tunSession.AllocateSendPacket(len(data))
	if err != nil {
		return fmt.Errorf("allocate Wintun packet: %w", err)
	}
	copy(packet, data)
	c.tunSession.SendPacket(packet)
	return nil
}

// sendError sends error to errCh without blocking (CRITICAL FIX #3)
func (c *Client) sendError(err error) {
	select {
	case c.errCh <- err:
	default:
		// Channel full, error already sent
	}
}

// ---------------------------------------------------
// Heartbeat (WebSocket only)
// ---------------------------------------------------

// connectionLifetimeLoop triggers reconnect via the main loop by closing the current connection.

// trafficInductionLoop generates lightweight fake HTTP requests during idle.
func (c *Client) trafficInductionLoop() {
	domains := c.cfg.InductionDomains
	if len(domains) == 0 {
		domains = []string{"http://httpbin.org/get", "http://example.com", "http://httpstat.us/200"}
	}

	for {
		interval := 30 + time.Duration(time.Now().UnixNano()%270)*time.Second

		select {
		case <-time.After(interval):
			if !c.isRunning() {
				return
			}
			url := domains[time.Now().UnixNano()%int64(len(domains))]
			resp, err := httpGet(url)
			if err != nil {
				continue
			}
			if resp != nil {
				resp.Body.Close()
			}
		case <-c.inductionCh:
			return
		}
	}
}

// httpGet performs a lightweight HTTP GET through the VPN tunnel.
func httpGet(url string) (*http.Response, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	return client.Do(req)
}

func (c *Client) irregularHeartbeat() {
	for {
		if !c.isRunning() {
			return
		}

		interval := obfuscation.GetHeartbeatInterval()
		select {
		case <-time.After(interval):
			// CRITICAL FIX #1: Protect WebSocket write with mutex
			c.wsWriteMu.Lock()
			err := c.conn.WriteMessage(websocket.PingMessage, nil)
			c.wsWriteMu.Unlock()
			if err != nil {
				c.sendError(fmt.Errorf("ping: %w", err))
				return
			}
		case <-c.stopCh:
			return
		}
	}
}

// ---------------------------------------------------
// Config helper
// ---------------------------------------------------

func validateConfig(cfg *Config) error {
	if cfg.ServerURL == "" {
		return fmt.Errorf("server_url is required")
	}
	if !strings.HasPrefix(cfg.ServerURL, "wss://") &&
		!strings.HasPrefix(cfg.ServerURL, "ws://") &&
		!strings.HasPrefix(cfg.ServerURL, "quic://") {
		return fmt.Errorf("server_url must start with wss://, ws://, or quic://")
	}

	if cfg.UUID == "" {
		return fmt.Errorf("uuid is required")
	}
	if len(cfg.UUID) < 8 {
		return fmt.Errorf("uuid must be at least 8 characters")
	}

	if cfg.ClientIP != "" {
		if net.ParseIP(cfg.ClientIP) == nil {
			return fmt.Errorf("client_ip is not a valid IP address")
		}
	}

	if cfg.Transport != "" && cfg.Transport != "websocket" && cfg.Transport != "quic" {
		return fmt.Errorf("transport must be 'websocket' or 'quic'")
	}

	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if cfg.LogLevel != "" && !validLevels[cfg.LogLevel] {
		return fmt.Errorf("log_level must be debug, info, warn, or error")
	}

	if cfg.DNS != "" {
		if net.ParseIP(cfg.DNS) == nil {
			return fmt.Errorf("dns is not a valid IP address")
		}
	}

	return nil
}

func loadConfig(path string) (*Config, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("access config file: %w", err)
	}

	// Windows-only build, just log file info
	log.Printf("Config file: %s (size: %d bytes)", path, info.Size())

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Apply defaults
	if cfg.TLSFingerprint == "" {
		cfg.TLSFingerprint = obfuscation.TLSFingerprintChrome
	}
	if cfg.TrafficShape == "" {
		cfg.TrafficShape = "off"
	}

	if err := validateConfig(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}
