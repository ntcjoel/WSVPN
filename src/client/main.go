package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	mrand "math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/quic-go/quic-go"
	utls "github.com/refraction-networking/utls"
	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
	"wsvpn/logger"
	"wsvpn/obfuscation"
)

const Version = "v1.2"

// packetPool for buffer reuse to reduce GC pressure
var packetPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 2048)
	},
}

// Global structured logger instance (named differently to avoid conflict with stdlib log)
type logAlias = logger.Logger
var structuredLog *logAlias

// Config represents the client configuration
type Config struct {
	Name              string `json:"name"`
	ClientIP          string `json:"client_ip"`
	ServerURL         string `json:"server_url"`
	UUID              string `json:"uuid"`
	Reconnect         bool   `json:"reconnect"`
	LogLevel          string `json:"log_level"`
	LogDir            string `json:"log_dir"`
	Obfuscation    bool   `json:"obfuscation"`
	Transport      string `json:"transport"`       // websocket or quic
	QUICSNI        string `json:"quic_sni"`        // SNI hostname for QUIC TLS (defaults to server hostname)
	TLSFingerprint string `json:"tls_fingerprint"` // Browser TLS fingerprint: chrome, firefox, ios, edge, random
	TrafficShape       string   `json:"traffic_shape"`       // Traffic shaping mode: off, jitter, browse, adaptive
	FrontDomain        string   `json:"front_domain"`        // CDN front domain for domain fronting: SNI=front, Host=real
	DispersionPeers    []string `json:"dispersion_peers"`    // Additional server URLs for traffic dispersion
	TrafficInduction   bool     `json:"traffic_induction"`   // Generate fake browsing noise during idle
	InductionDomains   []string `json:"induction_domains"`   // Domains to use for traffic induction
}

// Client represents the WSVPN client
type Client struct {
	config     *Config
	tunDevice  *water.Interface
	conn       *websocket.Conn
	quicConn   *quic.Conn
	quicStream *quic.Stream
	running    bool
	stopCh     chan struct{}
	shape       *obfuscation.ShaperState
	peerConns   []*websocket.Conn // Extra connections for traffic dispersion
	sendIdx     uint32             // Atomic counter for round-robin sending
	inductionCh chan struct{}      // Close to stop traffic induction
}

// loadConfig loads configuration from JSON file
func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Smart defaults — only server_url, uuid, client_ip are required
	if config.Name == "" {
		config.Name = "wsvpn-client"
	}
	if config.Transport == "" {
		config.Transport = "websocket"
	}
	if config.LogLevel == "" {
		config.LogLevel = "info"
	}
	if config.Reconnect {
		// reconnect is true by default when field present
	}
	if config.Obfuscation {
		// obfuscation is true by default
	}
	if config.TLSFingerprint == "" {
		config.TLSFingerprint = obfuscation.TLSFingerprintChrome
	}
	if config.TrafficShape == "" {
		config.TrafficShape = "off"
	}

	// Validate TLS fingerprint
	validFingerprints := obfuscation.ValidTLSFingerprints()
	fpValid := false
	for _, fp := range validFingerprints {
		if config.TLSFingerprint == fp {
			fpValid = true
			break
		}
	}
	if !fpValid {
		return nil, fmt.Errorf("invalid tls_fingerprint: %q (valid: %v)", config.TLSFingerprint, validFingerprints)
	}

	// Validate traffic shape
	validShapes := obfuscation.ValidTrafficShapes()
	shapeValid := false
	for _, s := range validShapes {
		if config.TrafficShape == s {
			shapeValid = true
			break
		}
	}
	if !shapeValid {
		return nil, fmt.Errorf("invalid traffic_shape: %q (valid: %v)", config.TrafficShape, validShapes)
	}

	return &config, nil
}

