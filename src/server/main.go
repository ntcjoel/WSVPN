package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/gorilla/websocket"
	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
	"wsvpn/logger"
	"wsvpn/obfuscation"
)

const Version = "v1.2"

// Config represents the server configuration
type Config struct {
	Name                     string `json:"name"`
	Network                  string `json:"network"`
	ServerIP                 string `json:"server_ip"`
	ListenAddr               string `json:"listen_addr"`
	QUICListenAddr           string `json:"quic_listen_addr"`
	WebSocketPath            string `json:"websocket_path"`
	ClientsFile              string `json:"clients_file"`
	LogLevel                 string `json:"log_level"`
	LogDir                   string `json:"log_dir"`
	Obfuscation   bool   `json:"obfuscation"`
	AdminToken    string `json:"admin_token"`
	Transport     string `json:"transport"`      // websocket, quic, or both
	SOCKS5Enabled bool   `json:"socks5_enabled"` // Built-in SOCKS5 proxy (default: true)
	SOCKS5Port    int    `json:"socks5_port"`    // SOCKS5 proxy port (default: 1744)
}

// Global structured logger instance (named differently to avoid conflict with stdlib log)
var structuredLog *logger.Logger

// Client represents a connected VPN client
type Client struct {
	ID                 string
	Conn               *websocket.Conn
	IP                 net.IP
	IPStr              string
	UUID   string
	stopCh chan struct{}
	writeMu sync.Mutex // protects Conn.WriteMessage from concurrent writes
}

// Server represents the WSVPN server
type Server struct {
	config        atomic.Pointer[Config]
	tunDevice     *water.Interface
	clients       map[string]*Client  // ID -> Client
	ipRoute       map[string]string   // IP -> ClientID
	routeMu       sync.RWMutex
	clientsMu     sync.RWMutex
	upgrader      websocket.Upgrader
	clientManager *ClientManager
	metrics       *Metrics
	ctx           context.Context
	cancel        context.CancelFunc
}

// packetPool for buffer reuse to reduce GC pressure
var packetPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 2048)
	},
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

// getConfig returns the current configuration atomically
func (s *Server) getConfig() *Config {
	return s.config.Load()
}

// setConfig updates the configuration atomically
func (s *Server) setConfig(cfg *Config) {
	s.config.Store(cfg)
}

