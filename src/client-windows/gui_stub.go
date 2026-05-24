//go:build windows && cli

package main

// Stub for CLI-only builds — no GUI dependency (no systray import).

func runGUI(cfgPath string) {
	// GUI not available in CLI build — fall through to connectCmd
}
