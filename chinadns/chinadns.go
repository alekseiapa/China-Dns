package chiandns

import (
	goc "github.com/Chenyao2333/golang-cache"
	"github.com/alekseiapa/China-Dns/loggerconfig"
	"github.com/miekg/dns"
	"github.com/oschwald/geoip2-golang"
	"github.com/sirupsen/logrus"
	"strings"
	"time"
)

type ServerConfig struct {
	PrimaryDNS   string
	SecondaryDNS string
	ListenAddr   string
}

type DNServer struct {
	config           ServerConfig
	dnsServers       [2]*dns.Server
	chinaDomainCache *goc.Cache
	responseCache    *goc.Cache
}

type cacheEntry struct {
	putin time.Time
	reply *dns.Msg
}

type DNServerError string

var geoIPDB *geoip2.Reader
var logger = loggerconfig.NewLogger()

const minimumTTL = 60

func init() {
	var err error
	geoIPDB, err = geoip2.Open("GeoLite2-Country.mmdb")
	if err != nil {
		logger.Fatalln(err)
	}
}

func (e DNServerError) Error() string {
	return string(e)
}

func NewDNServer(cfg ServerConfig) (*DNServer, error) {
	server := &DNServer{}

	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "127.0.0.1:53"
	}
	server.config = cfg

	server.dnsServers[0] = &dns.Server{
		Addr: cfg.ListenAddr,
		Net:  "udp",
		Handler: dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
			server.handleDNSRequest(w, req, "udp")
		}),
	}

	server.dnsServers[1] = &dns.Server{
		Addr: cfg.ListenAddr,
		Net:  "tcp",
		Handler: dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
			server.handleDNSRequest(w, req, "tcp")
		}),
	}

	var err error
	server.chinaDomainCache, err = goc.NewCache("lru", 1024*20)
	if err != nil {
		logger.Fatalln(err)
	}

	server.responseCache, err = goc.NewCache("lru", 1024*20)
	if err != nil {
		logger.Fatalln(err)
	}

	return server, nil
}

func (server *DNServer) Start() error {
	errorChannel := make(chan error, 2)
	doneChannel := make(chan struct{})
	var firstError error

	for _, dnsServer := range server.dnsServers {
		go func(ds *dns.Server) {
			if err := ds.ListenAndServe(); err != nil {
				errorChannel <- err
			}
			doneChannel <- struct{}{}
		}(dnsServer)
	}

	var doneCount int
	for doneCount < 2 {
		select {
		case err := <-errorChannel:
			if err != nil && firstError == nil {
				firstError = err
			}
		case <-doneChannel:
			doneCount++
		}
	}

	return firstError
}

func (server *DNServer) handleDNSRequest(writer dns.ResponseWriter, request *dns.Msg, network string) {
	response := &dns.Msg{}
	var err error
	var resolved bool

	if len(request.Question) < 1 {
		response.SetRcode(request, dns.RcodeNameError)
		writer.WriteMsg(response)
		return
	}

	qname := request.Question[0].Name
	var upstreamSource string

	// Check local hosts file, cache, or resolve via network
	if response, resolved = server.LookupHosts(request); resolved {
		upstreamSource = "hosts"
	} else if response, resolved = server.LookupCache(request, network); resolved {
		upstreamSource = "cache"
	} else {
		response, upstreamSource, err = server.LookupNetwork(request, network)
		if err != nil {
			logger.Error(err)
		}
		if response == nil {
			response = createServerFailureResponse(request)
		}
	}

	response.SetRcode(request, response.Rcode)

	logEntry := logger.WithFields(logrus.Fields{
		"action":     "handleDNSRequest",
		"domain":     qname,
		"queryType":  dns.TypeToString[request.Question[0].Qtype],
		"upstream":   upstreamSource,
		"statusCode": dns.RcodeToString[response.Rcode],
	})
	if response.Rcode == dns.RcodeSuccess {
		logEntry.Info()
	} else {
		logEntry.Warn()
	}

	writer.WriteMsg(response)
}