// initTUN initializes the TUN/TAP interface
func (s *Server) initTUN() error {
	config := s.getConfig()
	ifce, err := water.New(water.Config{
		DeviceType: water.TUN,
		PlatformSpecificParams: water.PlatformSpecificParams{
			Name: config.Name,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create TUN interface: %w", err)
	}
	s.tunDevice = ifce

	// Bring up the interface
	link, err := netlink.LinkByName(config.Name)
	if err != nil {
		return fmt.Errorf("failed to get link: %w", err)
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("failed to bring up interface: %w", err)
	}

	// Set IP address
	addr, err := netlink.ParseAddr(config.ServerIP + "/24")
	if err != nil {
		return fmt.Errorf("failed to parse address: %w", err)
	}

	if err := netlink.AddrAdd(link, addr); err != nil {
		return fmt.Errorf("failed to add address: %w", err)
	}

	structuredLog.Info("tun_init", "TUN interface initialized", map[string]interface{}{
		"name": config.Name,
		"ip":   config.ServerIP,
	})
	return nil
}

// getDstIP extracts destination IP from IP packet
func getDstIP(packet []byte) net.IP {
	if len(packet) < 20 {
		structuredLog.Debug("packet_read", "Packet too small", map[string]interface{}{
			"bytes": len(packet),
			"hex":   fmt.Sprintf("%x", packet[:min(len(packet), 32)]),
		})
		return nil
	}
	// IPv4
	if (packet[0] >> 4) == 4 {
		return net.IP(packet[16:20])
	}
	// Debug: log first 20 bytes
	structuredLog.Debug("packet_read", "Non-IPv4 packet", map[string]interface{}{
		"len":        len(packet),
		"first_byte": fmt.Sprintf("0x%02x", packet[0]),
		"header":     fmt.Sprintf("%x", packet[:20]),
	})
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// handleWebSocket handles WebSocket connections
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Extract UUID from path: /ws/{uuid}
	uuid := extractUUIDFromPath(r.URL.Path)
	if uuid == "" {
		structuredLog.Warn("websocket_rejected", "Invalid UUID in path", nil)
		http.Error(w, "Invalid UUID", http.StatusBadRequest)
		return
	}

	// Check if UUID is authorized
	if !s.clientManager.IsUUIDAuthorized(uuid) {
		structuredLog.Warn("websocket_rejected", "Unauthorized UUID", map[string]interface{}{
			"uuid": uuid,
		})
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get IP for this UUID
	clientIP, ok := s.clientManager.GetIPByUUID(uuid)
	if !ok {
		structuredLog.Warn("websocket_rejected", "No IP assigned for UUID", map[string]interface{}{
			"uuid": uuid,
		})
		http.Error(w, "No IP assigned", http.StatusInternalServerError)
		return
	}

	// Upgrade WebSocket connection
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		structuredLog.Error("websocket_upgrade", "Upgrade failed", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	// Create client instance
	clientID := fmt.Sprintf("client-%s", uuid)
	client := &Client{
		ID:     clientID,
		Conn:   conn,
		IP:     net.ParseIP(clientIP),
		IPStr:  clientIP,
		UUID:   uuid,
		stopCh: make(chan struct{}),
	}

	// Register client
	s.clientsMu.Lock()
	s.clients[clientID] = client
	s.metrics.AddClient(clientID)
	s.clientsMu.Unlock()

	// Add route table entry
	s.routeMu.Lock()
	s.ipRoute[clientIP] = clientID
	s.routeMu.Unlock()

	structuredLog.Info("client_connected", "Client connected", map[string]interface{}{
		"client_id": clientID,
		"uuid":      uuid,
		"ip":        clientIP,
	})

	// Send client their assigned IP (protected by write mutex)
	client.writeMu.Lock()
	if err := conn.WriteMessage(websocket.TextMessage, []byte(clientIP)); err != nil {
		structuredLog.Error("client_ip_send", "Failed to send IP to client", map[string]interface{}{
			"error": err.Error(),
		})
		conn.Close()
		client.writeMu.Unlock()
		return
	}
	client.writeMu.Unlock()

	// Start bidirectional packet forwarding
	go s.forwardToTUN(client)
	go s.forwardToClient(client)
}

// extractUUIDFromPath extracts UUID from WebSocket path /ws/{uuid}
func extractUUIDFromPath(path string) string {
	// Path format: /ws/{uuid}
	// Remove leading /ws/
	if len(path) < 5 {
		return ""
	}
	if path[:4] != "/ws/" {
		return ""
	}
	uuid := path[4:]
	if uuid == "" {
		return ""
	}
	return uuid
}

// forwardToTUN forwards packets from WebSocket to TUN interface
func (s *Server) forwardToTUN(client *Client) {
	defer func() {
		s.clientsMu.Lock()
		delete(s.clients, client.ID)
		s.clientsMu.Unlock()

		// Remove route table entry
		s.routeMu.Lock()
		delete(s.ipRoute, client.IPStr)
		s.routeMu.Unlock()

		s.metrics.RemoveClient(client.ID)
		client.Conn.Close()
		close(client.stopCh)
		structuredLog.Info("client_disconnected", "Client disconnected", map[string]interface{}{
			"client_id": client.ID,
			"uuid":      client.UUID,
		})
	}()

	for {
		messageType, data, err := client.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				structuredLog.Debug("websocket_error", "WebSocket read error", map[string]interface{}{
					"client_id": client.ID,
					"error":     err.Error(),
				})
			}
			return
		}

		if messageType != websocket.BinaryMessage {
			continue
		}

		// Record inbound traffic
		s.metrics.RecordInbound(len(data))

		// Remove obfuscation padding
		var packet []byte
		config := s.getConfig()
		if config.Obfuscation {
			packet, err = obfuscation.RemovePadding(data)
			if err != nil {
				structuredLog.Warn("obfuscation_remove", "Failed to remove padding", map[string]interface{}{
					"client_id": client.ID,
					"error":     err.Error(),
				})
				continue
			}
		} else {
			packet = data
		}

		// Write packet to TUN interface
		if _, err := s.tunDevice.Write(packet); err != nil {
			structuredLog.Error("tun_write", "Failed to write to TUN", map[string]interface{}{
				"client_id": client.ID,
				"error":     err.Error(),
			})
			return
		}
	}
}

// routePacket finds the client for a destination IP using O(1) lookup
func (s *Server) routePacket(packet []byte) *Client {
	dstIP := getDstIP(packet)
	if dstIP == nil {
		return nil
	}

	s.routeMu.RLock()
	defer s.routeMu.RUnlock()

	clientID := s.ipRoute[dstIP.String()]
	return s.clients[clientID]
}

// forwardToClient forwards packets from TUN interface to WebSocket
func (s *Server) forwardToClient(client *Client) {
	buffer := packetPool.Get().([]byte)
	defer packetPool.Put(buffer)

	for {
		select {
		case <-client.stopCh:
			return
		default:
		}

		n, err := s.tunDevice.Read(buffer)
		if err != nil {
			structuredLog.Error("tun_read", "Failed to read from TUN", map[string]interface{}{
				"client_id": client.ID,
				"error":     err.Error(),
			})
			return
		}

		packet := buffer[:n]

		// Debug: log packet info
		dstIP := getDstIP(packet)
		structuredLog.Debug("tun_read", "Packet read from TUN", map[string]interface{}{
			"bytes":   n,
			"dst_ip":  dstIP.String(),
			"client":  client.ID,
		})

		// Hold clientsMu read lock while looking up and writing to targetClient.
		// This prevents forwardToTUN's defer from removing the client between
		// the lookup and the write.
		s.clientsMu.RLock()
		targetClient := s.routePacket(packet)
		if targetClient == nil {
			s.clientsMu.RUnlock()
			structuredLog.Debug("route_miss", "No route for packet", map[string]interface{}{
				"dst_ip": dstIP.String(),
			})
			continue
		}

		// Add obfuscation padding
		config := s.getConfig()
		sendData := packet
		if config.Obfuscation {
			sendData = obfuscation.SimulateHTTPSPattern(packet)
		}

		// Record outbound traffic
		s.metrics.RecordOutbound(len(sendData))

		structuredLog.Debug("route_hit", "Packet routed to client", map[string]interface{}{
			"target":    targetClient.ID,
			"bytes":     len(sendData),
			"dst_ip":    dstIP.String(),
		})

		// Send packet to the target client (not necessarily the reader)
		targetClient.writeMu.Lock()
		if err := targetClient.Conn.WriteMessage(websocket.BinaryMessage, sendData); err != nil {
			structuredLog.Debug("websocket_write", "Failed to write to WebSocket", map[string]interface{}{
				"target":    targetClient.ID,
				"error":     err.Error(),
			})
			targetClient.writeMu.Unlock()
			s.clientsMu.RUnlock()
			// Don't return here, continue processing other packets
			continue
		}
		targetClient.writeMu.Unlock()
		s.clientsMu.RUnlock()
	}
}

// setupSignalHandlers sets up SIGHUP handler for config hot reload and graceful shutdown
func setupSignalHandlers(ctx context.Context, cancel context.CancelFunc, clientManager *ClientManager) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		for sig := range sigCh {
			switch sig {
			case syscall.SIGHUP:
				structuredLog.Info("config_reload", "Received SIGHUP, reloading configuration", nil)
				newConfig, err := loadConfig("server.json")
				if err != nil {
					structuredLog.Error("config_reload", "Failed to reload config", map[string]interface{}{
						"error": err.Error(),
					})
					continue
				}
				if err := clientManager.Reload(newConfig.ClientsFile); err != nil {
					structuredLog.Error("config_reload", "Failed to reload clients", map[string]interface{}{
						"error": err.Error(),
					})
					continue
				}
				structuredLog.Info("config_reload", "Configuration reloaded successfully", map[string]interface{}{
					"obfuscation": newConfig.Obfuscation,
					"transport":   newConfig.Transport,
				})
			case syscall.SIGINT, syscall.SIGTERM:
				structuredLog.Info("server_shutdown", "Received shutdown signal", nil)
				cancel()
			}
		}
	}()
}

