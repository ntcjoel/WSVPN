//go:build windows && !cli

package main

import (
	"context"
	"encoding/json"
	_ "embed"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/getlantern/systray"
	"wsvpn/obfuscation"
)

//go:embed config.html
var configFormHTML string

var (
	guiConnected  int32
	guiStopCh     chan struct{}
	guiDone       chan struct{}
	guiStatusItem *systray.MenuItem
	guiConnItem   *systray.MenuItem
	guiDiscItem   *systray.MenuItem
	guiCfgPath    string
	guiSrv        *http.Server
)

func MessageBox(hwnd uintptr, text, caption string, flags uint) int {
	u32, _ := syscall.UTF16PtrFromString(text)
	u16c, _ := syscall.UTF16PtrFromString(caption)
	ret, _, _ := syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(
		hwnd, uintptr(unsafe.Pointer(u32)), uintptr(unsafe.Pointer(u16c)), uintptr(flags))
	return int(ret)
}

func runGUI(cfgPath string) {
	guiCfgPath = cfgPath
	guiStopCh = make(chan struct{})
	guiDone = make(chan struct{})

	// First run with no config? Open web setup
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		openConfigWeb()
	}

	systray.Run(onReady, onExit)
}

func onReady() {
	systray.SetTooltip("WSVPN — disconnected")

	guiStatusItem = systray.AddMenuItem("Disconnected", "")
	guiStatusItem.Disable()
	systray.AddSeparator()

	guiConnItem = systray.AddMenuItem("Connect", "Start VPN connection")
	guiDiscItem = systray.AddMenuItem("Disconnect", "Stop VPN connection")
	guiDiscItem.Hide()

	systray.AddSeparator()
	settingsItem := systray.AddMenuItem("Settings...", "Configure VPN")
	reloadItem := systray.AddMenuItem("Reload Config", "Reconnect with new settings")
	systray.AddSeparator()
	quitItem := systray.AddMenuItem("Exit", "Quit WSVPN")

	go func() {
		for range guiConnItem.ClickedCh {
			if atomic.LoadInt32(&guiConnected) == 0 {
				go guiConnectLoop()
			}
		}
	}()
	go func() {
		for range guiDiscItem.ClickedCh {
			if client != nil {
				client.stop()
			}
		}
	}()
	go func() {
		for range settingsItem.ClickedCh {
			openConfigWeb()
		}
	}()
	go func() {
		for range reloadItem.ClickedCh {
			if client != nil {
				client.stop()
				time.Sleep(1 * time.Second)
			}
			go guiConnectLoop()
		}
	}()
	go func() {
		<-quitItem.ClickedCh
		systray.Quit()
	}()

	// Auto-connect if config exists
	if client == nil {
		cfg, err := loadConfig(guiCfgPath)
		if err == nil && cfg.ServerURL != "" && cfg.UUID != "" {
			client = &Client{cfg: cfg}
			go guiConnectLoop()
		}
	}
}

func onExit() {
	close(guiStopCh)
	if guiSrv != nil {
		guiSrv.Shutdown(context.Background())
	}
	if client != nil {
		client.stop()
	}
	<-guiDone
}

func openConfigWeb() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	mux := http.NewServeMux()

	mux.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		cfg, err := loadConfig(guiCfgPath)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]string{})
			return
		}
		json.NewEncoder(w).Encode(cfg)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(configFormHTML))
	})

	mux.HandleFunc("/save", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var cfg Config
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			http.Error(w, "Marshal error", http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(guiCfgPath, data, 0644); err != nil {
			http.Error(w, "Write error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		go func() {
			if client != nil {
				client.stop()
				time.Sleep(1 * time.Second)
			}
			client = &Client{cfg: &cfg}
			go guiConnectLoop()
		}()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	guiSrv = &http.Server{Addr: fmt.Sprintf("127.0.0.1:%d", port), Handler: mux}
	go guiSrv.ListenAndServe()

	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

func guiConnectLoop() {
	defer close(guiDone)

	for {
		select {
		case <-guiStopCh:
			return
		default:
		}

		if atomic.LoadInt32(&guiConnected) == 1 {
			return
		}

		if client == nil || client.cfg == nil {
			time.Sleep(3 * time.Second)
			continue
		}

		client.running = true
		client.stopCh = make(chan struct{})
		client.shape = obfuscation.NewShaperState(client.cfg.TrafficShape)

		_, err := client.connect()
		if err != nil {
			if client.cfg.Reconnect {
				time.Sleep(5 * time.Second)
				continue
			}
			return
		}

		atomic.StoreInt32(&guiConnected, 1)
		systray.SetTooltip("WSVPN — connected")
		guiStatusItem.SetTitle("Connected")
		guiConnItem.Hide()
		guiDiscItem.Show()

		go client.forwardToServer()
		go client.forwardFromServer()
		if client.cfg.Transport == "websocket" {
			go client.irregularHeartbeat()
		}
		if client.cfg.TrafficInduction {
			client.inductionCh = make(chan struct{})
			go client.trafficInductionLoop()
		}

		client.forwardFromServer()

		atomic.StoreInt32(&guiConnected, 0)
		systray.SetTooltip("WSVPN — disconnected")
		guiStatusItem.SetTitle("Disconnected")
		guiConnItem.Show()
		guiDiscItem.Hide()

		if !client.cfg.Reconnect {
			return
		}
		time.Sleep(5 * time.Second)
	}
}
