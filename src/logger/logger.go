package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// LogLevel represents the severity level of a log entry
type LogLevel string

const (
	LevelDebug LogLevel = "debug"
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
)

// LogEntry represents a structured log entry
type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     LogLevel               `json:"level"`
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

// Logger provides structured logging with file output
type Logger struct {
	mu        sync.Mutex
	writer    io.Writer
	file      *os.File
	level     LogLevel
	component string
	logDir    string
	date      string
}

// New creates a new structured logger
func New(component, logDir string, level LogLevel) (*Logger, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	l := &Logger{
		component: component,
		logDir:    logDir,
		level:     level,
		date:      time.Now().Format("2006-01-02"),
	}

	if err := l.openFile(); err != nil {
		return nil, err
	}

	return l, nil
}

// openFile opens or rotates the log file
func (l *Logger) openFile() error {
	currentDate := time.Now().Format("2006-01-02")
	
	// Rotate if date changed
	if currentDate != l.date {
		if l.file != nil {
			l.file.Close()
		}
		l.date = currentDate
	}

	// Open today's log file
	filename := filepath.Join(l.logDir, fmt.Sprintf("%s.jsonl", l.date))
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	l.file = file
	l.writer = file
	return nil
}

// log writes a log entry
func (l *Logger) log(level LogLevel, event, message string, fields map[string]interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Check if rotation is needed
	if err := l.openFile(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to rotate log: %v\n", err)
	}

	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     level,
		Component: l.component,
		Event:     event,
		Message:   message,
		Extra:     fields,
	}

	// Add caller info for debug level
	if level == LevelDebug {
		_, file, line, ok := runtime.Caller(2)
		if ok {
			parts := strings.Split(file, "/")
			if len(parts) > 2 {
				file = strings.Join(parts[len(parts)-2:], "/")
			}
			entry.Extra["caller"] = fmt.Sprintf("%s:%d", file, line)
		}
	}

	data, err := json.Marshal(entry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal log entry: %v\n", err)
		return
	}

	l.writer.Write(data)
	l.writer.Write([]byte("\n"))
}

// Debug logs a debug message
func (l *Logger) Debug(event, message string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelDebug, event, message, f)
}

// Info logs an info message
func (l *Logger) Info(event, message string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelInfo, event, message, f)
}

// Warn logs a warning message
func (l *Logger) Warn(event, message string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelWarn, event, message, f)
}

// Error logs an error message
func (l *Logger) Error(event, message string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelError, event, message, f)
}

// WithClientID returns a logger with client ID context
func (l *Logger) WithClientID(clientID string) *Logger {
	return &Logger{
		mu:        sync.Mutex{},
		writer:    l.writer,
		file:      l.file,
		level:     l.level,
		component: l.component,
		logDir:    l.logDir,
		date:      l.date,
	}
}

// Close closes the log file
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// ParseLevel parses a string into LogLevel
func ParseLevel(s string) LogLevel {
	switch strings.ToLower(s) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}