func main() {
	// Initialize obfuscation with secure random seed
	obfuscation.InitObfuscation()

	// Parse command-line flags
	configPath := flag.String("config", "server.json", "Path to server configuration file")
	clientsPath := flag.String("clients", "", "Path to clients configuration file (overrides config file)")
	version := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	// Print version if requested
	if *version {
		fmt.Printf("WSVPN Server %s\n", Version)
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

	// Override clients file if specified via flag
	if *clientsPath != "" {
		config.ClientsFile = *clientsPath
	}

	// Set default log directory if not specified
	if config.LogDir == "" {
		config.LogDir = "/var/log/wsvpn/server"
	}

	// Set default transport to websocket for backward compatibility
	if config.Transport == "" {
		config.Transport = "websocket"
	}

	// Set default SOCKS5 port
	if config.SOCKS5Port == 0 {
		config.SOCKS5Port = 1744
	}

	// Initialize structured logger
	structuredLog, err = logger.New("server", config.LogDir, logger.ParseLevel(config.LogLevel))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer structuredLog.Close()

	structuredLog.Info("server_start", "WSVPN Server starting", map[string]interface{}{
		"version":     Version,
		"transport":   config.Transport,
		"listen_addr": config.ListenAddr,
		"obfuscation": config.Obfuscation,
	})

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// Create client manager
	clientManager := NewClientManager()
	if err := clientManager.LoadClients(config.ClientsFile); err != nil {
		structuredLog.Error("client_load", "Failed to load clients", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}
	structuredLog.Info("client_load", "Clients loaded", map[string]interface{}{
		"count": clientManager.GetClientCount(),
		"file":  config.ClientsFile,
	})

	// Initialize TUN interface (shared by both transports)
	ifce, err := water.New(water.Config{
		DeviceType: water.TUN,
		PlatformSpecificParams: water.PlatformSpecificParams{
			Name: config.Name,
		},
	})
	if err != nil {
		structuredLog.Error("tun_init", "Failed to create TUN interface", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	// Bring up the interface
	link, err := netlink.LinkByName(config.Name)
	if err != nil {
		structuredLog.Error("tun_init", "Failed to get link", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	if err := netlink.LinkSetUp(link); err != nil {
		structuredLog.Error("tun_init", "Failed to bring up interface", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	// Set IP address
	addr, err := netlink.ParseAddr(config.ServerIP + "/24")
	if err != nil {
		structuredLog.Error("tun_init", "Failed to parse address", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	if err := netlink.AddrAdd(link, addr); err != nil {
		structuredLog.Error("tun_init", "Failed to add address", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	structuredLog.Info("tun_init", "TUN interface initialized", map[string]interface{}{
		"name": config.Name,
		"ip":   config.ServerIP,
	})

	// Start WebSocket server (TCP) - backward compatible with V0.3
	if config.Transport == "websocket" || config.Transport == "both" {
		wsAddr := config.ListenAddr
		if wsAddr == "" {
			wsAddr = ":8080"
		}
		go func() {
			server := &Server{
				clients:   make(map[string]*Client),
				ipRoute:   make(map[string]string),
				tunDevice: ifce,
				upgrader: websocket.Upgrader{
					ReadBufferSize:  2048,
					WriteBufferSize: 2048,
					CheckOrigin: func(r *http.Request) bool {
						return true
					},
				},
				clientManager: clientManager,
				metrics:       NewMetrics(),
				ctx:           ctx,
				cancel:        cancel,
			}
			server.setConfig(config)

			// Register handlers
			http.HandleFunc("/ws/", server.handleWebSocket)
			http.HandleFunc("/ws/health", server.HandleHealth)
			http.HandleFunc("/ws/reload", server.HandleConfigReload)

			structuredLog.Info("websocket_start", "WebSocket server listening", map[string]interface{}{
				"addr": wsAddr,
			})
			if err := http.ListenAndServe(wsAddr, nil); err != nil {
				structuredLog.Error("websocket_error", "WebSocket server error", map[string]interface{}{
					"error": err.Error(),
				})
			}
		}()
	}

	// Start QUIC server (UDP) - V0.4 new feature
	// TODO: QUIC implementation pending (quic.go not yet merged)
	// if config.Transport == "quic" || config.Transport == "both" {
	// 	quicAddr := config.QUICListenAddr
	// 	if quicAddr == "" {
	// 		quicAddr = ":443"
	// 	}
	// 	go func() {
	// 		quicServer, err := NewQUICServer(config, ifce)
	// 		if err != nil {
	// 			log.Fatalf("Failed to create QUIC server: %v", err)
	// 		}
	// 		log.Printf("QUIC server listening on %s", quicAddr)
	// 		if err := quicServer.Serve(); err != nil {
	// 			log.Printf("QUIC server error: %v", err)
	// 		}
	// 	}()
	// }

	// Start built-in SOCKS5 proxy (enabled by default)
	if config.SOCKS5Enabled {
		socks5Addr := net.JoinHostPort(config.ServerIP, fmt.Sprintf("%d", config.SOCKS5Port))
		go startSOCKS5(socks5Addr)
	} else {
		structuredLog.Info("socks5_disabled", "SOCKS5 proxy disabled via config", nil)
	}

	// Setup signal handlers for hot reload
	setupSignalHandlers(ctx, cancel, clientManager)

	// Block forever
	<-ctx.Done()
	structuredLog.Info("server_shutdown", "Server shutting down", nil)
}