// initTUN initializes the TUN/TAP interface
func (c *Client) initTUN() error {
	ifce, err := water.New(water.Config{
		DeviceType: water.TUN,
		PlatformSpecificParams: water.PlatformSpecificParams{
			Name: c.config.Name,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create TUN interface: %w", err)
	}
	c.tunDevice = ifce

	// Bring up the interface
	link, err := netlink.LinkByName(c.config.Name)
	if err != nil {
		return fmt.Errorf("failed to get link: %w", err)
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("failed to bring up interface: %w", err)
	}

	// Auto-adjust TUN MTU when obfuscation is enabled to avoid outer-layer fragmentation.
	// Overhead: header(4) + max_padding(500) + WS_frame(10) + TLS_record(29) = 543.
	// 1500 - 543 = 957. With smart ACK padding, 1200 is safe for the common case.
	if c.config.Obfuscation {
		tunMTU := 1300
		if err := netlink.LinkSetMTU(link, tunMTU); err != nil {
			structuredLog.Warn("tun_mtu", "Failed to set TUN MTU", nil)
		}
	}

	// Set IP address
	addr, err := netlink.ParseAddr(c.config.ClientIP + "/24")
	if err != nil {
		return fmt.Errorf("failed to parse address: %w", err)
	}

	if err := netlink.AddrAdd(link, addr); err != nil {
		return fmt.Errorf("failed to add address: %w", err)
	}

	structuredLog.Info("tun_init", "TUN interface initialized", map[string]interface{}{
		"name": c.config.Name,
		"ip":   c.config.ClientIP,
	})
	return nil
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

// connectWebSocket establishes WebSocket connection to server with uTLS fingerprint camouflage.
// When FrontDomain is configured, uses domain fronting: TCP+TLS to CDN, Host header to real server.
func (c *Client) connectWebSocket() error {
	u, err := url.Parse(c.config.ServerURL)
	if err != nil {
		return fmt.Errorf("invalid server URL: %w", err)
	}

	realHost := u.Hostname()
	connectHost := realHost // Host we actually TCP+TLS connect to
	sniHost := realHost     // TLS SNI

	// Domain fronting: connect to CDN front domain, SNI = front, Host header = real server
	if c.config.FrontDomain != "" {
		fu, err := url.Parse(c.config.FrontDomain)
		if err == nil {
			connectHost = fu.Hostname()
		} else {
			connectHost = c.config.FrontDomain
		}
		sniHost = connectHost
	}

	port := u.Port()
	if port == "" {
		port = "443"
	}
	connectAddr := net.JoinHostPort(connectHost, port)
	realAddr := net.JoinHostPort(realHost, port)

	// Use ws:// scheme so gorilla doesn't do TLS; uTLS handles it in NetDial when using WSS.
	// URL uses the REAL host for the Host header; TCP+TLS goes to the front domain.
	scheme := "ws"
	if u.Scheme == "wss" {
		scheme = "ws" // gorilla doesn't do TLS
	}
	wsURL := fmt.Sprintf("%s://%s/ws/%s", scheme, realAddr, c.config.UUID)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		NetDial: func(network, addr string) (net.Conn, error) {
			tcpConn, err := net.DialTimeout(network, connectAddr, 10*time.Second)
			if err != nil {
				return nil, err
			}

			// If not WSS, return plain TCP (for local testing without TLS)
			if u.Scheme != "wss" {
				return tcpConn, nil
			}

			// Perform uTLS handshake with SNI = front/connect domain
			helloID := getUTLSClientHelloID(c.config.TLSFingerprint)
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
		structuredLog.Error("websocket_connect", "Failed to connect to server", map[string]interface{}{
			"url":         wsURL,
			"fingerprint": c.config.TLSFingerprint,
			"error":       err.Error(),
		})
		return fmt.Errorf("failed to connect to server (%s): %w", wsURL, err)
	}

	c.conn = conn
	structuredLog.Info("websocket_connected", "Connected to WebSocket server", map[string]interface{}{
		"url":         wsURL,
		"fingerprint": c.config.TLSFingerprint,
	})

	// Read assigned IP from server
	_, message, err := conn.ReadMessage()
	if err != nil {
		structuredLog.Error("websocket_ip", "Failed to receive IP from server", map[string]interface{}{
			"error": err.Error(),
		})
		return fmt.Errorf("failed to receive IP from server: %w", err)
	}

	serverIP := string(message)
	structuredLog.Info("websocket_ip", "Server assigned IP", map[string]interface{}{
		"ip": serverIP,
	})

	return nil
}

// connect establishes connection using configured transport
func (c *Client) connect() error {
	// Default to websocket for backward compatibility
	if c.config.Transport == "" {
		c.config.Transport = "websocket"
	}

	switch c.config.Transport {
	case "quic":
		return c.connectQUIC()
	case "websocket":
		fallthrough
	default:
		return c.connectWebSocket()
	}
}

// getSNI returns the SNI hostname for QUIC, using config value
// or falling back to the hostname extracted from ServerURL
func (c *Client) getSNI() string {
	if c.config.QUICSNI != "" {
		return c.config.QUICSNI
	}
	// Derive from ServerURL: strip scheme and path
	host := strings.TrimPrefix(c.config.ServerURL, "wss://")
	host = strings.TrimPrefix(host, "ws://")
	host = strings.TrimPrefix(host, "quic://")
	if idx := strings.Index(host, "/"); idx != -1 {
		host = host[:idx]
	}
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}
	return host
}

// connectQUIC establishes QUIC connection to server
func (c *Client) connectQUIC() error {
	// Parse server URL (quic://server:port or just server:port)
	serverAddr := strings.TrimPrefix(c.config.ServerURL, "quic://")
	
	// Remove any path component (e.g., /ws/uuid)
	if idx := strings.Index(serverAddr, "/"); idx != -1 {
		serverAddr = serverAddr[:idx]
	}

	tlsConfig := &tls.Config{
		ServerName: c.getSNI(), // Resolved from config or derived from ServerURL
		NextProtos: []string{"h3"},     // HTTP/3 protocol
	}

	quicConfig := &quic.Config{
		MaxIdleTimeout:  30 * time.Second,
		EnableDatagrams: true,
		KeepAlivePeriod: 10 * time.Second,
	}

	// Dial QUIC connection
	conn, err := quic.DialAddr(context.Background(), serverAddr, tlsConfig, quicConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to QUIC server (%s): %w", serverAddr, err)
	}

	c.quicConn = conn

	// Open stream for data transfer
	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		conn.CloseWithError(0, "stream open failed")
		return fmt.Errorf("failed to open stream: %w", err)
	}

	c.quicStream = stream

	// Send UUID to server
	if _, err := stream.Write([]byte(c.config.UUID)); err != nil {
		stream.Close()
		conn.CloseWithError(0, "UUID send failed")
		return fmt.Errorf("failed to send UUID: %w", err)
	}

	// Read assigned IP from server
	ipBuf := make([]byte, 1024)
	n, err := stream.Read(ipBuf)
	if err != nil {
		stream.Close()
		conn.CloseWithError(0, "IP read failed")
		return fmt.Errorf("failed to receive IP from server: %w", err)
	}

	serverIP := string(ipBuf[:n])
	if serverIP == "UNAUTHORIZED" {
		stream.Close()
		conn.CloseWithError(0, "unauthorized")
		return fmt.Errorf("unauthorized UUID: %s", c.config.UUID)
	}

	structuredLog.Info("quic_connected", "Connected via QUIC", map[string]interface{}{
		"server": serverAddr,
		"ip":     serverIP,
	})
	return nil
}

// sendPacket writes data to the active transport, dispersing across peer connections.
func (c *Client) sendPacket(data []byte) error {
	if c.config.Transport == "quic" {
		if _, err := c.quicStream.Write(data); err != nil {
			return fmt.Errorf("QUIC write: %w", err)
		}
		return nil
	}

	// Traffic dispersion: round-robin across all connections
	idx := int(atomic.AddUint32(&c.sendIdx, 1))
	allConns := append([]*websocket.Conn{c.conn}, c.peerConns...)
	conn := allConns[idx%len(allConns)]

	if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return fmt.Errorf("WebSocket write: %w", err)
	}
	return nil
}

