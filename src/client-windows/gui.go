//go:build windows && !cli

package main

import (
	"fmt"
	"os"
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/getlantern/systray"
	"wsvpn/obfuscation"
)

// MessageBox calls the Windows MessageBoxW API.
func MessageBox(hwnd uintptr, text, caption string, flags uint) int {
	u32, _ := syscall.UTF16PtrFromString(text)
	u16c, _ := syscall.UTF16PtrFromString(caption)
	ret, _, _ := syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(
		hwnd, uintptr(unsafe.Pointer(u32)), uintptr(unsafe.Pointer(u16c)), uintptr(flags))
	return int(ret)
}

var (
	guiConnected  int32
	guiStopCh     chan struct{}
	guiDone       chan struct{}
	guiStatusItem *systray.MenuItem
	guiConnItem   *systray.MenuItem
	guiDiscItem   *systray.MenuItem
)

func showError(title, msg string) {
	MessageBox(0, msg, title, 0x10) // MB_ICONERROR
}

func runGUI(cfgPath string) {
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		showError("WSVPN", fmt.Sprintf("Cannot load config:\n%s\n\nPlace client.json next to wsvpn-client-gui.exe", cfgPath))
		os.Exit(1)
	}
	if cfg.ServerURL == "" {
		showError("WSVPN", "server_url is not set in client.json\n\nRight-click tray icon → Settings to configure.")
		os.Exit(1)
	}
	if cfg.UUID == "" {
		showError("WSVPN", "uuid is not set in client.json\n\nRight-click tray icon → Settings to configure.")
		os.Exit(1)
	}

	client = &Client{cfg: cfg}
	guiStopCh = make(chan struct{})
	guiDone = make(chan struct{})

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
	settingsItem := systray.AddMenuItem("Settings...", "Edit configuration file")
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
			openConfigEditor()
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

	// Auto-connect on start
	go guiConnectLoop()
}

func onExit() {
	close(guiStopCh)
	if client != nil {
		client.stop()
	}
	<-guiDone
}

func openConfigEditor() {
	cfgPath := findConfig()
	// Use Notepad to edit the config — the simplest cross-version approach
	exec.Command("notepad.exe", cfgPath).Start()
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
			return // already connected
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
