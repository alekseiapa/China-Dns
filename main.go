package main

import (
	"github.com/alekseiapa/China-Dns/chinadns"
	"log"
)

func main() {
	config := chinadns.Config{
		FastDNS:  "114.114.114.114",
		CleanDNS: "8.8.8.8",
		// Optionally, specify IP and Port if different from defaults
	}

	s, err := chinadns.NewServer(config)
	if err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
	}

	err = s.Run()
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
