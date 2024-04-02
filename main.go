package main

import (
	"flag"
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
	primaryDNS := flag.String("primarydns", "8.8.8.8", "Primary DNS server address")
	secondaryDNS := flag.String("secondarydns", "8.8.4.4", "Secondary DNS server address")
	listenAddr := flag.String("listenaddr", "127.0.0.1:53", "DNS server listen address")
	cacheSize := flag.Int("cachesize", 1024*20, "Size of cache")

	flag.Parse()

	config := chinadns.ServerConfig{
		PrimaryDNS:   *primaryDNS,
		SecondaryDNS: *secondaryDNS,
		ListenAddr:   *listenAddr,
		CacheSize:    *cacheSize,
	}

	return config
}
