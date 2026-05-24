//go:build windows && !cli

package main

import (
	"fmt"
	"os"
	"os/exec"
	"sync/atomic"
	"time"

	"github.com/getlantern/systray"
	"wsvpn/obfuscation"
)

var (
	guiConnected  int32
	guiStopCh     chan struct{}
	guiDone       chan struct{}
	guiStatusItem *systray.MenuItem
	guiConnItem   *systray.MenuItem
	guiDiscItem   *systray.MenuItem
)

func runGUI(cfgPath string) {
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
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
