package main

import (
	"os"
	"path/filepath"
	"testing"

	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/tunnel"
)

const unloadTestProfile = `mixed-port: 0
proxies:
  - name: unload-probe
    type: socks5
    server: 127.0.0.1
    port: 1
rules:
  - MATCH,unload-probe
`

func TestUnloadConfigDropsLoadedProfile(t *testing.T) {
	home := t.TempDir()
	C.SetHomeDir(home)
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte(unloadTestProfile), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := applyConfig(&SetupParams{}); err != nil {
		t.Fatalf("applyConfig: %v", err)
	}
	if _, ok := tunnel.Proxies()["unload-probe"]; !ok {
		t.Fatal("profile proxy missing after apply")
	}
	if len(tunnel.Rules()) == 0 {
		t.Fatal("profile rules missing after apply")
	}

	if !handleUnloadConfig() {
		t.Fatal("unload reported failure")
	}
	if _, ok := tunnel.Proxies()["unload-probe"]; ok {
		t.Fatal("profile proxy still loaded after unload")
	}
	if len(tunnel.Rules()) != 0 {
		t.Fatalf("rules still loaded after unload: %d", len(tunnel.Rules()))
	}
	if isRunning {
		t.Fatal("core still marked running after unload")
	}

	// A later setup must load cleanly on top of the unloaded core.
	if err := applyConfig(&SetupParams{}); err != nil {
		t.Fatalf("re-apply after unload: %v", err)
	}
	if _, ok := tunnel.Proxies()["unload-probe"]; !ok {
		t.Fatal("profile proxy missing after re-apply")
	}
}
