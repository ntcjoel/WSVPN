package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// LogEntry represents a structured log entry (matches logger.LogEntry)
type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Component string                 `json:"component"`
	Event     string                 `json:"event"`
	Message   string                 `json:"message,omitempty"`
	ClientID  string                 `json:"client_id,omitempty"`
	ClientIP  string                 `json:"client_ip,omitempty"`
	UUID      string                 `json:"uuid,omitempty"`
	Bytes     int                    `json:"bytes,omitempty"`
	DstIP     string                 `json:"dst_ip,omitempty"`
	SrcIP     string                 `json:"src_ip,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Duration  string                 `json:"duration,omitempty"`
	Extra     map[string]interface{} `json:"extra,omitempty"`
}

// LogStats represents log statistics
type LogStats struct {
	TotalEntries   int            `json:"total_entries"`
	EntriesByLevel map[string]int `json:"entries_by_level"`
	UniqueClients  int            `json:"unique_clients"`
	TotalBytes     int            `json:"total_bytes"`
	Connections    int            `json:"connections"`
	Errors         int            `json:"errors"`
	TimeRange      TimeRange      `json:"time_range"`
}

// TimeRange represents a time range
type TimeRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// LogViewer handles log viewing
type LogViewer struct {
	logDir      string
	mu          sync.RWMutex
	subscribers map[*websocket.Conn]bool
}

// NewLogViewer creates a new log viewer
func NewLogViewer(logDir string) *LogViewer {
	return &LogViewer{
		logDir:      logDir,
		subscribers: make(map[*websocket.Conn]bool),
	}
}

// LogFileInfo holds file name and modification time
type LogFileInfo struct {
	Name    string
	ModTime time.Time
	Size    int64
}

// GetLogFiles returns list of available log files sorted by modification time
func (lv *LogViewer) GetLogFiles() ([]LogFileInfo, error) {
	var files []LogFileInfo

	entries, err := os.ReadDir(lv.logDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read log directory: %w", err)
	}

	log.Printf("GetLogFiles: found %d entries in %s", len(entries), lv.logDir)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		log.Printf("GetLogFiles: checking file %s", name)
		if strings.HasSuffix(name, ".jsonl") || strings.HasSuffix(name, ".jsonl.gz") || strings.HasSuffix(name, ".log") {
			fullPath := filepath.Join(lv.logDir, name)
			info, err := os.Stat(fullPath)
			if err != nil {
				log.Printf("GetLogFiles: stat error for %s: %v", name, err)
				continue
			}
			log.Printf("GetLogFiles: added %s (%d bytes)", name, info.Size())
			files = append(files, LogFileInfo{
				Name:    name,
				ModTime: info.ModTime(),
				Size:    info.Size(),
			})
		}
	}

	// Sort by modification time (newest first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime.After(files[j].ModTime)
	})
	log.Printf("GetLogFiles: returning %d files", len(files))
	return files, nil
}

// ReadLogFile reads a log file (supports .gz, with limit)
func (lv *LogViewer) ReadLogFile(filename string, limit int) ([]LogEntry, error) {
	filepath := filepath.Join(lv.logDir, filename)
	file, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	var reader io.Reader = file
	if strings.HasSuffix(filename, ".gz") {
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	var entries []LogEntry
	scanner := bufio.NewScanner(reader)
	count := 0
	for scanner.Scan() {
		if limit > 0 && count >= limit {
			break
		}
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			// Fallback: treat as plain text log
			entry = LogEntry{
				Timestamp: time.Now().Format(time.RFC3339),
				Level:     "info",
				Component: "server",
				Event:     "log",
				Message:   line,
			}
		}
		entries = append(entries, entry)
		count++
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading log file: %w", err)
	}

	return entries, nil
}

// CalculateStats calculates statistics from log entries
func (lv *LogViewer) CalculateStats(entries []LogEntry) LogStats {
	stats := LogStats{
		EntriesByLevel: make(map[string]int),
	}

	clientSet := make(map[string]bool)

	for _, entry := range entries {
		stats.TotalEntries++
		stats.EntriesByLevel[entry.Level]++

		if entry.ClientID != "" {
			clientSet[entry.ClientID] = true
		}

		if entry.Bytes > 0 {
			stats.TotalBytes += entry.Bytes
		}

		if entry.Event == "client_connected" || strings.Contains(entry.Message, "connected") {
			stats.Connections++
		}

		if entry.Level == "error" {
			stats.Errors++
		}

		if stats.TimeRange.From == "" || entry.Timestamp < stats.TimeRange.From {
			stats.TimeRange.From = entry.Timestamp
		}
		if stats.TimeRange.To == "" || entry.Timestamp > stats.TimeRange.To {
			stats.TimeRange.To = entry.Timestamp
		}
	}

	stats.UniqueClients = len(clientSet)
	return stats
}

// FilterEntries filters log entries by criteria
func (lv *LogViewer) FilterEntries(entries []LogEntry, level, clientID, event, search string) []LogEntry {
	var filtered []LogEntry

	for _, entry := range entries {
		// Filter by level
		if level != "" && entry.Level != level {
			continue
		}

		// Filter by client ID
		if clientID != "" && !strings.Contains(entry.ClientID, clientID) {
			continue
		}

		// Filter by event
		if event != "" && !strings.Contains(entry.Event, event) {
			continue
		}

		// Full-text search
		if search != "" {
			searchLower := strings.ToLower(search)
			if !strings.Contains(strings.ToLower(entry.Message), searchLower) &&
				!strings.Contains(strings.ToLower(entry.Event), searchLower) &&
				!strings.Contains(strings.ToLower(entry.ClientID), searchLower) {
				continue
			}
		}

		filtered = append(filtered, entry)
	}

	return filtered
}

// BroadcastLog sends a log entry to all WebSocket subscribers
func (lv *LogViewer) BroadcastLog(entry LogEntry) {
	lv.mu.RLock()
	defer lv.mu.RUnlock()

	data, _ := json.Marshal(entry)
	for conn := range lv.subscribers {
		conn.WriteMessage(websocket.TextMessage, data)
	}
}

// WebSocket upgrader
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

// HTML Template
const logViewerTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>WSVPN Log Viewer</title>
    <script src="https://cdn.tailwindcss.com"></script>
    <script src="https://unpkg.com/htmx.org@1.9.10"></script>
    <style>
        .log-debug { border-left: 3px solid #6b7280; }
        .log-info { border-left: 3px solid #3b82f6; }
        .log-warn { border-left: 3px solid #f59e0b; }
        .log-error { border-left: 3px solid #ef4444; }
        .log-entry { font-family: 'Monaco', 'Consolas', monospace; font-size: 0.875rem; }
    </style>
</head>
<body class="bg-gray-100 h-screen flex">
    <!-- Sidebar -->
    <div class="w-64 bg-white shadow-lg p-4 overflow-y-auto">
        <h1 class="text-xl font-bold mb-4 text-gray-800">WSVPN Logs</h1>
        
        <!-- Stats -->
        <div class="mb-4 p-3 bg-blue-50 rounded">
            <h2 class="font-semibold text-sm text-gray-700">Today's Stats</h2>
            <div class="mt-2 space-y-1 text-xs">
                <div class="flex justify-between">
                    <span>Entries:</span>
                    <span class="font-mono">{{.Stats.TotalEntries}}</span>
                </div>
                <div class="flex justify-between">
                    <span>Clients:</span>
                    <span class="font-mono">{{.Stats.UniqueClients}}</span>
                </div>
                <div class="flex justify-between">
                    <span>Connections:</span>
                    <span class="font-mono">{{.Stats.Connections}}</span>
                </div>
                <div class="flex justify-between">
                    <span>Errors:</span>
                    <span class="font-mono text-red-600">{{.Stats.Errors}}</span>
                </div>
                <div class="flex justify-between">
                    <span>Traffic:</span>
                    <span class="font-mono">{{divf .Stats.TotalBytes 1048576 | printf "%.2f"}} MB</span>
                </div>
            </div>
        </div>

        <!-- Log Files -->
        <h2 class="font-semibold text-sm text-gray-700 mb-2">Log Files</h2>
        <div class="space-y-1">
            {{range .LogFiles}}
            <a href="/logs/{{.Name}}" 
               class="block px-3 py-2 text-sm rounded hover:bg-gray-100 {{if eq .Name $.CurrentFile}}bg-blue-100 text-blue-700{{end}}">
                {{.Name}} <span class="text-gray-400 text-xs">({{divf64 .Size 1024 | printf "%.1f"}} KB)</span>
            </a>
            {{end}}
        </div>
    </div>

    <!-- Main Content -->
    <div class="flex-1 flex flex-col">
        <!-- Toolbar -->
        <div class="bg-white shadow p-4 flex gap-4 items-center">
            <!-- Search -->
            <input type="text" 
                   id="search"
                   placeholder="Search logs..." 
                   class="flex-1 px-3 py-2 border rounded text-sm"
                   hx-get="/logs/search"
                   hx-trigger="keyup changed delay:300ms"
                   hx-target="#log-container"
                   hx-name="q">
            
            <!-- Level Filter -->
            <select class="px-3 py-2 border rounded text-sm"
                    hx-get="/logs/filter"
                    hx-trigger="change"
                    hx-target="#log-container"
                    hx-name="level">
                <option value="">All Levels</option>
                <option value="debug">Debug</option>
                <option value="info">Info</option>
                <option value="warn">Warn</option>
                <option value="error">Error</option>
            </select>

            <!-- Auto-scroll Toggle -->
            <label class="flex items-center text-sm">
                <input type="checkbox" id="auto-scroll" checked class="mr-2">
                Auto-scroll
            </label>

            <!-- Live Toggle -->
            <button id="live-toggle" 
                    class="px-4 py-2 bg-green-500 text-white rounded text-sm hover:bg-green-600"
                    onclick="toggleLive()">
                Live: ON
            </button>
        </div>

        <!-- Log Container -->
        <div id="log-container" class="flex-1 overflow-y-auto p-4 bg-white">
            {{range .Entries}}
            <div class="log-entry log-{{.Level}} mb-2 p-2 bg-gray-50 rounded hover:bg-gray-100">
                <div class="flex items-center gap-2 text-xs text-gray-500 mb-1">
                    <span class="font-mono">{{.Timestamp}}</span>
                    <span class="px-2 py-0.5 rounded text-white text-xs
                        {{if eq .Level "debug"}}bg-gray-500{{end}}
                        {{if eq .Level "info"}}bg-blue-500{{end}}
                        {{if eq .Level "warn"}}bg-yellow-500{{end}}
                        {{if eq .Level "error"}}bg-red-500{{end}}">
                        {{.Level}}
                    </span>
                    <span class="font-mono">{{.Event}}</span>
                    {{if .ClientID}}
                    <span class="text-blue-600">{{.ClientID}}</span>
                    {{end}}
                </div>
                <div class="text-gray-800">{{.Message}}</div>
                {{if .Bytes}}
                <div class="text-xs text-gray-500 mt-1">Bytes: {{.Bytes}} | Dst: {{.DstIP}}</div>
                {{end}}
            </div>
            {{end}}
        </div>
    </div>

    <script>
        let liveMode = true;
        let ws = null;

        function toggleLive() {
            liveMode = !liveMode;
            const btn = document.getElementById('live-toggle');
            if (liveMode) {
                btn.textContent = 'Live: ON';
                btn.classList.remove('bg-gray-500');
                btn.classList.add('bg-green-500');
                connectWebSocket();
            } else {
                btn.textContent = 'Live: OFF';
                btn.classList.remove('bg-green-500');
                btn.classList.add('bg-gray-500');
                if (ws) ws.close();
            }
        }

        function connectWebSocket() {
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            ws = new WebSocket(protocol + '//' + window.location.host + '/logs/stream');
            
            ws.onmessage = function(event) {
                if (!liveMode) return;
                
                const entry = JSON.parse(event.data);
                const container = document.getElementById('log-container');
                
                const div = document.createElement('div');
                div.className = 'log-entry log-' + entry.level + ' mb-2 p-2 bg-gray-50 rounded hover:bg-gray-100';
                
                var color = 'blue';
                if (entry.level === 'error') color = 'red';
                else if (entry.level === 'warn') color = 'yellow';
                else if (entry.level === 'debug') color = 'gray';
                
                var clientHtml = entry.client_id ? '<span class="text-blue-600">' + entry.client_id + '</span>' : '';
                var msgHtml = entry.message || '';
                
                div.innerHTML = '<div class="flex items-center gap-2 text-xs text-gray-500 mb-1">' +
                    '<span class="font-mono">' + entry.timestamp + '</span>' +
                    '<span class="px-2 py-0.5 rounded text-white text-xs bg-' + color + '-500">' + entry.level + '</span>' +
                    '<span class="font-mono">' + entry.event + '</span>' +
                    clientHtml +
                    '</div><div class="text-gray-800">' + msgHtml + '</div>';
                
                container.appendChild(div);
                
                if (document.getElementById('auto-scroll').checked) {
                    container.scrollTop = container.scrollHeight;
                }
            };

            ws.onclose = function() {
                if (liveMode) {
                    setTimeout(connectWebSocket, 3000);
                }
            };
        }

        // Connect on page load
        connectWebSocket();
    </script>
</body>
</html>
`