// forwardToServer forwards packets from TUN to server (supports both transports)
func (c *Client) forwardToServer() {
	buffer := packetPool.Get().([]byte)
	defer packetPool.Put(buffer)

	for c.running {
		select {
		case <-c.stopCh:
			return
		default:
		}

		n, err := c.tunDevice.Read(buffer)
		if err != nil {
			if err != io.EOF && c.running {
				structuredLog.Error("tun_read", "Failed to read from TUN", map[string]interface{}{
					"error": err.Error(),
				})
			}
			return
		}

		packet := buffer[:n]

		// Add obfuscation padding before sending
		var sendData []byte
		if c.config.Obfuscation {
			sendData = obfuscation.SimulateHTTPSPattern(packet)
		} else {
			sendData = packet
		}

		// Apply traffic shaping
		delay, shouldBuffer := c.shape.NextDelay()
		if shouldBuffer {
			// Pause — buffer packet and send after delay via goroutine
			dataCopy := make([]byte, len(sendData))
			copy(dataCopy, sendData)
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

		if err := c.sendPacket(sendData); err != nil {
			structuredLog.Error("send_packet", "Failed to send packet", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
	}
}

// forwardFromServer forwards packets from server to TUN (supports both transports)
func (c *Client) forwardFromServer() {
	buffer := packetPool.Get().([]byte)
	defer packetPool.Put(buffer)

	if c.config.Transport == "quic" {
		c.forwardFromQUIC(buffer)
	} else {
		c.forwardFromWebSocket(buffer)
	}
}

// forwardFromWebSocket forwards packets from WebSocket server to TUN
func (c *Client) forwardFromWebSocket(buffer []byte) {
	for c.running {
		messageType, data, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				structuredLog.Warn("ws_disconnect", "WebSocket disconnected unexpectedly", map[string]interface{}{
					"error": err.Error(),
				})
			}
			return
		}

		if messageType != websocket.BinaryMessage {
			continue
		}

		// Remove obfuscation padding
		var packet []byte
		if c.config.Obfuscation {
			var err error
			packet, err = obfuscation.RemovePadding(data)
			if err != nil {
				structuredLog.Warn("obfuscation_remove", "Failed to remove padding from WebSocket data", map[string]interface{}{
					"error": err.Error(),
				})
				continue
			}
		} else {
			packet = data
		}

		// Copy to pooled buffer for consistent memory management
		copy(buffer, packet)
		packet = buffer[:len(packet)]

		// Write packet to TUN interface
		if _, err := c.tunDevice.Write(packet); err != nil {
			structuredLog.Error("tun_write", "Failed to write to TUN (WebSocket)", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
	}
}

// forwardFromQUIC forwards packets from QUIC server to TUN
func (c *Client) forwardFromQUIC(buffer []byte) {
	for c.running {
		n, err := c.quicStream.Read(buffer)
		if err != nil {
			structuredLog.Error("quic_read", "QUIC stream read error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}

		// Remove obfuscation padding
		var packet []byte
		if c.config.Obfuscation {
			var err error
			packet, err = obfuscation.RemovePadding(buffer[:n])
			if err != nil {
				structuredLog.Warn("obfuscation_remove", "Failed to remove padding from QUIC data", map[string]interface{}{
					"error": err.Error(),
				})
				continue
			}
		} else {
			packet = buffer[:n]
		}

		// Write packet to TUN interface
		if _, err := c.tunDevice.Write(packet); err != nil {
			structuredLog.Error("tun_write", "Failed to write to TUN (QUIC)", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
	}
}

// reconnect implements reconnection logic with exponential backoff.
// It is called on connection loss.
func (c *Client) reconnect() {
	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second

	for {
		structuredLog.Info("reconnect_wait", "Attempting to reconnect", map[string]interface{}{
			"backoff": backoff.String(),
		})
		time.Sleep(backoff)

		// Reset running state and stopCh for a fresh connection cycle
		c.running = true
		c.stopCh = make(chan struct{})
		c.shape = obfuscation.NewShaperState(c.config.TrafficShape)

		if err := c.connect(); err != nil {
			c.running = false
			structuredLog.Warn("reconnect_fail", "Reconnection failed", map[string]interface{}{
				"error": err.Error(),
			})
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		// Reset backoff on successful connection
		backoff = 1 * time.Second
		structuredLog.Info("reconnect_success", "Reconnected successfully", nil)

		// Restart packet forwarding
		go c.forwardToServer()
		go c.forwardFromServer()
		go c.irregularHeartbeat()

		if c.config.TrafficInduction {
			c.inductionCh = make(chan struct{})
			go c.trafficInductionLoop()
		}

		c.forwardFromServer()
		return
	}
}

// start begins the VPN client operation

func (c *Client) start() error {
	c.running = true
	c.stopCh = make(chan struct{})

	// Initialize traffic shaper
	c.shape = obfuscation.NewShaperState(c.config.TrafficShape)

	// Initial connection
	if err := c.connect(); err != nil {
		return fmt.Errorf("initial connection failed: %w", err)
	}

	// Start packet forwarding
	go c.forwardToServer()
	go c.forwardFromServer()

	// Connect to dispersion peers (additional servers for traffic distribution)
	for _, peerURL := range c.config.DispersionPeers {
		// Temporarily swap config to connect to peer
		savedURL := c.config.ServerURL
		savedFront := c.config.FrontDomain
		c.config.ServerURL = peerURL
		c.config.FrontDomain = "" // Peer connections use direct URLs (no domain fronting)
		if err := c.connect(); err != nil {
			structuredLog.Warn("dispersion_peer", "Failed to connect to peer", map[string]interface{}{
				"peer":  peerURL,
				"error": err.Error(),
			})
			c.config.ServerURL = savedURL
			c.config.FrontDomain = savedFront
			continue
		}
		c.peerConns = append(c.peerConns, c.conn)
		structuredLog.Info("dispersion_peer", "Connected to peer", map[string]interface{}{
			"peer": peerURL,
		})
		c.config.ServerURL = savedURL
		c.config.FrontDomain = savedFront
	}

	// Start irregular heartbeat (DPI evasion)
	go c.irregularHeartbeat()



	// Traffic induction — generate fake browsing noise during idle
	if c.config.TrafficInduction {
		c.inductionCh = make(chan struct{})
		go c.trafficInductionLoop()
	}

	return nil
}

// trafficInductionLoop generates lightweight fake HTTP requests to random public sites
// during idle periods. This creates background noise that makes the connection look like
// normal multi-site browsing rather than a single persistent tunnel.
func (c *Client) trafficInductionLoop() {
	defaultDomains := []string{
		"http://httpbin.org/get",
		"http://example.com",
		"http://httpstat.us/200",
	}
	domains := c.config.InductionDomains
	if len(domains) == 0 {
		domains = defaultDomains
	}

	for {
		// Random interval: 30s to 5min
		interval := 30 + time.Duration(mrand.Int63n(270)) * time.Second

		select {
		case <-time.After(interval):
			if !c.running {
				return
			}
			// Pick a random domain and make a lightweight GET request
			url := domains[mrand.Intn(len(domains))]
			resp, err := httpGet(url)
			if err != nil {
				structuredLog.Debug("induction", "Induction request failed", map[string]interface{}{
					"url":   url,
					"error": err.Error(),
				})
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
// We use the system's default HTTP client — the request goes through the TUN interface.
func httpGet(url string) (*http.Response, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	return client.Do(req)
}

// irregularHeartbeat sends ping messages at irregular intervals (WebSocket only)
// QUIC has built-in keepalive, so heartbeat is only needed for WebSocket
func (c *Client) irregularHeartbeat() {
	// Skip heartbeat for QUIC (has built-in keepalive)
	if c.config.Transport == "quic" {
		return
	}

	for c.running {
		// Random interval 30-90 seconds (like different user behaviors)
		interval := obfuscation.GetHeartbeatInterval()

		select {
		case <-time.After(interval):
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				structuredLog.Warn("heartbeat_fail", "Ping failed", map[string]interface{}{
					"error": err.Error(),
				})
				if c.config.Reconnect {
					c.running = false
					close(c.stopCh)
					c.conn.Close()
					go c.reconnect()
					return
				}
			}
		case <-c.stopCh:
			return
		}
	}
}

// stop gracefully stops the client
func (c *Client) stop() {
	c.running = false
	close(c.stopCh)
	if c.inductionCh != nil {
		close(c.inductionCh)
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
		c.quicConn.CloseWithError(0, "client stopping")
	}
	if c.tunDevice != nil {
		c.tunDevice.Close()
	}
	structuredLog.Info("client_stopped", "Client stopped", nil)
}

func main() {
	// Initialize obfuscation with secure random seed
	obfuscation.InitObfuscation()

	// Parse flags — minimal: -config (default auto-discovery), -version, -help
	cfgFile := "client.json"
	showVersion := false
	for i, a := range os.Args[1:] {
		switch a {
		case "-config", "--config":
			if i+2 < len(os.Args) {
				cfgFile = os.Args[i+2]
			}
		case "-version", "--version":
			showVersion = true
		case "-h", "-help", "--help":
			fmt.Printf("WSVPN Client %s\n\nUsage: wsvpn-client [-config file] [-version]\n", Version)
			os.Exit(0)
		}
	}
	if showVersion {
		fmt.Printf("WSVPN Client %s\n  Go: %s\n  OS: %s/%s\n", Version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		os.Exit(0)
	}

	config, err := loadConfig(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Set default log directory if not specified
	if config.LogDir == "" {
		if runtime.GOOS == "windows" {
			config.LogDir = "C:\\wsvpn\\logs"
		} else {
			config.LogDir = "/var/log/wsvpn/client"
		}
	}

	// Set default transport to websocket for backward compatibility
	if config.Transport == "" {
		config.Transport = "websocket"
	}

	// Initialize structured logger
	structuredLog, err = logger.New("client", config.LogDir, logger.ParseLevel(config.LogLevel))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer structuredLog.Close()

	structuredLog.Info("client_start", "WSVPN Client starting", map[string]interface{}{
		"version":     Version,
		"transport":   config.Transport,
		"server_url":  config.ServerURL,
		"obfuscation": config.Obfuscation,
	})

	// Validate UUID is set
	if config.UUID == "" {
		structuredLog.Error("config_error", "UUID is required in client configuration", nil)
		os.Exit(1)
	}

	// Create client instance
	client := &Client{
		config: config,
	}

	// Initialize TUN interface
	if err := client.initTUN(); err != nil {
		structuredLog.Error("tun_init", "Failed to initialize TUN", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	// Start client
	if err := client.start(); err != nil {
		structuredLog.Error("client_start", "Client failed", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	// Keep running
	select {}
}
