package main

import (
	"github.com/metacubex/mihomo/component/geodata"
	"github.com/metacubex/mihomo/component/resolver"
	"github.com/metacubex/mihomo/config"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/hub"
	"github.com/metacubex/mihomo/listener"
	"github.com/metacubex/mihomo/log"
	"github.com/metacubex/mihomo/tunnel"
)

// handleUnloadConfig drops the loaded profile while keeping the core
// initialised: the next setupConfig is a cold load, but nothing the old
// profile built — proxies and their pooled transport sessions, rule sets,
// decoded geodata, DNS cache — stays reachable in between. Parking keeps
// all of that resident for a fast reconnect; on low-memory devices the
// resident profile is what the system ends up killing other apps over.
func handleUnloadConfig() bool {
	runLock.Lock()
	defer runLock.Unlock()
	isRunning = false
	listener.StopListener()
	closeConnections()

	replaced := loadedProxies()
	currentConfig, _ = config.ParseRawConfig(config.DefaultRawConfig())
	hub.ApplyConfig(currentConfig)
	// Replaced proxies are never closed by an apply; their idle pooled
	// sessions would otherwise keep a receive loop and buffers alive.
	for _, proxy := range replaced {
		if err := proxy.Adapter().Close(); err != nil {
			log.Debugln("[APP] close unloaded proxy %s: %v", proxy.Name(), err)
		}
	}

	geodata.ClearGeoSiteCache()
	geodata.ClearGeoIPCache()
	resolver.ClearCache()
	resolver.ResetConnection()
	handleForceGC()
	return true
}

// loadedProxies returns every proxy the current profile holds, top-level
// and provider-supplied alike.
func loadedProxies() []C.Proxy {
	proxies := make([]C.Proxy, 0, len(tunnel.Proxies()))
	for _, proxy := range tunnel.Proxies() {
		proxies = append(proxies, proxy)
	}
	for _, provider := range tunnel.Providers() {
		proxies = append(proxies, provider.Proxies()...)
	}
	return proxies
}