func main() {
	var (
		logDir   = flag.String("log-dir", "/var/log/wsvpn/server", "Log directory")
		port     = flag.Int("port", 8181, "Web viewer port")
		host     = flag.String("host", "0.0.0.0", "Bind address")
	)
	flag.Parse()

	viewer := NewLogViewer(*logDir)

	// Parse template
	tmpl := template.Must(template.New("logs").Funcs(template.FuncMap{
		"divf": func(a, b int) float64 {
			return float64(a) / float64(b)
		},
		"divf64": func(a, b int64) float64 {
			return float64(a) / float64(b)
		},
	}).Parse(logViewerTemplate))

	// Routes
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		files, err := viewer.GetLogFiles()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Use the latest log file if today's file doesn't exist
		var currentFile string
		var entries []LogEntry
		
		// Try today's file first
		today := time.Now().Format("2006-01-02") + ".jsonl"
		entries, err = viewer.ReadLogFile(today, 1000)
		if err == nil && len(entries) > 0 {
			currentFile = today
		} else if len(files) > 0 {
			// Fall back to the latest file
			currentFile = files[0].Name
			entries, _ = viewer.ReadLogFile(currentFile, 1000)
		}
		
		// Calculate stats
		stats := viewer.CalculateStats(entries)

		data := map[string]interface{}{
			"LogFiles":    files,
			"CurrentFile": currentFile,
			"Entries":     entries,
			"Stats":       stats,
		}

		w.Header().Set("Content-Type", "text/html")
		tmpl.Execute(w, data)
	})

	http.HandleFunc("/logs/", func(w http.ResponseWriter, r *http.Request) {
		filename := strings.TrimPrefix(r.URL.Path, "/logs/")
		if filename == "" || filename == "stream" || filename == "search" || filename == "filter" {
			return
		}

		// Get limit from query param (default 1000)
		limit := 1000
		if l := r.URL.Query().Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil && n > 0 {
				limit = n
			}
		}

		entries, err := viewer.ReadLogFile(filename, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		stats := viewer.CalculateStats(entries)
		files, _ := viewer.GetLogFiles()

		data := map[string]interface{}{
			"LogFiles":   files,
			"CurrentFile": filename,
			"Entries":    entries,
			"Stats":      stats,
		}

		w.Header().Set("Content-Type", "text/html")
		tmpl.Execute(w, data)
	})

	http.HandleFunc("/logs/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		today := time.Now().Format("2006-01-02") + ".jsonl"
		entries, _ := viewer.ReadLogFile(today, 500)
		filtered := viewer.FilterEntries(entries, "", "", "", q)

		// Render partial
		for _, entry := range filtered {
			renderLogEntry(w, entry)
		}
	})

	http.HandleFunc("/logs/filter", func(w http.ResponseWriter, r *http.Request) {
		level := r.URL.Query().Get("level")
		today := time.Now().Format("2006-01-02") + ".jsonl"
		entries, _ := viewer.ReadLogFile(today, 500)
		filtered := viewer.FilterEntries(entries, level, "", "", "")

		for _, entry := range filtered {
			renderLogEntry(w, entry)
		}
	})

	http.HandleFunc("/logs/stream", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		viewer.mu.Lock()
		viewer.subscribers[conn] = true
		viewer.mu.Unlock()

		defer func() {
			viewer.mu.Lock()
			delete(viewer.subscribers, conn)
			viewer.mu.Unlock()
		}()

		// Keep connection alive
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	})

	http.HandleFunc("/logs/stats", func(w http.ResponseWriter, r *http.Request) {
		today := time.Now().Format("2006-01-02") + ".jsonl"
		entries, err := viewer.ReadLogFile(today, 10000)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		stats := viewer.CalculateStats(entries)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	})

	addr := fmt.Sprintf("%s:%d", *host, *port)
	log.Printf("WSVPN Log Viewer starting on http://%s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func renderLogEntry(w io.Writer, entry LogEntry) {
	html := fmt.Sprintf(`
<div class="log-entry log-%s mb-2 p-2 bg-gray-50 rounded hover:bg-gray-100">
    <div class="flex items-center gap-2 text-xs text-gray-500 mb-1">
        <span class="font-mono">%s</span>
        <span class="px-2 py-0.5 rounded text-white text-xs bg-%s-500">%s</span>
        <span class="font-mono">%s</span>
        %s
    </div>
    <div class="text-gray-800">%s</div>
</div>`,
		entry.Level,
		entry.Timestamp,
		levelToColor(entry.Level),
		entry.Level,
		entry.Event,
		clientIDHTML(entry.ClientID),
		entry.Message,
	)
	io.WriteString(w, html)
}

func levelToColor(level string) string {
	switch level {
	case "debug":
		return "gray"
	case "info":
		return "blue"
	case "warn":
		return "yellow"
	case "error":
		return "red"
	default:
		return "gray"
	}
}

func clientIDHTML(clientID string) string {
	if clientID == "" {
		return ""
	}
	return fmt.Sprintf(`<span class="text-blue-600">%s</span>`, clientID)
}
