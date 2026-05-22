package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/quic-go/quic-go"
	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
	"wsvpn/logger"
	"wsvpn/obfuscation"
)

const Version = "v0.4.9"

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
	Name        string `json:"name"`
	ClientIP    string `json:"client_ip"`
	ServerURL   string `json:"server_url"`
	UUID        string `json:"uuid"`
	Reconnect   bool   `json:"reconnect"`
	LogLevel    string `json:"log_level"`
	LogDir      string `json:"log_dir"`
	Obfuscation bool   `json:"obfuscation"`
	Transport   string `json:"transport"`   // websocket or quic
	QUICSNI     string `json:"quic_sni"`    // SNI hostname for QUIC TLS (defaults to server hostname)
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

// connectWebSocket establishes WebSocket connection to server
func (c *Client) connectWebSocket() error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	// Build WebSocket URL with UUID: ws://server:port/ws/{uuid}
	wsURL := fmt.Sprintf("%s/ws/%s", c.config.ServerURL, c.config.UUID)

	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		structuredLog.Error("websocket_connect", "Failed to connect to server", map[string]interface{}{
			"url":   wsURL,
			"error": err.Error(),
		})
		return fmt.Errorf("failed to connect to server (%s): %w", wsURL, err)
	}

	c.conn = conn
	structuredLog.Info("websocket_connected", "Connected to WebSocket server", map[string]interface{}{
		"url": wsURL,
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

		// Send based on transport type
		if c.config.Transport == "quic" {
			if _, err := c.quicStream.Write(sendData); err != nil {
				structuredLog.Error("quic_write", "Failed to write to QUIC stream", map[string]interface{}{
					"error": err.Error(),
				})
				return
			}
		} else {
			if err := c.conn.WriteMessage(websocket.BinaryMessage, sendData); err != nil {
				structuredLog.Error("ws_write", "Failed to write to WebSocket", map[string]interface{}{
					"error": err.Error(),
				})
				return
			}
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

// reconnect implements reconnection logic with exponential backoff
func (c *Client) reconnect() {
	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second

	for c.running {
		structuredLog.Info("reconnect_wait", "Attempting to reconnect", map[string]interface{}{
			"backoff": backoff.String(),
		})
		time.Sleep(backoff)

		if err := c.connect(); err != nil {
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
		c.forwardFromServer()
	}
}

// start begins the VPN client operation
func (c *Client) start() error {
	c.running = true
	c.stopCh = make(chan struct{})

	// Initial connection
	if err := c.connect(); err != nil {
		return fmt.Errorf("initial connection failed: %w", err)
	}

	// Start packet forwarding
	go c.forwardToServer()
	go c.forwardFromServer()

	// Start irregular heartbeat (DPI evasion)
	go c.irregularHeartbeat()

	return nil
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
	if c.conn != nil {
		c.conn.Close()
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

	// Parse command-line flags
	configPath := flag.String("config", "client.json", "Path to client configuration file")
	version := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	// Print version if requested
	if *version {
		fmt.Printf("WSVPN Client %s\n", Version)
		fmt.Printf("  Go Version: %s\n", runtime.Version())
		fmt.Printf("  OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		os.Exit(0)
	}

	// Load configuration
	config, err := loadConfig(*configPath)
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
