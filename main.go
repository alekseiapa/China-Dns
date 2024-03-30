package main

import (
	chinadns "github.com/alekseiapa/China-Dns/chinadns"
	"github.com/alekseiapa/China-Dns/loggerconfig"
)

func main() {
	logger := loggerconfig.NewLogger()
	config := loadServerConfig()
	logger.Infof("Starting DNS server on %s", config.ListenAddr)
	dnsServer, err := chinadns.NewDNServer(config)
	if err != nil {
		logger.Fatalf("Failed to create DNS server: %v", err)
	}
	if err := dnsServer.Start(); err != nil {
		logger.Fatalf("DNS server failed: %v", err)
	}
}

// loadServerConfig loads the server configuration.
func loadServerConfig() chinadns.ServerConfig {
	return chinadns.ServerConfig{
		PrimaryDNS:   "8.8.8.8",
		SecondaryDNS: "8.8.4.4",
		ListenAddr:   "127.0.0.1:53",
	}
}