// LookupNetwork resolves the DNS request through the network.
// The function returns the DNS response, the name of the used upstream server, and an error, if any.
// In the current implementation, the error is always nil.
func (server *DNServer) LookupNetwork(request *dns.Msg, network string) (*dns.Msg, string, error) {
	primaryDNSChan := make(chan *dns.Msg)
	secondaryDNSChan := make(chan *dns.Msg)

	queryDNS := func(responseChan chan *dns.Msg, useSecondary bool) {
		upstreamDNS := server.config.PrimaryDNS
		if useSecondary {
			upstreamDNS = server.config.SecondaryDNS
		}
		if !strings.Contains(upstreamDNS, ":") {
			upstreamDNS += ":53"
		}

		response, err := resolve(request, upstreamDNS, network)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"operation": "resolve",
				"upstream":  upstreamDNS,
				"domain":    request.Question[0].Name,
			}).Error(err)
		}

		if response == nil || (!useSecondary && (response.Rcode != dns.RcodeSuccess || err != nil || server.isResponsePolluted(response))) {
			responseChan <- nil
			return
		}

		responseChan <- response
	}

	go queryDNS(primaryDNSChan, false)
	go queryDNS(secondaryDNSChan, true)

	go func() {
		time.Sleep(2 * time.Second)
		primaryDNSChan <- nil
		secondaryDNSChan <- nil
	}()

	response := <-primaryDNSChan
	if response != nil {
		server.setResponseCache(response, network)
		return response, server.config.PrimaryDNS, nil
	}

	response = <-secondaryDNSChan
	if response == nil {
		response = createServerFailureResponse(request)
	}
	if response.Rcode == dns.RcodeSuccess {
		server.setResponseCache(response, network)
	}
	return response, server.config.SecondaryDNS, nil
}

func (server *DNServer) isResponsePolluted(response *dns.Msg) bool {
	if containsARecord(response) {
		chinaIP := containsChinaIP(response)
		server.chinaDomainCache.Set(response.Question[0].Name, chinaIP)
		return !chinaIP
	}

	china, found := server.chinaDomainCache.Get(response.Question[0].Name)
	if found {
		return !china.(bool)
	}
	return false
}

func (server *DNServer) LookupCache(request *dns.Msg, network string) (*dns.Msg, bool) {
	key := generateCacheKey(request, network)
	cachedItem, found := server.responseCache.Get(key)
	if found {
		cacheEntry := cachedItem.(cacheEntry)
		elapsed := time.Since(cacheEntry.putin).Seconds()

		responseCopy := cacheEntry.reply.Copy()
		needsUpdate := subtractTTL(responseCopy, int(elapsed))
		if needsUpdate {
			go server.refreshCache(request, network)
		}

		return responseCopy, true
	}
	return nil, false
}

func (server *DNServer) refreshCache(request *dns.Msg, network string) {
	response, upstream, _ := server.LookupNetwork(request, network)
	logEntry := logger.WithFields(logrus.Fields{
		"action":    "refreshCache",
		"domain":    request.Question[0].Name,
		"queryType": dns.TypeToString[request.Question[0].Qtype],
		"upstream":  upstream,
		"status":    dns.RcodeToString[response.Rcode],
	})

	if response.Rcode != dns.RcodeSuccess {
		logEntry.Warn()
	} else {
		logEntry.Info()
	}
}

func (server *DNServer) setResponseCache(response *dns.Msg, network string) {
	key := generateCacheKey(response, network)
	server.responseCache.Set(key, cacheEntry{
		putin: time.Now(),
		reply: response,
	})
}

func (server *DNServer) LookupHosts(request *dns.Msg) (*dns.Msg, bool) {
	// TODO: Implement host lookup logic
	return nil, false
}
